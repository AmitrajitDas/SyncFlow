package checkpoint

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/internal/kafka"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Generator creates point-in-time checkpoints of bucket data
type Generator struct {
	config        *config.Config
	mongoClient   *mongo.Client
	kafkaProducer *kafka.Producer
	storagePath   string
}

// NewGenerator creates a new checkpoint generator
func NewGenerator(cfg *config.Config, mongoClient *mongo.Client, kafkaProducer *kafka.Producer) *Generator {
	storagePath := cfg.Checkpoint.Dir
	if storagePath == "" {
		storagePath = "./checkpoints"
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		log.Printf("[Checkpoint] Failed to create storage directory: %v", err)
	}

	return &Generator{
		config:        cfg,
		mongoClient:   mongoClient,
		kafkaProducer: kafkaProducer,
		storagePath:   storagePath,
	}
}

// GenerateCheckpoint creates a checkpoint for a specific bucket
func (g *Generator) GenerateCheckpoint(ctx context.Context, bucketID string) (*models.CheckpointMetadata, error) {
	log.Printf("[Checkpoint] Starting checkpoint generation for bucket=%s", bucketID)

	startTime := time.Now()
	checkpointID := uuid.New().String()

	// Query all documents in the bucket
	documents, err := g.fetchBucketDocuments(ctx, bucketID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bucket documents: %w", err)
	}

	log.Printf("[Checkpoint] Fetched %d documents for bucket=%s", len(documents), bucketID)

	// Create checkpoint file
	checkpointPath, err := g.createCheckpointFile(checkpointID, bucketID, documents)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint file: %w", err)
	}

	// Get file stats
	fileInfo, err := os.Stat(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint file stats: %w", err)
	}

	// Create metadata
	metadata := &models.CheckpointMetadata{
		CheckpointID:  checkpointID,
		BucketID:      bucketID,
		CreatedAt:     startTime,
		DocumentCount: len(documents),
		SizeBytes:     fileInfo.Size(),
		FilePath:      checkpointPath,
		Format:        "tar.gz",
		Version:       "1.0",
	}

	// Save metadata
	if err := g.saveMetadata(metadata); err != nil {
		log.Printf("[Checkpoint] Failed to save metadata: %v", err)
	}

	// Publish checkpoint created event to Kafka
	event := &models.CheckpointCreatedEvent{
		EventID:       uuid.New().String(),
		CheckpointID:  checkpointID,
		BucketID:      bucketID,
		DocumentCount: len(documents),
		SizeBytes:     fileInfo.Size(),
		CreatedAt:     startTime,
		StorageURL:    checkpointPath,
	}

	if err := g.kafkaProducer.PublishCheckpointCreated(event); err != nil {
		log.Printf("[Checkpoint] Failed to publish checkpoint event: %v", err)
	}

	duration := time.Since(startTime)
	log.Printf("[Checkpoint] Checkpoint created: id=%s, bucket=%s, docs=%d, size=%d bytes, duration=%v",
		checkpointID, bucketID, len(documents), fileInfo.Size(), duration)

	return metadata, nil
}

// fetchBucketDocuments retrieves all documents for a bucket
func (g *Generator) fetchBucketDocuments(ctx context.Context, bucketID string) ([]map[string]interface{}, error) {
	database := g.mongoClient.Database(g.config.MongoDB.Database)

	// Get all collections
	collections, err := database.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	var allDocuments []map[string]interface{}

	// Query each collection for documents with the bucket ID
	for _, collName := range collections {
		// Skip system collections
		if shouldSkipCollection(collName) {
			continue
		}

		collection := database.Collection(collName)

		// Find documents with matching bucketId
		filter := bson.M{
			"$or": []bson.M{
				{"bucketId": bucketID},
				{"bucket_id": bucketID},
			},
		}

		cursor, err := collection.Find(ctx, filter)
		if err != nil {
			log.Printf("[Checkpoint] Error querying collection %s: %v", collName, err)
			continue
		}

		var docs []bson.M
		if err := cursor.All(ctx, &docs); err != nil {
			log.Printf("[Checkpoint] Error decoding documents from %s: %v", collName, err)
			cursor.Close(ctx)
			continue
		}
		cursor.Close(ctx)

		// Convert to map[string]interface{} and add collection info
		for _, doc := range docs {
			docMap := make(map[string]interface{})
			for k, v := range doc {
				docMap[k] = v
			}
			docMap["_collection"] = collName
			allDocuments = append(allDocuments, docMap)
		}
	}

	return allDocuments, nil
}

// createCheckpointFile creates a compressed tar.gz file containing the checkpoint data
func (g *Generator) createCheckpointFile(checkpointID, bucketID string, documents []map[string]interface{}) (string, error) {
	// Create checkpoint directory structure: checkpoints/{bucketID}/{checkpointID}.tar.gz
	bucketDir := filepath.Join(g.storagePath, bucketID)
	if err := os.MkdirAll(bucketDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bucket directory: %w", err)
	}

	checkpointPath := filepath.Join(bucketDir, fmt.Sprintf("%s.tar.gz", checkpointID))

	// Create checkpoint file
	file, err := os.Create(checkpointPath)
	if err != nil {
		return "", fmt.Errorf("failed to create checkpoint file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Write documents to tar archive
	for i, doc := range documents {
		docJSON, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			log.Printf("[Checkpoint] Failed to marshal document %d: %v", i, err)
			continue
		}

		// Create tar header
		header := &tar.Header{
			Name:    fmt.Sprintf("doc_%d.json", i),
			Size:    int64(len(docJSON)),
			Mode:    0644,
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return "", fmt.Errorf("failed to write tar header: %w", err)
		}

		if _, err := tarWriter.Write(docJSON); err != nil {
			return "", fmt.Errorf("failed to write document to tar: %w", err)
		}
	}

	return checkpointPath, nil
}

// saveMetadata saves checkpoint metadata to a JSON file
func (g *Generator) saveMetadata(metadata *models.CheckpointMetadata) error {
	metadataPath := metadata.FilePath + ".meta.json"

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// LoadCheckpoint loads a checkpoint file and returns the documents
func (g *Generator) LoadCheckpoint(checkpointID, bucketID string) ([]map[string]interface{}, error) {
	checkpointPath := filepath.Join(g.storagePath, bucketID, fmt.Sprintf("%s.tar.gz", checkpointID))

	// Open checkpoint file
	file, err := os.Open(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint file: %w", err)
	}
	defer file.Close()

	// Create gzip reader
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	var documents []map[string]interface{}

	// Read documents from tar archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Read document data
		docData := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, docData); err != nil {
			return nil, fmt.Errorf("failed to read document data: %w", err)
		}

		// Unmarshal document
		var doc map[string]interface{}
		if err := json.Unmarshal(docData, &doc); err != nil {
			log.Printf("[Checkpoint] Failed to unmarshal document %s: %v", header.Name, err)
			continue
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

// GetCheckpointMetadata retrieves metadata for a checkpoint
func (g *Generator) GetCheckpointMetadata(checkpointID, bucketID string) (*models.CheckpointMetadata, error) {
	metadataPath := filepath.Join(g.storagePath, bucketID, fmt.Sprintf("%s.tar.gz.meta.json", checkpointID))

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata models.CheckpointMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// ListCheckpoints lists all checkpoints for a bucket
func (g *Generator) ListCheckpoints(bucketID string) ([]models.CheckpointMetadata, error) {
	bucketDir := filepath.Join(g.storagePath, bucketID)

	// Check if bucket directory exists
	if _, err := os.Stat(bucketDir); os.IsNotExist(err) {
		return []models.CheckpointMetadata{}, nil
	}

	// Read directory
	entries, err := os.ReadDir(bucketDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read bucket directory: %w", err)
	}

	var checkpoints []models.CheckpointMetadata

	// Load metadata for each checkpoint
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		metadataPath := filepath.Join(bucketDir, entry.Name())
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			log.Printf("[Checkpoint] Failed to read metadata file %s: %v", entry.Name(), err)
			continue
		}

		var metadata models.CheckpointMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			log.Printf("[Checkpoint] Failed to unmarshal metadata %s: %v", entry.Name(), err)
			continue
		}

		checkpoints = append(checkpoints, metadata)
	}

	return checkpoints, nil
}

// DeleteCheckpoint deletes a checkpoint and its metadata
func (g *Generator) DeleteCheckpoint(checkpointID, bucketID string) error {
	checkpointPath := filepath.Join(g.storagePath, bucketID, fmt.Sprintf("%s.tar.gz", checkpointID))
	metadataPath := checkpointPath + ".meta.json"

	// Delete checkpoint file
	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete checkpoint file: %w", err)
	}

	// Delete metadata file
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete metadata file: %w", err)
	}

	log.Printf("[Checkpoint] Deleted checkpoint: id=%s, bucket=%s", checkpointID, bucketID)

	return nil
}

// shouldSkipCollection determines if a collection should be skipped
func shouldSkipCollection(collection string) bool {
	skipCollections := map[string]bool{
		"system.views":      true,
		"system.indexes":    true,
		"system.namespaces": true,
		"system.js":         true,
	}

	return skipCollections[collection]
}
