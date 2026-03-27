package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modx/outbox/internal/broker"
	"github.com/modx/outbox/internal/config"
	"github.com/modx/outbox/internal/db"
	"github.com/modx/outbox/internal/models"
)

func main() {
	cfg := config.Load()

	// Connect to the shared DB to track processing state (Inbox Pattern)
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	var sub broker.Subscriber
	consumerGroup := "notification-group"
	consumerName := "notification-consumer-1"

	switch cfg.BrokerType {
	case "kafka":
		sub = broker.NewKafkaSubscriber(cfg.KafkaBrokers, consumerGroup)
	case "redis":
		sub = broker.NewRedisSubscriber(cfg.RedisAddr, consumerGroup, consumerName)
	default:
		log.Fatalf("unsupported BROKER_TYPE: %s (expected kafka or redis)", cfg.BrokerType)
	}
	defer sub.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	topic := os.Getenv("TOPIC")
	if topic == "" {
		topic = "order.created"
	}
	log.Printf("📥 Notification Service listening on topic=%s (broker=%s)", topic, cfg.BrokerType)

	err = sub.Subscribe(ctx, topic, func(eventID string, payload []byte) error {
		// --- Inbox Pattern: Transactional Processing ---
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		// 1. Check if already processed (Technical Deduplication via eventID)
		var exists bool
		err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)", eventID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			log.Printf("⚠️  Skipping event %s (Technical duplicate: already processed)", eventID)
			return nil 
		}

		// 2. Business logic (mock)
		var order models.Order
		if err := unmarshalPayload(payload, &order); err != nil {
			log.Printf("⚠️  Skipping event %s: %v", eventID, err)
			return nil
		}
		// 1b. Check Business-Level Idempotency (Same order, same event type)
		// We do this after unmarshalling so we have access to order.ID
		var businessExists bool
		err = tx.QueryRowContext(ctx, 
			"SELECT EXISTS(SELECT 1 FROM processed_events WHERE aggregate_id = $1 AND event_type = $2 AND consumer_name = $3)", 
			order.ID, topic, consumerName).Scan(&businessExists)
		if err != nil {
			return err
		}
		if businessExists {
			log.Printf("⚠️  Skipping event %s (Business duplicate: %s already handled for order %s)", eventID, topic, order.ID)
			return nil 
		}

		fmt.Println("═══════════════════════════════════════════════════")
		fmt.Printf("📬 NEW ORDER NOTIFICATION (ID: %s)\n", eventID)
		fmt.Printf("   Order ID:  %s\n", order.ID)
		fmt.Printf("   Customer:  %s\n", order.CustomerName)
		fmt.Printf("   Product:   %s\n", order.Product)
		fmt.Printf("   Quantity:  %d\n", order.Quantity)
		fmt.Printf("   Created:   %s\n", order.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
		fmt.Println("═══════════════════════════════════════════════════")

		// 3. Mark as processed in the Inbox (Both Technical & Business)
		_, err = tx.ExecContext(ctx, 
			"INSERT INTO processed_events (event_id, aggregate_id, event_type, consumer_name) VALUES ($1, $2, $3, $4)", 
			eventID, order.ID, topic, consumerName)
		if err != nil {
			return err
		}

		// 4. Commit everything together
		return tx.Commit()
	})
	if err != nil {
		log.Fatalf("subscribe error: %v", err)
	}
}

// unmarshalPayload handles various Debezium and Go-relay JSON formats to extract the Order model.
func unmarshalPayload(data []byte, order *models.Order) error {
	// Attempt 1: Debezium Envelope (Postgres Connect)
	var envelope struct {
		After struct {
			Payload string `json:"payload"`
		} `json:"after"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.After.Payload != "" {
		return json.Unmarshal([]byte(envelope.After.Payload), order)
	}

	// Attempt 2: Direct Unmarshal (Go Relay or Redis-Sink default)
	if err := json.Unmarshal(data, order); err == nil {
		return nil
	}

	// Attempt 3: Doubled-quoted JSON string (Debezium Server Redis case)
	var jsonStr string
	if err := json.Unmarshal(data, &jsonStr); err == nil {
		return json.Unmarshal([]byte(jsonStr), order)
	}

	return fmt.Errorf("unsupported payload format: %s", string(data))
}
