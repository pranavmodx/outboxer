package models

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusFailed    = "FAILED"
)

// OutboxEvent represents a row in the outbox_events table.
type OutboxEvent struct {
	ID            string    `json:"id"`
	AggregateType string    `json:"aggregate_type"` // e.g. "order"
	AggregateID   string    `json:"aggregate_id"`   // e.g. the order ID
	EventType     string    `json:"event_type"`     // e.g. "order.created"
	Payload       []byte    `json:"payload"`        // JSON-encoded event body
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"` // PENDING, PUBLISHED, FAILED
}
