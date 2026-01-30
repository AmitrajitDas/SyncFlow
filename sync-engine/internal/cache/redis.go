package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/syncflow/sync-engine/internal/config"
)

// RedisClient wraps the Redis client with sync-engine specific operations
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client
func NewRedisClient(cfg *config.Config) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// StoreResumeToken stores a MongoDB change stream resume token
func (r *RedisClient) StoreResumeToken(ctx context.Context, collection string, token []byte) error {
	key := fmt.Sprintf("resume_token:%s", collection)
	return r.client.Set(ctx, key, token, 0).Err()
}

// GetResumeToken retrieves a stored resume token
func (r *RedisClient) GetResumeToken(ctx context.Context, collection string) ([]byte, error) {
	key := fmt.Sprintf("resume_token:%s", collection)
	result, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return result, err
}

// TrackConnection tracks a WebSocket connection for a bucket
func (r *RedisClient) TrackConnection(ctx context.Context, bucketID, clientID string) error {
	key := fmt.Sprintf("connections:%s", bucketID)
	return r.client.SAdd(ctx, key, clientID).Err()
}

// UntrackConnection removes a WebSocket connection tracking
func (r *RedisClient) UntrackConnection(ctx context.Context, bucketID, clientID string) error {
	key := fmt.Sprintf("connections:%s", bucketID)
	return r.client.SRem(ctx, key, clientID).Err()
}

// GetConnectionCount returns the number of active connections for a bucket
func (r *RedisClient) GetConnectionCount(ctx context.Context, bucketID string) (int64, error) {
	key := fmt.Sprintf("connections:%s", bucketID)
	return r.client.SCard(ctx, key).Result()
}

// GetAllConnections returns all client IDs connected to a bucket
func (r *RedisClient) GetAllConnections(ctx context.Context, bucketID string) ([]string, error) {
	key := fmt.Sprintf("connections:%s", bucketID)
	return r.client.SMembers(ctx, key).Result()
}

// CacheBucketState caches the current state of a bucket
func (r *RedisClient) CacheBucketState(ctx context.Context, bucketID string, state interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("bucket_state:%s", bucketID)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal bucket state: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// GetBucketState retrieves cached bucket state
func (r *RedisClient) GetBucketState(ctx context.Context, bucketID string, dest interface{}) error {
	key := fmt.Sprintf("bucket_state:%s", bucketID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// SetUserBucket maps a user ID to their bucket ID
func (r *RedisClient) SetUserBucket(ctx context.Context, userID, bucketID string) error {
	key := fmt.Sprintf("user_bucket:%s", userID)
	return r.client.Set(ctx, key, bucketID, 0).Err()
}

// GetUserBucket retrieves the bucket ID for a user
func (r *RedisClient) GetUserBucket(ctx context.Context, userID string) (string, error) {
	key := fmt.Sprintf("user_bucket:%s", userID)
	result, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return result, err
}

// PublishSyncEvent publishes a sync event to a Redis channel (for multi-instance support)
func (r *RedisClient) PublishSyncEvent(ctx context.Context, bucketID string, event interface{}) error {
	channel := fmt.Sprintf("sync:%s", bucketID)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal sync event: %w", err)
	}
	return r.client.Publish(ctx, channel, data).Err()
}

// SubscribeSyncEvents subscribes to sync events for a bucket
func (r *RedisClient) SubscribeSyncEvents(ctx context.Context, bucketID string) *redis.PubSub {
	channel := fmt.Sprintf("sync:%s", bucketID)
	return r.client.Subscribe(ctx, channel)
}

// IncrementMetric increments a metric counter
func (r *RedisClient) IncrementMetric(ctx context.Context, metric string) error {
	key := fmt.Sprintf("metrics:%s", metric)
	return r.client.Incr(ctx, key).Err()
}

// GetMetric retrieves a metric value
func (r *RedisClient) GetMetric(ctx context.Context, metric string) (int64, error) {
	key := fmt.Sprintf("metrics:%s", metric)
	result, err := r.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return result, err
}

// HealthCheck performs a health check on Redis
func (r *RedisClient) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
