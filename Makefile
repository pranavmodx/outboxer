.PHONY: infra-up infra-down run-order-service run-relay-service run-notification-service test-order debezium-up debezium-down debezium-register build stop

# ── Infrastructure ─────────────────────────────────────────────────
infra-up:
	docker-compose up -d --remove-orphans
	@echo "⏳ Waiting for services to be healthy..."
	@bash -c 'until [ "$$(docker inspect -f {{.State.Health.Status}} outbox-kafka-1)" == "healthy" ]; do \
		if [ "$$(docker inspect -f {{.State.Status}} outbox-kafka-1)" == "exited" ]; then \
			echo "❌ Kafka failed to start. Logs:"; \
			docker logs outbox-kafka-1; \
			exit 1; \
		fi; \
		printf "."; sleep 2; \
	done'
	@echo "\n✅ Infrastructure is up (postgres, kafka, redis)"

infra-down:
	docker-compose down -v

# ── Build ──────────────────────────────────────────────────────────
build:
	go build -o bin/order-service   ./cmd/order-service
	go build -o bin/relay           ./cmd/relay
	go build -o bin/notification    ./cmd/notification-service

# ── Services ───────────────────────────────────────────────────────
# Set BROKER_TYPE=kafka (default) or BROKER_TYPE=redis before running.

run-order-service:
	@bash -c "trap 'kill 0' EXIT; go run ./cmd/order-service"

run-relay-service:
	@bash -c "trap 'kill 0' EXIT; BROKER_TYPE=$(or $(BROKER_TYPE),kafka) go run ./cmd/relay"

run-notification-service:
	@bash -c "trap 'kill 0' EXIT; BROKER_TYPE=$(or $(BROKER_TYPE),kafka) TOPIC=order.created go run ./cmd/notification-service"

stop:
	@echo "🛑 Stopping all services..."
	-@pkill -f "go run ./cmd/" || true
	-@pkill -f "bin/order-service" || true
	-@pkill -f "bin/relay" || true
	-@pkill -f "bin/notification" || true
	-@lsof -ti:8080,8081,8082 | xargs kill -9 2>/dev/null || true
	@echo "✅ Services stopped."

# ── Testing ────────────────────────────────────────────────────────
test-order:
	@bash scripts/test-order.sh

# ── Debezium CDC (Kafka) ───────────────────────────────────────────
debezium-kafka-up:
	docker-compose -f docker-compose.yml -f docker-compose.debezium.yml up -d
	@echo "⏳ Waiting for Debezium Connect (Kafka)..."
	@sleep 15
	@echo "✅ Debezium Connect is ready"

debezium-kafka-down:
	docker-compose -f docker-compose.yml -f docker-compose.debezium.yml down -v

debezium-kafka-register:
	@bash scripts/register-debezium.sh

# ── Debezium CDC (Redis) ───────────────────────────────────────────
debezium-redis-up:
	docker-compose -f docker-compose.yml -f docker-compose.debezium-redis.yml up -d
	@echo "⏳ Waiting for Debezium Server (Redis)..."
	@sleep 5
	@echo "✅ Debezium Server is ready"

debezium-redis-down:
	docker-compose -f docker-compose.yml -f docker-compose.debezium-redis.yml down -v
