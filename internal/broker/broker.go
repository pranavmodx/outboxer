package broker

import "context"

// Publisher sends events to a message broker.
type Publisher interface {
	// Publish sends a message to the given topic/channel.
	// key can be used for partitioning (Kafka) or is ignored (Redis).
	Publish(ctx context.Context, topic string, key string, payload []byte) error
	Close() error
}

// Subscriber receives events from a message broker.
type Subscriber interface {
	// Subscribe listens on the given topic/channel and calls handler for each message.
	// This method blocks until the context is cancelled.
	Subscribe(ctx context.Context, topic string, handler func(key string, payload []byte) error) error
	Close() error
}
