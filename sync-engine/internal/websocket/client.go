package websocket

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512 KB
)

// Client represents a WebSocket client connection
type Client struct {
	// WebSocket connection
	conn *websocket.Conn

	// Hub that manages this client
	hub *Hub

	// Buffered channel of outbound messages
	send chan []byte

	// User ID
	userID string

	// Device ID
	deviceID string

	// Subscribed buckets
	buckets []string

	// Client metadata
	metadata map[string]string
}

// ClientMessage represents incoming messages from clients
type ClientMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// NewClient creates a new WebSocket client
func NewClient(conn *websocket.Conn, hub *Hub, userID, deviceID string, buckets []string) *Client {
	return &Client{
		conn:     conn,
		hub:      hub,
		send:     make(chan []byte, 256),
		userID:   userID,
		deviceID: deviceID,
		buckets:  buckets,
		metadata: make(map[string]string),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket Client] Error: %v", err)
			}
			break
		}

		// Handle incoming message
		c.handleMessage(message)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Client) handleMessage(data []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[WebSocket Client] Failed to parse message: %v", err)
		c.sendError("invalid_message", "Failed to parse message")
		return
	}

	switch msg.Type {
	case "ping":
		c.handlePing()
	case "subscribe":
		c.handleSubscribe(msg.Payload)
	case "unsubscribe":
		c.handleUnsubscribe(msg.Payload)
	case "get_stats":
		c.handleGetStats()
	default:
		log.Printf("[WebSocket Client] Unknown message type: %s", msg.Type)
		c.sendError("unknown_type", "Unknown message type")
	}
}

// handlePing responds to ping messages
func (c *Client) handlePing() {
	response := map[string]interface{}{
		"type":      "pong",
		"timestamp": time.Now().Unix(),
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// handleSubscribe subscribes the client to a bucket
func (c *Client) handleSubscribe(payload map[string]interface{}) {
	bucketID, ok := payload["bucketId"].(string)
	if !ok {
		c.sendError("invalid_payload", "Missing bucketId")
		return
	}

	c.hub.SubscribeToBucket(c, bucketID)

	// Send confirmation
	response := map[string]interface{}{
		"type": "subscribed",
		"payload": map[string]string{
			"bucketId": bucketID,
		},
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// handleUnsubscribe unsubscribes the client from a bucket
func (c *Client) handleUnsubscribe(payload map[string]interface{}) {
	bucketID, ok := payload["bucketId"].(string)
	if !ok {
		c.sendError("invalid_payload", "Missing bucketId")
		return
	}

	c.hub.UnsubscribeFromBucket(c, bucketID)

	// Send confirmation
	response := map[string]interface{}{
		"type": "unsubscribed",
		"payload": map[string]string{
			"bucketId": bucketID,
		},
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// handleGetStats returns client statistics
func (c *Client) handleGetStats() {
	stats := map[string]interface{}{
		"userId":         c.userID,
		"deviceId":       c.deviceID,
		"buckets":        c.buckets,
		"queuedMessages": len(c.send),
	}

	response := map[string]interface{}{
		"type":    "stats",
		"payload": stats,
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// sendError sends an error message to the client
func (c *Client) sendError(code, message string) {
	response := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// SendMessage sends a message to the client
func (c *Client) SendMessage(msgType string, payload interface{}) error {
	message := map[string]interface{}{
		"type":      msgType,
		"payload":   payload,
		"timestamp": time.Now().Unix(),
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case c.send <- data:
		return nil
	default:
		return nil // Buffer full, skip
	}
}

// Close gracefully closes the client connection
func (c *Client) Close() {
	c.hub.unregister <- c
}
