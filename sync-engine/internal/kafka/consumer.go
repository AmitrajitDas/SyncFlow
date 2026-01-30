package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/IBM/sarama"
	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Topics to consume
const (
	TopicUserCreated = "user.created"
)

// Consumer handles Kafka message consumption
type Consumer struct {
	consumerGroup sarama.ConsumerGroup
	config        *config.Config
	handlers      map[string]MessageHandler
}

// MessageHandler is a function that handles a message from a topic
type MessageHandler func(message []byte) error

// UserCreatedHandler handles user.created events
type UserCreatedHandler func(event *models.UserCreatedEvent) error

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg *config.Config) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	consumerGroup, err := sarama.NewConsumerGroup(
		cfg.GetKafkaServers(),
		cfg.Kafka.ConsumerGroup,
		saramaConfig,
	)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		consumerGroup: consumerGroup,
		config:        cfg,
		handlers:      make(map[string]MessageHandler),
	}, nil
}

// Close closes the Kafka consumer
func (c *Consumer) Close() error {
	return c.consumerGroup.Close()
}

// RegisterHandler registers a message handler for a topic
func (c *Consumer) RegisterHandler(topic string, handler MessageHandler) {
	c.handlers[topic] = handler
}

// RegisterUserCreatedHandler registers a handler for user.created events
func (c *Consumer) RegisterUserCreatedHandler(handler UserCreatedHandler) {
	c.handlers[TopicUserCreated] = func(message []byte) error {
		var event models.UserCreatedEvent
		if err := json.Unmarshal(message, &event); err != nil {
			return err
		}
		return handler(&event)
	}
}

// Start starts consuming messages from registered topics
func (c *Consumer) Start(ctx context.Context) error {
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}

	if len(topics) == 0 {
		log.Println("[Kafka] No topics registered, consumer not starting")
		return nil
	}

	handler := &consumerGroupHandler{
		handlers: c.handlers,
	}

	log.Printf("[Kafka] Starting consumer for topics: %v", topics)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Kafka] Consumer context cancelled, stopping")
			return ctx.Err()
		default:
			if err := c.consumerGroup.Consume(ctx, topics, handler); err != nil {
				log.Printf("[Kafka] Error consuming: %v", err)
			}
		}
	}
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	handlers map[string]MessageHandler
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	log.Println("[Kafka] Consumer group session setup")
	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Println("[Kafka] Consumer group session cleanup")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			log.Printf("[Kafka] Received message from topic=%s partition=%d offset=%d",
				message.Topic, message.Partition, message.Offset)

			if handler, exists := h.handlers[message.Topic]; exists {
				if err := handler(message.Value); err != nil {
					log.Printf("[Kafka] Error handling message from %s: %v", message.Topic, err)
					// Don't mark as consumed on error - will be retried
					continue
				}
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}
