# Transactional Outbox Pattern — Event Relay in Go

A learning-focused project demonstrating the **Transactional Outbox Pattern** using a dedicated **Relay Service** (Polling) and CDC (**Debezium**), with two microservices and pluggable message brokers (**Kafka** / **Redis Streams**).

## Architecture

```mermaid
graph LR
    subgraph Producer ["📦 Order Service"]
        direction TB
        App([Go Application])
        subgraph DB ["🗄️ PostgreSQL"]
            direction LR
            Orders[(Orders Table)]
            Outbox[(Outbox Table)]
            WAL{WAL Log}
        end
    end

    App -- "(1) Business Data" --> Orders
    App -- "(2) Event Data" --> Outbox
    Orders -.-|"Atomic Transaction"| Outbox
    Outbox -.-> WAL

    subgraph Strategies ["⚡ Relay Strategies"]
        direction TB
        PR[[Go Polling Relay]]
        DZ[[Debezium CDC]]
    end

    PR -- "SQL Select" --> Outbox
    DZ -- "Replication Protocol" --> WAL

    subgraph Broker ["📡 Message Brokers"]
        direction TB
        Kafka[[Apache Kafka]]
        Redis[[Redis Streams]]
    end

    PR -- "Publishes" --> Kafka
    PR -- "Publishes" --> Redis
    DZ -- "Streams" --> Kafka
    DZ -- "Streams" --> Redis

    Consumer([🔔 Notification Service])

    Kafka -- "Consumes" --> Consumer
    Redis -- "Consumes" --> Consumer

    %% Premium Styling
    classDef producer fill:#f5f3ff,stroke:#7c3aed,stroke-width:2px,color:#2e1065
    classDef strategy fill:#fff7ed,stroke:#ea580c,stroke-width:2px,color:#7c2d12
    classDef broker fill:#ecfdf5,stroke:#059669,stroke-width:2px,color:#064e3b
    classDef consumer fill:#f0fdf4,stroke:#16a34a,stroke-width:2px,color:#14532d
    
    class Producer producer
    class Strategies strategy
    class Broker broker
    class Consumer consumer
```

### The Outbox Pattern

The problem: You need to update a database **and** send an event to a message broker atomically. If you do them sequentially, one can fail and leave the system inconsistent.

The solution: Write the event to an **outbox table** in the **same database transaction** as the business data. A separate process (relay or CDC) reads the outbox and publishes to the broker.

**Guarantees:**
- **At-least-once delivery** — events are never lost (they're in the DB)
- **Eventual consistency** — the relay/CDC will eventually publish all events
- **No distributed transactions** — everything stays within a single DB transaction

### Two Relay Strategies

| Strategy | How it works | Pros | Cons |
|---|---|---|---|
| **Polling Relay** | Periodically queries unpublished outbox rows | Simple, works with any broker | Slight delay (poll interval), DB load |
| **Debezium CDC** | Streams the DB WAL (write-ahead log) to Kafka | Near real-time, no polling | Kafka-only, more infrastructure |

### Two Brokers

| Broker | Durability | Use case |
|---|---|---|
| **Kafka** | Durable log, consumer groups, replay | Production-grade event streaming |
| **Redis Streams** | Durable event log, consumer groups | Persistent alternative to Kafka |

---

## Quick Start

### Prerequisites
- Go 1.21+
- Docker & Docker Compose
- `jq` (for script output formatting)

### 1. Start Infrastructure

```bash
make infra-up
```

This starts PostgreSQL, Kafka (+ Zookeeper), and Redis.

### 2. Build the Project

```bash
make build
```

This compiles the three Go services into the `bin/` directory.

### 3. Start the Order Service

```bash
make run-order-service
```

### 3. Start the Relay + Notification Service

**With Kafka (default):**
```bash
# Terminal 1
make run-relay-service

# Terminal 2
make run-notification-service
```

**With Redis Streams:**
```bash
# Terminal 1
BROKER_TYPE=redis make run-relay-service

# Terminal 2
BROKER_TYPE=redis make run-notification-service
```

### 4. Place a Test Order

```bash
make test-order
```

You should see:
- Order Service logs: `✅ Order <id> created with outbox event <id>`
- Relay logs: `📤 Relayed 1 event(s)`
- Notification Service logs: `📬 NEW ORDER NOTIFICATION` with order details

### 5. Inspect the Outbox Table

```bash
docker exec -it $(docker ps -qf name=postgres) psql -U outbox -c \
  "SELECT id, event_type, status, created_at FROM outbox_events ORDER BY created_at DESC LIMIT 5;"
```

### 6. Inspect Redis Streams (if using Redis)

```bash
docker exec -it $(docker ps -qf name=redis) redis-cli XREVRANGE order.created + -  COUNT 5
```

---

## CDC with Debezium

Debezium watches the PostgreSQL WAL and streams outbox table changes directly to a message broker — no polling relay needed.

This project supports two CDC architectures:
1. **Kafka Sink (Debezium Connect)**: The classic architecture using Kafka Connect.
2. **Redis Sink (Debezium Server)**: A Kafka-less architecture using Debezium Server.

### Option A: CDC with Kafka

### 1. Start Debezium Connect
```bash
make debezium-kafka-up
```

### 2. Register the Connector
```bash
make debezium-kafka-register
```

### 3. Consume via Kafka
```bash
make run-notification-service   # BROKER_TYPE defaults to kafka
```

### Option B: CDC with Redis

### 1. Start Debezium Server
```bash
make debezium-redis-up
```
*(No need to register a connector, Debezium Server auto-starts based on its `application.properties`)*

### 2. Consume via Redis
```bash
BROKER_TYPE=redis make run-notification-service
```

### 4. Place an Order (no relay needed!)

```bash
make test-order
```

Debezium captures the INSERT from the WAL and routes it to the correct topic/stream (`order.created`).

---

### 2. Dual-Layer Idempotency (Exactly-Once)
Because the system guarantees **At-Least-Once** delivery, the Notification Service ensures **Exactly-Once** processing using two checks:

1.  **Technical Check (`event_id`)**: Prevents processing the same broker message twice (e.g., if Kafka retries).
2.  **Business Check (`aggregate_id` + `event_type`)**: Prevents processing duplicate business actions (e.g., if a user accidentally creates two "Order Created" events for the same order).

Both checks are performed atomically within a single database transaction (the **Inbox Pattern**). 

#### **Exactly-Once vs. At-Least-Once**
- **Outbox Pattern (Producer-side)**: Guarantees **At-Least-Once** delivery. The message will eventually be sent, but retries might cause duplicates.
- **Inbox Pattern (Consumer-side)**: Guarantees **Exactly-Once** processing. By keeping an "Inbox" table of processed IDs, the consumer can safely ignore those duplicates.

#### **Why the Transaction is Critical:**
-   **Guarateed Atomicity**: It ensures the "Process" and "Mark as Processed" steps happen together. If the service crashes after sending a notification but before marking it done, the transaction rolls back, preventing a "ghost" record.
-   **Race Condition Prevention**: If two instances of the service receive the same message during a broker rebalance, the database transaction ensures only one can successfully "claim" and commit the event.

---

## Project Structure

```text
outboxer/
├── cmd/
│   ├── order-service/       # HTTP API — writes orders + outbox events
│   ├── relay/               # Polling relay-service — outbox → broker
│   └── notification-service/# Consumer-service — broker → stdout
├── internal/
│   ├── models/              # Order, OutboxEvent structs
│   ├── db/                  # PostgreSQL connection + migrations
│   ├── broker/              # Multi-broker impls (Kafka, Redis Streams)
│   └── config/              # Environment-based configuration
├── debezium/                # Debezium connector configs (Postgres/Redis)
├── scripts/                 # Helper scripts (register, test)
├── .env.example             # Template for local environment
├── .gitignore               # Excludes binaries, .env, and drafts
├── docker-compose.yml       # Core infrastructure (DB, Kafka, Redis)
├── docker-compose.debezium.yml       # CDC overlay for Kafka
├── docker-compose.debezium-redis.yml # CDC overlay for Redis
└── Makefile                 # Standardized automation commands
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://outbox:outbox@localhost:5432/outbox?sslmode=disable` | PostgreSQL connection string |
| `BROKER_TYPE` | `kafka` | `kafka` or `redis` |
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated Kafka brokers |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `POLL_INTERVAL` | `1s` | How often the relay polls for new events |
| `HTTP_PORT` | `8080` | Order Service HTTP port |

---

## Teardown

```bash
# Stop core infrastructure
make infra-down

# Stop Debezium Kafka setup
make debezium-kafka-down

# Stop Debezium Redis setup
make debezium-redis-down
```
