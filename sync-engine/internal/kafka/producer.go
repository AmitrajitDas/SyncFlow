package kafka

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"
	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Topics
const (
	TopicDataChanged       = "data.changed"
	TopicSyncEvents        = "sync.events"
	TopicCheckpointCreated = "checkpoint.created"
	TopicConflictDetected  = "conflict.detected"
)

// Producer handles Kafka message production
type Producer struct {
	syncProducer sarama.SyncProducer
	config       *config.Config
}

// NewProducer creates a new Kafka producer
func NewProducer(cfg *config.Config) (*Producer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	saramaConfig.Producer.Retry.Max = 3
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Idempotent = true
	saramaConfig.Net.MaxOpenRequests = 1

	producer, err := sarama.NewSyncProducer(cfg.GetKafkaServers(), saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Producer{
		syncProducer: producer,
		config:       cfg,
	}, nil
}

// Close closes the Kafka producer
func (p *Producer) Close() error {
	return p.syncProducer.Close()
}

// PublishDataChanged publishes a data changed event
func (p *Producer) PublishDataChanged(event *models.DataChangedEvent) error {
	return p.publish(TopicDataChanged, event.DocumentID, event)
}

// PublishSyncEvent publishes a sync event
func (p *Producer) PublishSyncEvent(event *models.SyncEvent) error {
	return p.publish(TopicSyncEvents, event.BucketID, event)
}

// PublishCheckpointCreated publishes a checkpoint created event
func (p *Producer) PublishCheckpointCreated(event *models.CheckpointCreatedEvent) error {
	return p.publish(TopicCheckpointCreated, event.CheckpointID, event)
}

// PublishConflictDetected publishes a conflict detected event
func (p *Producer) PublishConflictDetected(event *models.ConflictDetectedEvent) error {
	return p.publish(TopicConflictDetected, event.DocumentID, event)
}

// publish sends a message to a Kafka topic
func (p *Producer) publish(topic, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
	}

	partition, offset, err := p.syncProducer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to send message to %s: %w", topic, err)
	}

	log.Printf("[Kafka] Message sent to topic=%s partition=%d offset=%d key=%s",
		topic, partition, offset, key)

	return nil
}

// PublishBatch publishes multiple messages to a topic
func (p *Producer) PublishBatch(topic string, messages []struct {
	Key   string
	Value interface{}
}) error {
	var producerMessages []*sarama.ProducerMessage

	for _, msg := range messages {
		data, err := json.Marshal(msg.Value)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		producerMessages = append(producerMessages, &sarama.ProducerMessage{
			Topic:     topic,
			Key:       sarama.StringEncoder(msg.Key),
			Value:     sarama.ByteEncoder(data),
			Timestamp: time.Now(),
		})
	}

	return p.syncProducer.SendMessages(producerMessages)
}
