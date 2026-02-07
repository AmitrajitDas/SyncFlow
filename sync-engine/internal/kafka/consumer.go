package kafka

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/syncflow/sync-engine/internal/config"
	"github.com/syncflow/sync-engine/pkg/models"
)

// Topics to consume (some are also defined in producer.go)
const (
	TopicUserCreated = "user.created"
)

// Message represents a Kafka message
type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Consumer handles Kafka message consumption
type Consumer struct {
	consumerGroup    sarama.ConsumerGroup
	config           *config.Config
	handlers         map[string]MessageHandler
	messageChannel   chan *Message
	subscribedTopics []string
	channelStartOnce sync.Once
	channelCtx       context.Context
	channelCancel    context.CancelFunc
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
		consumerGroup:    consumerGroup,
		config:           cfg,
		handlers:         make(map[string]MessageHandler),
		messageChannel:   make(chan *Message, 1000), // Buffered channel for messages
		subscribedTopics: []string{},
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

// StartChannelConsumer starts consuming messages and sends them to the internal channel
// This is used for channel-based consumption (e.g., WebSocket hub)
func (c *Consumer) StartChannelConsumer(ctx context.Context, topics []string) error {
	if len(topics) == 0 {
		log.Println("[Kafka] No topics specified for channel consumer")
		return nil
	}

	c.subscribedTopics = topics

	handler := &channelConsumerHandler{
		messageChannel: c.messageChannel,
	}

	log.Printf("[Kafka] Starting channel consumer for topics: %v", topics)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Kafka] Channel consumer context cancelled, stopping")
			close(c.messageChannel)
			return ctx.Err()
		default:
			if err := c.consumerGroup.Consume(ctx, topics, handler); err != nil {
				log.Printf("[Kafka] Error in channel consumer: %v", err)
			}
		}
	}
}

// ConsumeMessages polls for messages from subscribed topics with a timeout
// Returns messages that were received within the timeout period
// Automatically starts the channel consumer on first call
func (c *Consumer) ConsumeMessages(ctx context.Context, topics []string, timeout time.Duration) ([]*Message, error) {
	// Start channel consumer on first call
	c.channelStartOnce.Do(func() {
		c.channelCtx, c.channelCancel = context.WithCancel(context.Background())
		go func() {
			if err := c.StartChannelConsumer(c.channelCtx, topics); err != nil {
				log.Printf("[Kafka] Channel consumer stopped: %v", err)
			}
		}()
	})

	messages := make([]*Message, 0)
	deadline := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return messages, ctx.Err()
		case <-deadline:
			return messages, nil
		case msg, ok := <-c.messageChannel:
			if !ok {
				return messages, nil
			}
			// Filter by requested topics
			if c.isTopicRequested(msg.Topic, topics) {
				messages = append(messages, msg)
			}
		}
	}
}

// isTopicRequested checks if a topic is in the requested topics list
func (c *Consumer) isTopicRequested(topic string, requestedTopics []string) bool {
	for _, t := range requestedTopics {
		if t == topic {
			return true
		}
	}
	return false
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

// channelConsumerHandler implements sarama.ConsumerGroupHandler for channel-based consumption
type channelConsumerHandler struct {
	messageChannel chan *Message
}

func (h *channelConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	log.Println("[Kafka] Channel consumer group session setup")
	return nil
}

func (h *channelConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Println("[Kafka] Channel consumer group session cleanup")
	return nil
}

func (h *channelConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			log.Printf("[Kafka] Channel consumer received message from topic=%s partition=%d offset=%d",
				message.Topic, message.Partition, message.Offset)

			// Send message to channel (non-blocking)
			msg := &Message{
				Topic:     message.Topic,
				Key:       message.Key,
				Value:     message.Value,
				Partition: message.Partition,
				Offset:    message.Offset,
				Timestamp: message.Timestamp,
			}

			select {
			case h.messageChannel <- msg:
				// Message sent successfully
				session.MarkMessage(message, "")
			default:
				// Channel full, log and skip
				log.Printf("[Kafka] Message channel full, dropping message from %s", message.Topic)
			}

		case <-session.Context().Done():
			return nil
		}
	}
}
