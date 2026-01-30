package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserCreatedEvent represents the event published by auth-service when a user registers
type UserCreatedEvent struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	BucketID  string    `json:"bucketId"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChangeOperation represents the type of database operation
type ChangeOperation string

const (
	OperationInsert  ChangeOperation = "insert"
	OperationUpdate  ChangeOperation = "update"
	OperationReplace ChangeOperation = "replace"
	OperationDelete  ChangeOperation = "delete"
)

// SyncEvent represents a data change event to be broadcasted
type SyncEvent struct {
	ID          string                 `json:"id"`
	Operation   ChangeOperation        `json:"operation"`
	Collection  string                 `json:"collection"`
	DocumentID  string                 `json:"documentId"`
	BucketID    string                 `json:"bucketId"`
	Changes     map[string]interface{} `json:"changes,omitempty"`
	FullDoc     map[string]interface{} `json:"fullDoc,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	ResumeToken string                 `json:"resumeToken,omitempty"`
}

// DataChangedEvent represents a change published to Kafka
type DataChangedEvent struct {
	EventID     string                 `json:"eventId"`
	Operation   ChangeOperation        `json:"operation"`
	Collection  string                 `json:"collection"`
	DocumentID  string                 `json:"documentId"`
	BucketID    string                 `json:"bucketId"`
	OldData     map[string]interface{} `json:"oldData,omitempty"`
	NewData     map[string]interface{} `json:"newData,omitempty"`
	Changes     map[string]interface{} `json:"changes,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	ServerTime  time.Time              `json:"serverTime"`
	ResumeToken string                 `json:"resumeToken"`
}

// ConflictDetectedEvent represents a conflict that was resolved
type ConflictDetectedEvent struct {
	EventID       string                 `json:"eventId"`
	DocumentID    string                 `json:"documentId"`
	BucketID      string                 `json:"bucketId"`
	Collection    string                 `json:"collection"`
	LocalVersion  map[string]interface{} `json:"localVersion"`
	RemoteVersion map[string]interface{} `json:"remoteVersion"`
	WinnerVersion map[string]interface{} `json:"winnerVersion"`
	Resolution    string                 `json:"resolution"` // "lww" (Last-Write-Wins)
	LocalTime     time.Time              `json:"localTime"`
	RemoteTime    time.Time              `json:"remoteTime"`
	ResolvedAt    time.Time              `json:"resolvedAt"`
}

// CheckpointCreatedEvent represents a checkpoint creation event
type CheckpointCreatedEvent struct {
	CheckpointID string    `json:"checkpointId"`
	BucketID     string    `json:"bucketId"`
	Collection   string    `json:"collection,omitempty"`
	FilePath     string    `json:"filePath"`
	FileSize     int64     `json:"fileSize"`
	DocumentCount int64    `json:"documentCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ChangeStreamEvent represents a MongoDB change stream event
type ChangeStreamEvent struct {
	ID                primitive.M        `bson:"_id"`
	OperationType     string             `bson:"operationType"`
	FullDocument      primitive.M        `bson:"fullDocument,omitempty"`
	FullDocumentBeforeChange primitive.M `bson:"fullDocumentBeforeChange,omitempty"`
	NS                Namespace          `bson:"ns"`
	DocumentKey       primitive.M        `bson:"documentKey"`
	UpdateDescription *UpdateDescription `bson:"updateDescription,omitempty"`
	ClusterTime       primitive.Timestamp `bson:"clusterTime"`
	WallTime          time.Time          `bson:"wallTime"`
}

// Namespace represents MongoDB namespace (database.collection)
type Namespace struct {
	DB   string `bson:"db"`
	Coll string `bson:"coll"`
}

// UpdateDescription contains the fields that were updated
type UpdateDescription struct {
	UpdatedFields   primitive.M `bson:"updatedFields,omitempty"`
	RemovedFields   []string    `bson:"removedFields,omitempty"`
	TruncatedArrays []string    `bson:"truncatedArrays,omitempty"`
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// WebSocket message types
const (
	WSMessageTypeSync       = "sync"
	WSMessageTypeAck        = "ack"
	WSMessageTypePing       = "ping"
	WSMessageTypePong       = "pong"
	WSMessageTypeSubscribe  = "subscribe"
	WSMessageTypeUnsubscribe = "unsubscribe"
	WSMessageTypeError      = "error"
)

// SubscriptionRequest represents a client subscription request
type SubscriptionRequest struct {
	BucketID    string   `json:"bucketId"`
	Collections []string `json:"collections,omitempty"`
}

// DiffResult represents the result of comparing two document versions
type DiffResult struct {
	HasChanges    bool                   `json:"hasChanges"`
	AddedFields   map[string]interface{} `json:"addedFields,omitempty"`
	ModifiedFields map[string]interface{} `json:"modifiedFields,omitempty"`
	RemovedFields []string               `json:"removedFields,omitempty"`
}

// ConflictResolution represents how a conflict was resolved
type ConflictResolution struct {
	Strategy     string                 `json:"strategy"` // "lww", "merge", "manual"
	Winner       string                 `json:"winner"`   // "local" or "remote"
	MergedDoc    map[string]interface{} `json:"mergedDoc,omitempty"`
	ResolvedAt   time.Time              `json:"resolvedAt"`
}

// Checkpoint represents a point-in-time snapshot
type Checkpoint struct {
	ID            string    `json:"id" bson:"_id"`
	BucketID      string    `json:"bucketId" bson:"bucketId"`
	Collection    string    `json:"collection,omitempty" bson:"collection,omitempty"`
	FilePath      string    `json:"filePath" bson:"filePath"`
	FileSize      int64     `json:"fileSize" bson:"fileSize"`
	DocumentCount int64     `json:"documentCount" bson:"documentCount"`
	ResumeToken   string    `json:"resumeToken,omitempty" bson:"resumeToken,omitempty"`
	CreatedAt     time.Time `json:"createdAt" bson:"createdAt"`
}
