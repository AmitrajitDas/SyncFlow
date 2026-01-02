# SyncFlow - Makefile for Common Operations
# Run 'make help' to see all available commands

.PHONY: help setup up down restart logs clean test init-kafka init-mongo status ps

# Default target
.DEFAULT_GOAL := help

# ============================================
# Configuration
# ============================================
DOCKER_COMPOSE := docker compose
PROJECT_NAME := syncflow

# ============================================
# Help
# ============================================
help: ## Show this help message
	@echo "SyncFlow - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ============================================
# Setup & Initialization
# ============================================
setup: ## Initial setup - create .env file from template
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "✅ Created .env file from .env.example"; \
		echo "⚠️  Please update .env with your configuration"; \
	else \
		echo "⚠️  .env file already exists, skipping..."; \
	fi

init: setup ## Initialize everything (setup + start services)
	@echo "🚀 Starting SyncFlow infrastructure..."
	$(DOCKER_COMPOSE) up -d
	@echo "⏳ Waiting for services to be healthy..."
	@sleep 20
	@$(MAKE) init-mongo
	@$(MAKE) init-kafka
	@echo "✅ SyncFlow initialization complete!"
	@$(MAKE) status

# ============================================
# Service Management
# ============================================
up: ## Start all services
	@echo "🚀 Starting all services..."
	$(DOCKER_COMPOSE) up -d
	@echo "✅ Services started!"

down: ## Stop all services
	@echo "🛑 Stopping all services..."
	$(DOCKER_COMPOSE) down
	@echo "✅ Services stopped!"

restart: ## Restart all services
	@echo "🔄 Restarting all services..."
	$(DOCKER_COMPOSE) restart
	@echo "✅ Services restarted!"

stop: ## Stop all services (alias for down)
	@$(MAKE) down

start: ## Start all services (alias for up)
	@$(MAKE) up

# ============================================
# Monitoring & Logs
# ============================================
logs: ## Tail logs from all services
	$(DOCKER_COMPOSE) logs -f

logs-mongo: ## Tail MongoDB logs
	$(DOCKER_COMPOSE) logs -f mongodb

logs-kafka: ## Tail Kafka logs
	$(DOCKER_COMPOSE) logs -f kafka

logs-redis: ## Tail Redis logs
	$(DOCKER_COMPOSE) logs -f redis

logs-prometheus: ## Tail Prometheus logs
	$(DOCKER_COMPOSE) logs -f prometheus

logs-grafana: ## Tail Grafana logs
	$(DOCKER_COMPOSE) logs -f grafana

status: ## Show status of all services
	@echo "📊 SyncFlow Service Status:"
	@echo ""
	$(DOCKER_COMPOSE) ps
	@echo ""
	@echo "🌐 Service URLs:"
	@echo "  API Gateway:     http://localhost:8080 (coming in Phase 5)"
	@echo "  Auth Service:    http://localhost:8081 (coming in Phase 2)"
	@echo "  Sync Engine:     http://localhost:8082 (coming in Phase 3)"
	@echo "  WebSocket:       ws://localhost:8083 (coming in Phase 4)"
	@echo "  Grafana:         http://localhost:3000 (admin/admin123)"
	@echo "  Prometheus:      http://localhost:9091"
	@echo "  Jaeger UI:       http://localhost:16686"
	@echo "  Kafka UI:        http://localhost:9090"

ps: ## Show running containers (alias for status)
	$(DOCKER_COMPOSE) ps

# ============================================
# Infrastructure Initialization
# ============================================
init-mongo: ## Initialize MongoDB replica set
	@echo "🗄️  Initializing MongoDB replica set..."
	@sleep 5
	$(DOCKER_COMPOSE) exec -T mongodb bash /docker-entrypoint-initdb.d/init-replica-set.sh || true
	@echo "✅ MongoDB replica set initialized!"

init-kafka: ## Create Kafka topics
	@echo "📨 Creating Kafka topics..."
	@sleep 5
	@$(DOCKER_COMPOSE) exec -T kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic user.created --partitions 3 --replication-factor 1
	@$(DOCKER_COMPOSE) exec -T kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic data.changed --partitions 3 --replication-factor 1
	@$(DOCKER_COMPOSE) exec -T kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic sync.events --partitions 3 --replication-factor 1
	@$(DOCKER_COMPOSE) exec -T kafka kafka-topics --bootstrap-server localhost:9092 --create --if-not-exists --topic checkpoint.created --partitions 1 --replication-factor 1
	@echo "✅ Kafka topics created!"

list-kafka-topics: ## List all Kafka topics
	$(DOCKER_COMPOSE) exec kafka kafka-topics --bootstrap-server localhost:9092 --list

# ============================================
# Cleanup
# ============================================
clean: ## Stop services and remove volumes (destructive!)
	@echo "⚠️  WARNING: This will delete all data!"
	@read -p "Are you sure? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	@echo "🧹 Cleaning up..."
	$(DOCKER_COMPOSE) down -v
	@echo "✅ Cleanup complete!"

clean-all: clean ## Complete cleanup including images
	@echo "🧹 Removing Docker images..."
	docker images | grep $(PROJECT_NAME) | awk '{print $$3}' | xargs -r docker rmi -f
	@echo "✅ Complete cleanup finished!"

# ============================================
# Development
# ============================================
shell-mongo: ## Open MongoDB shell
	$(DOCKER_COMPOSE) exec mongodb mongosh -u admin -p admin123 --authenticationDatabase admin

shell-redis: ## Open Redis CLI
	$(DOCKER_COMPOSE) exec redis redis-cli -a redis123

shell-kafka: ## Open Kafka container shell
	$(DOCKER_COMPOSE) exec kafka bash

# ============================================
# Health Checks
# ============================================
health: ## Check health of all services
	@echo "🏥 Checking service health..."
	@echo ""
	@echo "MongoDB:"
	@$(DOCKER_COMPOSE) exec -T mongodb mongosh --quiet --eval "db.adminCommand('ping')" 2>/dev/null && echo "  ✅ Healthy" || echo "  ❌ Unhealthy"
	@echo ""
	@echo "Kafka:"
	@$(DOCKER_COMPOSE) exec -T kafka kafka-broker-api-versions --bootstrap-server localhost:9092 >/dev/null 2>&1 && echo "  ✅ Healthy" || echo "  ❌ Unhealthy"
	@echo ""
	@echo "Redis:"
	@$(DOCKER_COMPOSE) exec -T redis redis-cli -a redis123 ping 2>/dev/null | grep -q PONG && echo "  ✅ Healthy" || echo "  ❌ Unhealthy"
	@echo ""
	@echo "Prometheus:"
	@curl -s http://localhost:9091/-/healthy >/dev/null 2>&1 && echo "  ✅ Healthy" || echo "  ❌ Unhealthy"
	@echo ""
	@echo "Grafana:"
	@curl -s http://localhost:3000/api/health >/dev/null 2>&1 && echo "  ✅ Healthy" || echo "  ❌ Unhealthy"
	@echo ""

# ============================================
# Testing (to be added in Phase 6)
# ============================================
test: ## Run all tests (placeholder)
	@echo "⚠️  Tests not yet implemented (Phase 6)"

# ============================================
# Documentation
# ============================================
docs: ## Open documentation in browser
	@echo "📚 Opening documentation..."
	@open http://localhost:3000 || xdg-open http://localhost:3000 || echo "Please visit http://localhost:3000"
