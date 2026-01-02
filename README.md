# SyncFlow

A distributed real-time data synchronization system with conflict resolution, differential syncing, and WebSocket-based updates.

## Features

- **Real-time Sync**: WebSocket push updates to connected devices
- **Conflict Resolution**: Automatic Last-Write-Wins (LWW) strategy
- **Differential Sync**: Only transmit changed data
- **CDC-based**: MongoDB Change Streams for data capture
- **Multi-tenant**: Bucket-based data isolation
- **Checkpoint System**: Point-in-time recovery
- **$0/month**: 100% free architecture using Docker Compose

## Architecture

SyncFlow uses a microservices architecture with:

- **Auth Service** (Spring Boot) - JWT authentication and user management
- **Sync Engine** (Go) - CDC capture, conflict resolution, WebSocket server
- **API Gateway** (Spring Boot) - REST APIs for client operations
- **Infrastructure**: MongoDB, Kafka, Redis (all self-hosted)
- **Observability**: Prometheus, Grafana, Loki, Jaeger

## Quick Start

### Prerequisites

- Docker Desktop or Docker Engine (v24+)
- Docker Compose (v2.20+)
- 4GB+ available RAM
- 10GB+ available disk space

### Installation

1. **Clone the repository**

   ```bash
   git clone [repo-url](https://github.com/AmitrajitDas/SyncFlow)
   cd SyncFlow
   ```

2. **Initialize the environment**

   ```bash
   make init
   ```

   This will:

   - Create `.env` from `.env.example`
   - Start all infrastructure services
   - Initialize MongoDB replica set
   - Create Kafka topics

3. **Verify services are running**
   ```bash
   make status
   ```

### Access Services

Once running, access the following services:

- **Grafana**: http://localhost:3000 (admin/admin123)
- **Prometheus**: http://localhost:9091
- **Jaeger UI**: http://localhost:16686
- **Kafka UI**: http://localhost:9090
- **API Gateway**: http://localhost:8080 _(coming in Phase 5)_
- **Auth Service**: http://localhost:8081 _(coming in Phase 2)_
- **Sync Engine**: http://localhost:8082 _(coming in Phase 3)_

## Development

### Common Commands

```bash
make help           # Show all available commands
make up             # Start all services
make down           # Stop all services
make logs           # Tail logs from all services
make status         # Show service status
make health         # Check health of all services
make clean          # Remove all data (destructive!)
```

### Service-specific Logs

```bash
make logs-mongo     # MongoDB logs
make logs-kafka     # Kafka logs
make logs-redis     # Redis logs
```

### Database Access

```bash
make shell-mongo    # Open MongoDB shell
make shell-redis    # Open Redis CLI
make shell-kafka    # Open Kafka container shell
```

## Project Structure

```
SyncFlow/
├── docker-compose.yml              # Infrastructure orchestration
├── Makefile                        # Common commands
├── .env.example                    # Environment template
├── claude.md                       # Project context for AI
│
├── infrastructure/                 # Config files
│   ├── mongodb/
│   ├── kafka/
│   ├── prometheus/
│   ├── grafana/
│   ├── loki/
│   └── promtail/
│
├── auth-service/                   # Auth Service (Phase 2)
├── sync-engine/                    # Sync Engine (Phase 3)
└── api-gateway/                    # API Gateway (Phase 5)
```

## Technology Stack

### Infrastructure

- **Orchestration**: Docker Compose
- **Database**: MongoDB 7.0 (with replica set)
- **Message Queue**: Apache Kafka 7.5
- **Cache**: Redis 7.2
- **Metrics**: Prometheus
- **Visualization**: Grafana
- **Logging**: Loki + Promtail
- **Tracing**: Jaeger

### Services

- **Auth Service**: Spring Boot 3.x, Spring Security, JWT
- **Sync Engine**: Go 1.21+, Gorilla WebSocket
- **API Gateway**: Spring Boot 3.x, Spring Web MVC

## Configuration

### Environment Variables

Copy `.env.example` to `.env` and customize:

```bash
# MongoDB
MONGO_ROOT_USERNAME=admin
MONGO_ROOT_PASSWORD=admin123

# Redis
REDIS_PASSWORD=redis123

# Grafana
GRAFANA_USER=admin
GRAFANA_PASSWORD=admin123
```

### Kafka Topics

The following topics are auto-created:

- `user.created` - User registration events
- `data.changed` - Data change events from CDC
- `sync.events` - Sync operation events
- `checkpoint.created` - Checkpoint creation events
- `conflict.detected` - Conflict detection events

## Monitoring

### Grafana Dashboards

Access Grafana at http://localhost:3000 with credentials `admin/admin123`.

Datasources are pre-configured for:

- Prometheus (metrics)
- Loki (logs)
- Jaeger (traces)

### Prometheus Metrics

View raw metrics at http://localhost:9091/targets

### Jaeger Tracing

View distributed traces at http://localhost:16686

## Troubleshooting

### Services won't start

```bash
# Check logs
make logs

# Check individual service health
docker-compose ps
```

### MongoDB replica set not initialized

```bash
# Manually initialize
make init-mongo
```

### Kafka topics not created

```bash
# Manually create topics
make init-kafka

# List existing topics
make list-kafka-topics
```

### Clean restart

```bash
# Stop everything and remove volumes
make clean

# Restart from scratch
make init
```

## Contributing

This is a personal learning project. Feel free to fork and experiment!

## License

TBD

## Contact

**Developer**: Amitrajit Das

---

**Status**: Phase 1 Complete - Infrastructure Ready! 🚀
