# SyncFlow - Project Context for Claude

**Last Updated:** 2025-12-23
**Current Phase:** Phase 1 - COMPLETE ✅ | Ready for Phase 2
**Project Status:** Infrastructure Deployed & Operational

---

## Project Overview

**SyncFlow** is a distributed real-time data synchronization system designed to handle multi-device state synchronization with conflict resolution, differential syncing, and real-time updates via WebSocket.

### Key Features
- **Real-time Sync**: WebSocket-based push updates to connected devices
- **Conflict Resolution**: Automatic conflict detection and resolution using Last-Write-Wins (LWW)
- **Differential Sync**: Only transmit changed data, not full snapshots
- **CDC-based**: MongoDB Change Streams for capturing database changes
- **Multi-tenant**: Bucket-based isolation for different data domains
- **Checkpoint System**: Point-in-time recovery and historical state snapshots
- **Free Tier Architecture**: $0/month operational cost using Docker Compose

---

## Architecture Philosophy

### Cost Optimization: $0/month Approach
The entire system runs **100% free** using:
- **Docker Compose** for local orchestration (no Kubernetes)
- **Self-hosted infrastructure** (MongoDB, Kafka, Redis) in containers
- **No cloud services** required (optional: AWS S3 Free Tier for checkpoints)
- **Single machine deployment** - can run on laptop or free-tier EC2

### Previous Architecture (Rejected - $2,500/month)
- AWS EKS (Kubernetes): $580/month
- AWS MSK (Managed Kafka): $720/month
- AWS ElastiCache (Redis): $450/month
- MongoDB Atlas M30: $580/month
- Total: **$2,500/month** → Replaced with **$0/month** Docker Compose solution

---

## Tech Stack

### Infrastructure
- **Orchestration**: Docker Compose
- **Message Queue**: Apache Kafka + Zookeeper (self-hosted)
- **Cache**: Redis (self-hosted)
- **Database**: MongoDB Community Edition with replica set (self-hosted)
- **Storage**: Local filesystem + Optional AWS S3 Free Tier (5GB)

### Services
1. **Auth Service** - Spring Boot (Java)
   - JWT authentication and authorization
   - User management and bucket assignment
   - Kafka integration for event publishing

2. **Sync Engine Service** - Go
   - MongoDB Change Stream CDC capture
   - Differential sync calculation
   - WebSocket server for real-time updates
   - Conflict resolution engine
   - Checkpoint generation

3. **API Gateway Service** - Spring Boot (Java)
   - REST API for all client operations
   - Device management
   - Subscription management
   - Checkpoint retrieval
   - OpenAPI/Swagger documentation

### Observability Stack
- **Prometheus**: Metrics collection
- **Grafana**: Dashboards and visualization
- **Loki**: Log aggregation
- **Jaeger**: Distributed tracing

---

## Service Architecture

### 1. Auth Service (Spring Boot)
**Port**: 8081
**Responsibilities**:
- User registration and login
- JWT token generation and validation
- Bucket assignment for new users
- Publishing user events to Kafka

**Tech**:
- Spring Boot 3.x
- Spring Security + JWT
- Spring Kafka
- PostgreSQL/MongoDB for user data

**Endpoints**:
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - User login (returns JWT)
- `POST /api/auth/refresh` - Token refresh
- `GET /api/auth/me` - Get current user info

---

### 2. Sync Engine Service (Go)
**Port**: 8082 (HTTP), 8083 (WebSocket)
**Responsibilities**:
- Monitor MongoDB Change Streams for data changes
- Calculate differential syncs (only changed fields)
- Detect and resolve conflicts using LWW strategy
- Broadcast changes to connected WebSocket clients
- Generate checkpoints for point-in-time recovery
- Store checkpoints in S3 or local storage

**Tech**:
- Go 1.21+
- Gorilla WebSocket
- MongoDB Go Driver
- Kafka Go Client (Sarama/Confluent)
- AWS SDK for S3 (optional)

**Key Components**:
- **CDC Listener**: Monitors `change_stream` collection
- **Sync Calculator**: Computes diffs between states
- **Conflict Resolver**: LWW based on timestamp
- **WebSocket Manager**: Connection pool and broadcasting
- **Checkpoint Generator**: Async checkpoint creation

---

### 3. API Gateway Service (Spring Boot)
**Port**: 8080
**Responsibilities**:
- Expose REST APIs for device and subscription management
- Handle checkpoint requests
- Route requests to appropriate backend services
- API documentation via Swagger

**Tech**:
- Spring Boot 3.x
- Spring Web MVC
- SpringDoc OpenAPI (Swagger)
- Spring Cloud Gateway (optional)

**Endpoints**:
- `POST /api/devices` - Register device
- `GET /api/devices/:id` - Get device info
- `POST /api/subscriptions` - Create subscription
- `GET /api/buckets/:id/data` - Get bucket data
- `GET /api/checkpoints/:id` - Download checkpoint

---

## Data Flow

### 1. User Registration Flow
```
Client → API Gateway → Auth Service → Kafka (user.created) → Sync Engine → Assign Bucket
```

### 2. Data Change Flow
```
App → MongoDB Write → Change Stream → Sync Engine → Calculate Diff → Kafka → WebSocket Broadcast
```

### 3. Real-time Sync Flow
```
Sync Engine (CDC) → Detect Change → Resolve Conflicts → Publish to Kafka → WebSocket Push to Devices
```

### 4. Checkpoint Flow
```
Client Request → API Gateway → Sync Engine → Generate Checkpoint → S3/Local Storage → Return URL
```

---

## Key Design Decisions

### 1. Why Docker Compose over Kubernetes?
- **Cost**: Free vs $580/month for EKS
- **Simplicity**: No complex YAML manifests
- **Learning**: Easier for single developer
- **Portability**: Runs anywhere with Docker

### 2. Why Self-hosted Kafka over AWS MSK?
- **Cost**: Free vs $720/month
- **Control**: Full access to configuration
- **Learning**: Better understanding of Kafka internals

### 3. Why Go for Sync Engine?
- **Performance**: Goroutines for concurrent WebSocket connections
- **Efficiency**: Low memory footprint for CDC processing
- **Built-in Concurrency**: Perfect for real-time systems

### 4. Why Spring Boot for Auth/Gateway?
- **Ecosystem**: Rich libraries for JWT, Kafka, REST
- **Maturity**: Battle-tested for enterprise APIs
- **Developer Experience**: Excellent tooling and documentation

### 5. Conflict Resolution Strategy: Last-Write-Wins (LWW)
- **Simplicity**: Easy to implement and reason about
- **Deterministic**: Same result across all nodes
- **Timestamp-based**: Uses `updated_at` field
- **Trade-off**: May lose concurrent updates (acceptable for MVP)

---

## Development Phases

### Phase 1: Infrastructure Setup ✅ COMPLETE
- [x] Todo list created
- [x] Create `docker-compose.yml` with all services (10 containers)
- [x] Create `.env.example` and `Makefile` (20+ commands)
- [x] Write init scripts (MongoDB replica set, Kafka topics)
- [x] MongoDB replica set with keyfile authentication
- [x] 5 Kafka topics created
- [x] Observability stack configured (Prometheus, Grafana, Loki, Jaeger)
- [ ] Optional: Set up AWS S3 free tier (deferred to Phase 3)

### Phase 2: Auth Service
- [ ] Spring Boot project setup
- [ ] JWT authentication implementation
- [ ] Bucket assignment with Kafka
- [ ] Dockerfile for Auth Service

### Phase 3: Sync Engine Service
- [ ] Go project setup
- [ ] MongoDB Change Stream CDC
- [ ] Differential sync + conflict resolution
- [ ] Checkpoint generation

### Phase 4: WebSocket Implementation
- [ ] WebSocket server in Sync Engine
- [ ] Connection manager
- [ ] Message broadcasting
- [ ] Dockerfile for Sync Engine

### Phase 5: API Gateway
- [ ] Spring Boot project setup
- [ ] REST controllers (Device, Subscription, Bucket, Checkpoint)
- [ ] OpenAPI/Swagger docs
- [ ] Dockerfile for API Gateway

### Phase 6: Observability & Launch
- [ ] Configure Prometheus, Grafana, Loki, Jaeger
- [ ] Create Grafana dashboards
- [ ] Unit and integration tests
- [ ] GitHub Actions CI/CD
- [ ] Documentation (README, setup guide, API docs)

---

## Current Status

**Active Phase**: Phase 1 COMPLETE ✅
**Next Phase**: Phase 2 - Auth Service Development
**Infrastructure**: All 10 services running and healthy
**Blockers**: None
**Last Activity**: Phase 1 completion - All infrastructure operational (2025-12-23)

### What's Working Now:
- ✅ MongoDB 7.0 with replica set (Change Streams enabled)
- ✅ Kafka + Zookeeper (5 topics created)
- ✅ Redis (caching layer)
- ✅ Prometheus + Grafana (monitoring)
- ✅ Loki + Promtail (log aggregation)
- ✅ Jaeger (distributed tracing)
- ✅ Kafka UI (web interface)

### Access URLs:
- Grafana: http://localhost:3000 (admin/admin123)
- Prometheus: http://localhost:9091
- Jaeger: http://localhost:16686
- Kafka UI: http://localhost:9090

---

## Quick Start

Infrastructure is ready! Start the system with:

```bash
# Clone repository
git clone <repo-url>
cd SyncFlow

# Copy environment file
cp .env.example .env

# Start all services (or use: make init for full initialization)
docker compose up -d

# Initialize MongoDB replica set (first time only)
docker compose exec mongodb bash /docker-entrypoint-initdb.d/init-replica-set.sh

# View logs
docker compose logs -f

# Check status
docker compose ps
```

### Access Services:
- **Grafana**: http://localhost:3000 (admin/admin123)
- **Prometheus**: http://localhost:9091
- **Jaeger UI**: http://localhost:16686
- **Kafka UI**: http://localhost:9090
- **MongoDB**: mongodb://admin:admin123@localhost:27017
- **Redis**: localhost:6379 (password: redis123)
- **Kafka**: localhost:9092

### Coming in Future Phases:
- API Gateway: http://localhost:8080 (Phase 5)
- Auth Service: http://localhost:8081 (Phase 2)
- Sync Engine: http://localhost:8082 (Phase 3)

---

## Project Structure (Planned)

```
SyncFlow/
├── docker-compose.yml              # All infrastructure + services
├── .env.example                    # Environment variables template
├── Makefile                        # Common commands
├── README.md                       # User-facing documentation
├── claude.md                       # This file (Claude context)
│
├── auth-service/                   # Spring Boot Auth Service
│   ├── src/main/java/
│   ├── Dockerfile
│   └── pom.xml
│
├── sync-engine/                    # Go Sync Engine Service
│   ├── cmd/
│   ├── internal/
│   ├── Dockerfile
│   └── go.mod
│
├── api-gateway/                    # Spring Boot API Gateway
│   ├── src/main/java/
│   ├── Dockerfile
│   └── pom.xml
│
├── infrastructure/                 # Init scripts and configs
│   ├── mongodb/
│   │   └── init-replica-set.js
│   ├── kafka/
│   │   └── create-topics.sh
│   └── prometheus/
│       └── prometheus.yml
│
├── observability/                  # Monitoring configs
│   ├── grafana/
│   │   └── dashboards/
│   └── jaeger/
│
└── docs/                          # Additional documentation
    ├── api-reference.md
    ├── architecture.md
    └── deployment.md
```

---

## Important Notes for Future Sessions

1. **Always check this file first** when starting a new chat
2. **Update Current Status section** after major milestones
3. **Add new design decisions** as they're made
4. **Keep tech stack up-to-date** if dependencies change
5. **This is a $0/month project** - no cloud services unless free tier

---

## References

- **MongoDB Change Streams**: https://docs.mongodb.com/manual/changeStreams/
- **Kafka Docker Setup**: https://github.com/confluentinc/cp-docker-images
- **Spring Boot + Kafka**: https://spring.io/projects/spring-kafka
- **Go WebSocket (Gorilla)**: https://github.com/gorilla/websocket
- **Docker Compose**: https://docs.docker.com/compose/

---

## Contact & Collaboration

**Developer**: Amitrajit Das
**Project Location**: `/Users/amitrajitdas31/Developer/Coding/Dev/Projects/SyncFlow`
**Git Status**: Not yet initialized (recommend initializing before Phase 2)
**License**: TBD

---

## Phase 1 Completion Summary

### Infrastructure Deployed (2025-12-23):
- **10 Docker containers** running with health checks
- **MongoDB replica set** initialized with keyfile authentication
- **5 Kafka topics** created (user.created, data.changed, sync.events, checkpoint.created, conflict.detected)
- **Observability stack** fully configured
- **$0/month cost** - 100% self-hosted

### Files Created:
- `docker-compose.yml` (329 lines)
- `.env` + `.env.example`
- `Makefile` (20+ commands)
- `README.md` (comprehensive docs)
- `.gitignore` (secure configuration)
- `infrastructure/` config files (MongoDB, Kafka, Prometheus, Grafana, Loki, Promtail)

### Issues Resolved:
- MongoDB replica set keyfile authentication
- Docker Compose version warnings
- Kafka topic creation
- Network configuration

---

**End of Context Document**
