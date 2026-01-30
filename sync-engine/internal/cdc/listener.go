package cdc

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/internal/kafka"
	"github.com/syncflow/sync-engine/internal/sync"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Listener watches MongoDB Change Streams and processes data changes
type Listener struct {
	config          *config.Config
	mongoClient     *mongo.Client
	kafkaProducer   *kafka.Producer
	redisClient     *redis.Client
	diffCalculator  *sync.Calculator
	conflictResolver *sync.Resolver
	stopChan        chan struct{}
}

// NewListener creates a new CDC listener
func NewListener(cfg *config.Config, mongoClient *mongo.Client, kafkaProducer *kafka.Producer, redisClient *redis.Client) *Listener {
	diffCalculator := sync.NewCalculator()
	conflictResolver := sync.NewResolver(redisClient, kafkaProducer)

	return &Listener{
		config:          cfg,
		mongoClient:     mongoClient,
		kafkaProducer:   kafkaProducer,
		redisClient:     redisClient,
		diffCalculator:  diffCalculator,
		conflictResolver: conflictResolver,
		stopChan:        make(chan struct{}),
	}
}

// Start begins watching MongoDB Change Streams
func (l *Listener) Start(ctx context.Context) error {
	database := l.mongoClient.Database(l.config.MongoDB.Database)

	// Create change stream options
	opts := options.ChangeStream().
		SetFullDocument(options.UpdateLookup).
		SetFullDocumentBeforeChange(options.WhenAvailable)

	// Watch all collections in the database
	changeStream, err := database.Watch(ctx, mongo.Pipeline{}, opts)
	if err != nil {
		return fmt.Errorf("failed to create change stream: %w", err)
	}
	defer changeStream.Close(ctx)

	log.Printf("[CDC] Started watching MongoDB change streams on database: %s", l.config.MongoDB.Database)

	// Process change events
	for changeStream.Next(ctx) {
		select {
		case <-l.stopChan:
			log.Println("[CDC] Stopping change stream listener")
			return nil
		default:
			if err := l.processChangeEvent(ctx, changeStream); err != nil {
				log.Printf("[CDC] Error processing change event: %v", err)
				continue
			}
		}
	}

	if err := changeStream.Err(); err != nil {
		return fmt.Errorf("change stream error: %w", err)
	}

	return nil
}

// Stop gracefully stops the listener
func (l *Listener) Stop() {
	close(l.stopChan)
}

// processChangeEvent handles a single change stream event
func (l *Listener) processChangeEvent(ctx context.Context, changeStream *mongo.ChangeStream) error {
	var event models.ChangeStreamEvent
	if err := changeStream.Decode(&event); err != nil {
		return fmt.Errorf("failed to decode change event: %w", err)
	}

	// Extract metadata
	collection := event.NS.Coll
	docID := extractDocumentID(event.DocumentKey)
	operation := mapOperationType(event.OperationType)

	log.Printf("[CDC] Received %s event for collection=%s, docID=%s", operation, collection, docID)

	// Skip system collections
	if shouldSkipCollection(collection) {
		return nil
	}

	// Process based on operation type
	switch operation {
	case models.OperationInsert:
		return l.handleInsert(ctx, &event, collection, docID)
	case models.OperationUpdate, models.OperationReplace:
		return l.handleUpdate(ctx, &event, collection, docID)
	case models.OperationDelete:
		return l.handleDelete(ctx, &event, collection, docID)
	default:
		log.Printf("[CDC] Ignoring operation type: %s", event.OperationType)
		return nil
	}
}

// handleInsert processes insert operations
func (l *Listener) handleInsert(ctx context.Context, event *models.ChangeStreamEvent, collection, docID string) error {
	newDoc := primitiveToMap(event.FullDocument)
	bucketID := extractBucketID(newDoc)

	// Calculate diff (oldDoc is nil for inserts)
	diff := l.diffCalculator.CalculateDiff(nil, newDoc)

	log.Printf("[CDC] Insert: %s", l.diffCalculator.FormatDiffSummary(diff))

	// Publish to Kafka
	dataEvent := &models.DataChangedEvent{
		EventID:     uuid.New().String(),
		Operation:   models.OperationInsert,
		Collection:  collection,
		DocumentID:  docID,
		BucketID:    bucketID,
		NewData:     newDoc,
		Changes:     diff.AddedFields,
		Timestamp:   event.WallTime,
		ServerTime:  time.Now(),
		ResumeToken: extractResumeToken(event.ID),
	}

	return l.kafkaProducer.PublishDataChanged(dataEvent)
}

// handleUpdate processes update and replace operations
func (l *Listener) handleUpdate(ctx context.Context, event *models.ChangeStreamEvent, collection, docID string) error {
	oldDoc := primitiveToMap(event.FullDocumentBeforeChange)
	newDoc := primitiveToMap(event.FullDocument)
	bucketID := extractBucketID(newDoc)

	// Calculate diff
	diff := l.diffCalculator.CalculateDiff(oldDoc, newDoc)

	if !diff.HasChanges {
		log.Printf("[CDC] No changes detected for doc=%s", docID)
		return nil
	}

	log.Printf("[CDC] Update: %s", l.diffCalculator.FormatDiffSummary(diff))

	// Detect and resolve conflicts
	resolution, err := l.conflictResolver.DetectAndResolve(ctx, newDoc, docID, bucketID, collection)
	if err != nil {
		log.Printf("[CDC] Conflict resolution error: %v", err)
	}

	// If conflict was resolved, use the winning version
	finalDoc := newDoc
	if resolution != nil {
		finalDoc = resolution.MergedDoc
		log.Printf("[CDC] Conflict resolved using %s strategy, winner=%s", resolution.Strategy, resolution.Winner)
	}

	// Publish to Kafka
	dataEvent := &models.DataChangedEvent{
		EventID:     uuid.New().String(),
		Operation:   models.OperationUpdate,
		Collection:  collection,
		DocumentID:  docID,
		BucketID:    bucketID,
		OldData:     oldDoc,
		NewData:     finalDoc,
		Changes:     diff.ModifiedFields,
		Timestamp:   event.WallTime,
		ServerTime:  time.Now(),
		ResumeToken: extractResumeToken(event.ID),
	}

	return l.kafkaProducer.PublishDataChanged(dataEvent)
}

// handleDelete processes delete operations
func (l *Listener) handleDelete(ctx context.Context, event *models.ChangeStreamEvent, collection, docID string) error {
	// For deletes, we don't have fullDocument, try to get from cache or use documentKey
	bucketID := ""

	log.Printf("[CDC] Delete: doc=%s", docID)

	// Publish to Kafka
	dataEvent := &models.DataChangedEvent{
		EventID:     uuid.New().String(),
		Operation:   models.OperationDelete,
		Collection:  collection,
		DocumentID:  docID,
		BucketID:    bucketID,
		Timestamp:   event.WallTime,
		ServerTime:  time.Now(),
		ResumeToken: extractResumeToken(event.ID),
	}

	return l.kafkaProducer.PublishDataChanged(dataEvent)
}

// Helper functions

func extractDocumentID(documentKey primitive.M) string {
	if id, ok := documentKey["_id"]; ok {
		switch v := id.(type) {
		case primitive.ObjectID:
			return v.Hex()
		case string:
			return v
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func extractBucketID(doc map[string]interface{}) string {
	if bucketID, ok := doc["bucketId"]; ok {
		if str, ok := bucketID.(string); ok {
			return str
		}
	}
	if bucketID, ok := doc["bucket_id"]; ok {
		if str, ok := bucketID.(string); ok {
			return str
		}
	}
	return ""
}

func extractResumeToken(id primitive.M) string {
	if data, ok := id["_data"]; ok {
		if str, ok := data.(string); ok {
			return str
		}
	}
	return ""
}

func mapOperationType(opType string) models.ChangeOperation {
	switch opType {
	case "insert":
		return models.OperationInsert
	case "update":
		return models.OperationUpdate
	case "replace":
		return models.OperationReplace
	case "delete":
		return models.OperationDelete
	default:
		return models.ChangeOperation(opType)
	}
}

func shouldSkipCollection(collection string) bool {
	// Skip system collections and internal metadata
	skipCollections := map[string]bool{
		"system.views":      true,
		"system.indexes":    true,
		"system.namespaces": true,
		"system.js":         true,
	}

	return skipCollections[collection]
}

func primitiveToMap(pm primitive.M) map[string]interface{} {
	if pm == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range pm {
		result[key] = convertPrimitiveValue(value)
	}
	return result
}

func convertPrimitiveValue(value interface{}) interface{} {
	switch v := value.(type) {
	case primitive.M:
		return primitiveToMap(v)
	case primitive.A:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = convertPrimitiveValue(item)
		}
		return result
	case primitive.ObjectID:
		return v.Hex()
	case primitive.DateTime:
		return v.Time()
	default:
		return v
	}
}
