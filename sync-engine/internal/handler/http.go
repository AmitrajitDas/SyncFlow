package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/syncflow/sync-engine/internal/checkpoint"
	"github.com/syncflow/sync-engine/internal/config"
	ws "github.com/syncflow/sync-engine/internal/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper CORS check in production
		return true
	},
}

// Server represents the HTTP server
type Server struct {
	config        *config.Config
	engine        *gin.Engine
	wsHub         *ws.Hub
	checkpointGen *checkpoint.Generator
	mongoClient   *mongo.Client
	redisClient   *redis.Client
	httpServer    *http.Server
}

// NewServer creates a new HTTP server
func NewServer(
	cfg *config.Config,
	wsHub *ws.Hub,
	checkpointGen *checkpoint.Generator,
	mongoClient *mongo.Client,
	redisClient *redis.Client,
) *Server {
	// Set Gin mode based on environment
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		config:        cfg,
		engine:        gin.Default(),
		wsHub:         wsHub,
		checkpointGen: checkpointGen,
		mongoClient:   mongoClient,
		redisClient:   redisClient,
	}

	s.setupRoutes()

	return s
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.engine.GET("/health", s.handleHealth)

	// Metrics
	s.engine.GET("/metrics", s.handleMetrics)

	// WebSocket endpoint
	s.engine.GET("/ws", s.handleWebSocket)

	// Checkpoint endpoints
	checkpoints := s.engine.Group("/checkpoints")
	{
		checkpoints.POST("/:bucketId", s.handleCreateCheckpoint)
		checkpoints.GET("/:bucketId", s.handleListCheckpoints)
		checkpoints.GET("/:bucketId/:checkpointId", s.handleGetCheckpoint)
		checkpoints.DELETE("/:bucketId/:checkpointId", s.handleDeleteCheckpoint)
	}
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[HTTP Server] Starting on %s", addr)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("[HTTP Server] Shutting down...")
	return s.httpServer.Shutdown(ctx)
}

// handleHealth returns server health status
func (s *Server) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	health := gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"services":  gin.H{},
	}

	services := health["services"].(gin.H)

	// Check MongoDB
	if err := s.mongoClient.Ping(ctx, nil); err != nil {
		services["mongodb"] = "unhealthy"
		health["status"] = "degraded"
	} else {
		services["mongodb"] = "healthy"
	}

	// Check Redis
	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		services["redis"] = "unhealthy"
		health["status"] = "degraded"
	} else {
		services["redis"] = "healthy"
	}

	// WebSocket hub stats
	wsStats := s.wsHub.GetStats()
	health["websocket"] = wsStats

	c.JSON(http.StatusOK, health)
}

// handleMetrics returns server metrics
func (s *Server) handleMetrics(c *gin.Context) {
	wsStats := s.wsHub.GetStats()

	metrics := gin.H{
		"timestamp":       time.Now().Unix(),
		"websocket_stats": wsStats,
		"server_version":  "1.0.0",
	}

	c.JSON(http.StatusOK, metrics)
}

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(c *gin.Context) {
	// Extract authentication from query params
	// TODO: Implement proper JWT validation
	userID := c.Query("userId")
	deviceID := c.Query("deviceId")
	bucketsStr := c.Query("buckets")

	if userID == "" || deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing userId or deviceId",
		})
		return
	}

	// Parse buckets (comma-separated)
	var buckets []string
	if bucketsStr != "" {
		buckets = strings.Split(bucketsStr, ",")
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[HTTP Server] WebSocket upgrade failed: %v", err)
		return
	}

	// Create client
	client := ws.NewClient(conn, s.wsHub, userID, deviceID, buckets)

	// Register client with hub
	s.wsHub.Register(client)

	// Start read/write pumps
	go client.WritePump()
	go client.ReadPump()

	log.Printf("[HTTP Server] WebSocket client connected: userID=%s, deviceID=%s", userID, deviceID)
}

// handleCreateCheckpoint creates a checkpoint for a bucket
func (s *Server) handleCreateCheckpoint(c *gin.Context) {
	bucketID := c.Param("bucketId")

	// Generate checkpoint
	metadata, err := s.checkpointGen.GenerateCheckpoint(c.Request.Context(), bucketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create checkpoint",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, metadata)
}

// handleListCheckpoints lists all checkpoints for a bucket
func (s *Server) handleListCheckpoints(c *gin.Context) {
	bucketID := c.Param("bucketId")

	checkpoints, err := s.checkpointGen.ListCheckpoints(bucketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to list checkpoints",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bucketId":    bucketID,
		"checkpoints": checkpoints,
		"count":       len(checkpoints),
	})
}

// handleGetCheckpoint retrieves a specific checkpoint
func (s *Server) handleGetCheckpoint(c *gin.Context) {
	bucketID := c.Param("bucketId")
	checkpointID := c.Param("checkpointId")

	metadata, err := s.checkpointGen.GetCheckpointMetadata(checkpointID, bucketID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Checkpoint not found",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// handleDeleteCheckpoint deletes a checkpoint
func (s *Server) handleDeleteCheckpoint(c *gin.Context) {
	bucketID := c.Param("bucketId")
	checkpointID := c.Param("checkpointId")

	if err := s.checkpointGen.DeleteCheckpoint(checkpointID, bucketID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to delete checkpoint",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Checkpoint deleted successfully",
	})
}
