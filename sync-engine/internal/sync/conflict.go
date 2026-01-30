package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/syncflow/sync-engine/internal/kafka"
	"github.com/syncflow/sync-engine/pkg/models"
)

const (
	// Redis key prefixes
	redisDocVersionPrefix    = "doc:version:"
	redisDocTimestampPrefix  = "doc:timestamp:"
	redisConflictWindowSecs  = 5  // Consider edits within 5 seconds as potential conflicts
	redisTTL                 = 5 * time.Minute
)

// Resolver handles conflict detection and resolution
type Resolver struct {
	redisClient    *redis.Client
	kafkaProducer  *kafka.Producer
	conflictWindow time.Duration
}

// NewResolver creates a new conflict resolver
func NewResolver(redisClient *redis.Client, kafkaProducer *kafka.Producer) *Resolver {
	return &Resolver{
		redisClient:    redisClient,
		kafkaProducer:  kafkaProducer,
		conflictWindow: redisConflictWindowSecs * time.Second,
	}
}

// DetectAndResolve checks for conflicts and resolves them using Last-Write-Wins
func (r *Resolver) DetectAndResolve(ctx context.Context, doc map[string]interface{}, docID, bucketID, collection string) (*models.ConflictResolution, error) {
	// Get document timestamp
	docTimestamp, err := r.getDocumentTimestamp(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to get document timestamp: %w", err)
	}

	// Check Redis for recent version
	cachedVersion, cachedTimestamp, err := r.getCachedVersion(ctx, docID)
	if err != nil {
		// No cached version, no conflict
		if err == redis.Nil {
			// Cache current version
			r.cacheVersion(ctx, docID, doc, docTimestamp)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get cached version: %w", err)
	}

	// Check if timestamps are within conflict window
	timeDiff := docTimestamp.Sub(cachedTimestamp).Abs()
	if timeDiff > r.conflictWindow {
		// Not a conflict - edits are far apart
		r.cacheVersion(ctx, docID, doc, docTimestamp)
		return nil, nil
	}

	// Conflict detected! Resolve using Last-Write-Wins
	log.Printf("[Conflict] Detected conflict for doc=%s, timeDiff=%v", docID, timeDiff)

	resolution := r.resolveLastWriteWins(doc, docTimestamp, cachedVersion, cachedTimestamp)

	// Publish conflict event to Kafka
	conflictEvent := &models.ConflictDetectedEvent{
		EventID:       uuid.New().String(),
		DocumentID:    docID,
		BucketID:      bucketID,
		Collection:    collection,
		LocalVersion:  doc,
		RemoteVersion: cachedVersion,
		WinnerVersion: resolution.MergedDoc,
		Resolution:    "lww",
		LocalTime:     docTimestamp,
		RemoteTime:    cachedTimestamp,
		ResolvedAt:    time.Now(),
	}

	if err := r.kafkaProducer.PublishConflictDetected(conflictEvent); err != nil {
		log.Printf("[Conflict] Failed to publish conflict event: %v", err)
	}

	// Cache the winner version
	r.cacheVersion(ctx, docID, resolution.MergedDoc, docTimestamp)

	return resolution, nil
}

// resolveLastWriteWins implements Last-Write-Wins conflict resolution
func (r *Resolver) resolveLastWriteWins(localDoc map[string]interface{}, localTime time.Time, remoteDoc map[string]interface{}, remoteTime time.Time) *models.ConflictResolution {
	var winner string
	var mergedDoc map[string]interface{}

	if localTime.After(remoteTime) {
		// Local version is newer
		winner = "local"
		mergedDoc = localDoc
	} else if remoteTime.After(localTime) {
		// Remote version is newer
		winner = "remote"
		mergedDoc = remoteDoc
	} else {
		// Exact same timestamp (rare) - use document ID hash as tiebreaker
		// This ensures deterministic resolution across all nodes
		localID := getStringField(localDoc, "_id")
		remoteID := getStringField(remoteDoc, "_id")

		if localID > remoteID {
			winner = "local"
			mergedDoc = localDoc
		} else {
			winner = "remote"
			mergedDoc = remoteDoc
		}
	}

	log.Printf("[Conflict] LWW Resolution: winner=%s, localTime=%v, remoteTime=%v", winner, localTime, remoteTime)

	return &models.ConflictResolution{
		Strategy:   "lww",
		Winner:     winner,
		MergedDoc:  mergedDoc,
		ResolvedAt: time.Now(),
	}
}

// getCachedVersion retrieves the cached version of a document from Redis
func (r *Resolver) getCachedVersion(ctx context.Context, docID string) (map[string]interface{}, time.Time, error) {
	// Get document version
	versionKey := redisDocVersionPrefix + docID
	versionJSON, err := r.redisClient.Get(ctx, versionKey).Result()
	if err != nil {
		return nil, time.Time{}, err
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(versionJSON), &doc); err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to unmarshal cached doc: %w", err)
	}

	// Get timestamp
	timestampKey := redisDocTimestampPrefix + docID
	timestampStr, err := r.redisClient.Get(ctx, timestampKey).Result()
	if err != nil {
		return nil, time.Time{}, err
	}

	timestamp, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return doc, timestamp, nil
}

// cacheVersion stores a document version in Redis
func (r *Resolver) cacheVersion(ctx context.Context, docID string, doc map[string]interface{}, timestamp time.Time) {
	// Cache document version
	versionKey := redisDocVersionPrefix + docID
	docJSON, err := json.Marshal(doc)
	if err != nil {
		log.Printf("[Conflict] Failed to marshal doc for caching: %v", err)
		return
	}

	if err := r.redisClient.Set(ctx, versionKey, docJSON, redisTTL).Err(); err != nil {
		log.Printf("[Conflict] Failed to cache doc version: %v", err)
		return
	}

	// Cache timestamp
	timestampKey := redisDocTimestampPrefix + docID
	if err := r.redisClient.Set(ctx, timestampKey, timestamp.Format(time.RFC3339Nano), redisTTL).Err(); err != nil {
		log.Printf("[Conflict] Failed to cache timestamp: %v", err)
		return
	}
}

// getDocumentTimestamp extracts the timestamp from a document
func (r *Resolver) getDocumentTimestamp(doc map[string]interface{}) (time.Time, error) {
	// Try common timestamp fields
	timestampFields := []string{"updated_at", "updatedAt", "timestamp", "modifiedAt", "modified_at"}

	for _, field := range timestampFields {
		if val, exists := doc[field]; exists {
			switch v := val.(type) {
			case time.Time:
				return v, nil
			case string:
				parsed, err := time.Parse(time.RFC3339, v)
				if err == nil {
					return parsed, nil
				}
			case int64:
				return time.Unix(v, 0), nil
			}
		}
	}

	// If no timestamp field found, use current time
	return time.Now(), nil
}

// getStringField safely retrieves a string field from a document
func getStringField(doc map[string]interface{}, field string) string {
	if val, exists := doc[field]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ClearCache removes cached version for a document (useful for testing)
func (r *Resolver) ClearCache(ctx context.Context, docID string) error {
	versionKey := redisDocVersionPrefix + docID
	timestampKey := redisDocTimestampPrefix + docID

	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, versionKey)
	pipe.Del(ctx, timestampKey)

	_, err := pipe.Exec(ctx)
	return err
}

// GetCacheStats returns statistics about cached document versions
func (r *Resolver) GetCacheStats(ctx context.Context) (int, error) {
	pattern := redisDocVersionPrefix + "*"
	keys, err := r.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}
