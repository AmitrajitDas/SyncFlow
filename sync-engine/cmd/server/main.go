package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/syncflow/sync-engine/internal/cdc"
	"github.com/syncflow/sync-engine/internal/checkpoint"
	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/internal/handler"
	"github.com/syncflow/sync-engine/internal/kafka"
	"github.com/syncflow/sync-engine/internal/websocket"
)

func main() {
	log.Println("🚀 Starting SyncFlow Sync Engine...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("✓ Configuration loaded: environment=%s", cfg.Environment)

	// Initialize MongoDB client
	mongoClient, err := initMongoClient(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Printf("Error disconnecting MongoDB: %v", err)
		}
	}()

	log.Println("✓ MongoDB connected")

	// Initialize Redis client
	redisClient := initRedisClient(cfg)
	defer redisClient.Close()

	log.Println("✓ Redis connected")

	// Initialize Kafka producer
	kafkaProducer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Kafka producer: %v", err)
	}
	defer kafkaProducer.Close()

	log.Println("✓ Kafka producer initialized")

	// Initialize Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Kafka consumer: %v", err)
	}
	defer kafkaConsumer.Close()

	log.Println("✓ Kafka consumer initialized")

	// Initialize WebSocket hub
	wsHub := websocket.NewHub(kafkaConsumer)
	go wsHub.Run()

	log.Println("✓ WebSocket hub started")

	// Initialize checkpoint generator
	checkpointGen := checkpoint.NewGenerator(cfg, mongoClient, kafkaProducer)

	log.Println("✓ Checkpoint generator initialized")

	// Initialize CDC listener
	cdcListener := cdc.NewListener(cfg, mongoClient, kafkaProducer, redisClient)

	// Start CDC listener in background
	go func() {
		log.Println("✓ Starting CDC listener...")
		if err := cdcListener.Start(context.Background()); err != nil {
			log.Printf("CDC listener error: %v", err)
		}
	}()

	// Initialize HTTP server
	httpServer := handler.NewServer(cfg, wsHub, checkpointGen, mongoClient, redisClient)

	// Start HTTP server in background
	go func() {
		addr := cfg.GetString("server.http_addr")
		if addr == "" {
			addr = ":8082"
		}

		log.Printf("✓ HTTP server listening on %s", addr)
		if err := httpServer.Start(addr); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Println("✅ SyncFlow Sync Engine is running!")
	log.Println("   - HTTP API: http://localhost:8082")
	log.Println("   - WebSocket: ws://localhost:8082/ws")
	log.Println("   - Health: http://localhost:8082/health")
	log.Println("   - Metrics: http://localhost:8082/metrics")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("\n🛑 Shutting down SyncFlow Sync Engine...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop CDC listener
	cdcListener.Stop()
	log.Println("✓ CDC listener stopped")

	// Stop WebSocket hub
	wsHub.Stop()
	log.Println("✓ WebSocket hub stopped")

	// Stop HTTP server
	if err := httpServer.Stop(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("✓ HTTP server stopped")

	log.Println("✅ SyncFlow Sync Engine shut down gracefully")
}

// initMongoClient initializes MongoDB client with connection pooling
func initMongoClient(cfg *config.Config) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().
		ApplyURI(cfg.MongoDB.URI).
		SetMaxPoolSize(50).
		SetMinPoolSize(10).
		SetMaxConnIdleTime(30 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return client, nil
}

// initRedisClient initializes Redis client
func initRedisClient(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Host,
		Password:     cfg.Redis.Password,
		DB:           0,
		PoolSize:     20,
		MinIdleConns: 5,
		MaxRetries:   3,
	})
}
