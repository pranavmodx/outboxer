package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/modx/outbox/internal/broker"
	"github.com/modx/outbox/internal/config"
	"github.com/modx/outbox/internal/db"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	// Build the publisher based on broker type.
	var pub broker.Publisher
	switch cfg.BrokerType {
	case "kafka":
		pub = broker.NewKafkaPublisher(cfg.KafkaBrokers)
	case "redis":
		pub = broker.NewRedisPublisher(cfg.RedisAddr)
	default:
		log.Fatalf("unsupported BROKER_TYPE: %s (expected kafka or redis)", cfg.BrokerType)
	}
	defer pub.Close()

	log.Printf("🔄 Relay started (broker=%s, poll_interval=%s)", cfg.BrokerType, cfg.PollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Relay shutting down")
			return
		case <-ticker.C:
			if err := relayBatch(ctx, database, pub); err != nil {
				log.Printf("relay error: %v", err)
			}
		}
	}
}

// relayBatch reads pending outbox events, publishes them to the broker,
// and marks them as PUBLISHED — all in a tight loop.
func relayBatch(ctx context.Context, database *sql.DB, pub broker.Publisher) error {
	rows, err := database.QueryContext(ctx,
		`SELECT id, aggregate_type, aggregate_id, event_type, payload
		 FROM outbox_events
		 WHERE status = 'PENDING'
		 ORDER BY created_at
		 LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type event struct {
		id            string
		aggregateType string
		aggregateID   string
		eventType     string
		payload       []byte
	}

	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.aggregateType, &e.aggregateID, &e.eventType, &e.payload); err != nil {
			return err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range events {
		// Publish to the broker using the event type as the topic
		// and the aggregate ID as the message key (for Kafka partitioning).
		if err := pub.Publish(ctx, e.eventType, e.id, e.payload); err != nil {
			log.Printf("publish event %s failed: %v", e.id, err)
			continue // skip this one, retry on next poll
		}

		// Mark as published.
		if _, err := database.ExecContext(ctx,
			`UPDATE outbox_events SET status = 'PUBLISHED' WHERE id = $1`, e.id,
		); err != nil {
			log.Printf("mark published %s failed: %v", e.id, err)
		}
	}

	if len(events) > 0 {
		log.Printf("📤 Relayed %d event(s)", len(events))
	}
	return nil
}
