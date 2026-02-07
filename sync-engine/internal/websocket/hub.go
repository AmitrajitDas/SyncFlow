package websocket

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/syncflow/sync-engine/internal/kafka"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Hub manages WebSocket client connections and message broadcasting
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Bucket-based client subscriptions (bucketID -> clients)
	subscriptions map[string]map[*Client]bool

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast messages to clients
	broadcast chan *BroadcastMessage

	// Kafka consumer for sync events
	kafkaConsumer *kafka.Consumer

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// BroadcastMessage represents a message to be broadcast to clients
type BroadcastMessage struct {
	BucketID string
	Message  []byte
	Type     string
}

// NewHub creates a new WebSocket hub
func NewHub(kafkaConsumer *kafka.Consumer) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[*Client]bool),
		register:      make(chan *Client, 256),
		unregister:    make(chan *Client, 256),
		broadcast:     make(chan *BroadcastMessage, 1024),
		kafkaConsumer: kafkaConsumer,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	log.Println("[WebSocket Hub] Starting...")

	// Start Kafka consumer in background
	go h.consumeKafkaEvents()

	// Main event loop
	for {
		select {
		case <-h.ctx.Done():
			log.Println("[WebSocket Hub] Shutting down...")
			return

		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastToBucket(message)
		}
	}
}

// Stop gracefully shuts down the hub
func (h *Hub) Stop() {
	h.cancel()

	// Close all client connections
	h.mu.Lock()
	for client := range h.clients {
		close(client.send)
	}
	h.mu.Unlock()

	log.Println("[WebSocket Hub] Stopped")
}

// Register adds a client to the hub (public method for handler)
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub (public method for handler)
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// registerClient registers a new client
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = true

	// Subscribe client to buckets
	for _, bucketID := range client.buckets {
		if h.subscriptions[bucketID] == nil {
			h.subscriptions[bucketID] = make(map[*Client]bool)
		}
		h.subscriptions[bucketID][client] = true
	}

	log.Printf("[WebSocket Hub] Client registered: userID=%s, deviceID=%s, buckets=%v",
		client.userID, client.deviceID, client.buckets)
}

// unregisterClient removes a client
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		// Remove from global clients
		delete(h.clients, client)

		// Remove from bucket subscriptions
		for bucketID := range h.subscriptions {
			if clients, exists := h.subscriptions[bucketID]; exists {
				delete(clients, client)

				// Clean up empty subscription maps
				if len(clients) == 0 {
					delete(h.subscriptions, bucketID)
				}
			}
		}

		// Close send channel
		close(client.send)

		log.Printf("[WebSocket Hub] Client unregistered: userID=%s, deviceID=%s",
			client.userID, client.deviceID)
	}
}

// broadcastToBucket sends a message to all clients subscribed to a bucket
func (h *Hub) broadcastToBucket(message *BroadcastMessage) {
	h.mu.RLock()
	clients := h.subscriptions[message.BucketID]
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	log.Printf("[WebSocket Hub] Broadcasting to bucket=%s, clients=%d, type=%s",
		message.BucketID, len(clients), message.Type)

	for client := range clients {
		select {
		case client.send <- message.Message:
			// Message sent successfully
		default:
			// Client's send buffer is full, close connection
			log.Printf("[WebSocket Hub] Client buffer full, closing: userID=%s", client.userID)
			h.unregisterClient(client)
		}
	}
}

// BroadcastToAll sends a message to all connected clients
func (h *Hub) BroadcastToAll(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	log.Printf("[WebSocket Hub] Broadcasting to all clients: count=%d", len(h.clients))

	for client := range h.clients {
		select {
		case client.send <- message:
			// Message sent successfully
		default:
			// Skip if buffer is full
			log.Printf("[WebSocket Hub] Skipping client with full buffer: userID=%s", client.userID)
		}
	}
}

// consumeKafkaEvents consumes data.changed events from Kafka and broadcasts to WebSocket clients
func (h *Hub) consumeKafkaEvents() {
	log.Println("[WebSocket Hub] Starting Kafka consumer...")

	// Subscribe to topics
	topics := []string{
		kafka.TopicDataChanged,
		kafka.TopicSyncEvents,
	}

	// Consume messages
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			messages, err := h.kafkaConsumer.ConsumeMessages(h.ctx, topics, 100*time.Millisecond)
			if err != nil {
				log.Printf("[WebSocket Hub] Kafka consume error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			for _, msg := range messages {
				h.handleKafkaMessage(msg)
			}
		}
	}
}

// handleKafkaMessage processes a Kafka message and broadcasts to WebSocket clients
func (h *Hub) handleKafkaMessage(msg *kafka.Message) {
	switch msg.Topic {
	case kafka.TopicDataChanged:
		h.handleDataChangedEvent(msg)
	case kafka.TopicSyncEvents:
		h.handleSyncEvent(msg)
	default:
		log.Printf("[WebSocket Hub] Unknown topic: %s", msg.Topic)
	}
}

// handleDataChangedEvent processes data.changed events
func (h *Hub) handleDataChangedEvent(msg *kafka.Message) {
	var event models.DataChangedEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("[WebSocket Hub] Failed to unmarshal DataChangedEvent: %v", err)
		return
	}

	// Create WebSocket message
	wsMessage := models.WebSocketMessage{
		Type:      "data_changed",
		Timestamp: time.Now(),
		Payload:   event,
	}

	messageBytes, err := json.Marshal(wsMessage)
	if err != nil {
		log.Printf("[WebSocket Hub] Failed to marshal WebSocket message: %v", err)
		return
	}

	// Broadcast to bucket subscribers
	h.broadcast <- &BroadcastMessage{
		BucketID: event.BucketID,
		Message:  messageBytes,
		Type:     "data_changed",
	}
}

// handleSyncEvent processes sync.events
func (h *Hub) handleSyncEvent(msg *kafka.Message) {
	var event models.SyncEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.Printf("[WebSocket Hub] Failed to unmarshal SyncEvent: %v", err)
		return
	}

	// Create WebSocket message
	wsMessage := models.WebSocketMessage{
		Type:      "sync_event",
		Timestamp: time.Now(),
		Payload:   event,
	}

	messageBytes, err := json.Marshal(wsMessage)
	if err != nil {
		log.Printf("[WebSocket Hub] Failed to marshal WebSocket message: %v", err)
		return
	}

	// Broadcast to bucket subscribers
	h.broadcast <- &BroadcastMessage{
		BucketID: event.BucketID,
		Message:  messageBytes,
		Type:     "sync_event",
	}
}

// GetStats returns hub statistics
func (h *Hub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	bucketStats := make(map[string]int)
	for bucketID, clients := range h.subscriptions {
		bucketStats[bucketID] = len(clients)
	}

	return map[string]interface{}{
		"total_clients":       len(h.clients),
		"total_subscriptions": len(h.subscriptions),
		"bucket_stats":        bucketStats,
	}
}

// SubscribeToBucket subscribes a client to a bucket
func (h *Hub) SubscribeToBucket(client *Client, bucketID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscriptions[bucketID] == nil {
		h.subscriptions[bucketID] = make(map[*Client]bool)
	}
	h.subscriptions[bucketID][client] = true

	// Add to client's bucket list
	client.buckets = append(client.buckets, bucketID)

	log.Printf("[WebSocket Hub] Client subscribed to bucket: userID=%s, bucketID=%s", client.userID, bucketID)
}

// UnsubscribeFromBucket unsubscribes a client from a bucket
func (h *Hub) UnsubscribeFromBucket(client *Client, bucketID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, exists := h.subscriptions[bucketID]; exists {
		delete(clients, client)

		if len(clients) == 0 {
			delete(h.subscriptions, bucketID)
		}
	}

	// Remove from client's bucket list
	for i, id := range client.buckets {
		if id == bucketID {
			client.buckets = append(client.buckets[:i], client.buckets[i+1:]...)
			break
		}
	}

	log.Printf("[WebSocket Hub] Client unsubscribed from bucket: userID=%s, bucketID=%s", client.userID, bucketID)
}
