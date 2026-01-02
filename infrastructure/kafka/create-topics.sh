#!/bin/bash
# Kafka Topics Initialization Script

echo "Waiting for Kafka to be ready..."
sleep 15

KAFKA_BROKER="kafka:29092"

echo "Creating Kafka topics for SyncFlow..."

# User events topic
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create --if-not-exists \
  --topic user.created \
  --partitions 3 \
  --replication-factor 1 \
  --config retention.ms=604800000 \
  && echo "✅ Created topic: user.created"

# Data change events topic
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create --if-not-exists \
  --topic data.changed \
  --partitions 3 \
  --replication-factor 1 \
  --config retention.ms=604800000 \
  && echo "✅ Created topic: data.changed"

# Sync events topic
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create --if-not-exists \
  --topic sync.events \
  --partitions 3 \
  --replication-factor 1 \
  --config retention.ms=604800000 \
  && echo "✅ Created topic: sync.events"

# Checkpoint created topic
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create --if-not-exists \
  --topic checkpoint.created \
  --partitions 1 \
  --replication-factor 1 \
  --config retention.ms=2592000000 \
  && echo "✅ Created topic: checkpoint.created"

# Conflict resolution topic
kafka-topics --bootstrap-server $KAFKA_BROKER \
  --create --if-not-exists \
  --topic conflict.detected \
  --partitions 3 \
  --replication-factor 1 \
  --config retention.ms=604800000 \
  && echo "✅ Created topic: conflict.detected"

echo ""
echo "📋 Listing all topics:"
kafka-topics --bootstrap-server $KAFKA_BROKER --list

echo ""
echo "✅ Kafka topics initialization complete!"
