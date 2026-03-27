package broker

import (
	"context"
	"log"
	"strings"

	"github.com/segmentio/kafka-go"
)

// --- Kafka Publisher ---

type KafkaPublisher struct {
	writers map[string]*kafka.Writer
	brokers []string
}

func NewKafkaPublisher(brokers string) *KafkaPublisher {
	return &KafkaPublisher{
		writers: make(map[string]*kafka.Writer),
		brokers: strings.Split(brokers, ","),
	}
}

func (k *KafkaPublisher) getWriter(topic string) *kafka.Writer {
	if w, ok := k.writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(k.brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
	}
	k.writers[topic] = w
	return w
}

func (k *KafkaPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	w := k.getWriter(topic)
	err := w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: payload,
	})
	if err != nil {
		return err
	}
	log.Printf("[kafka] published to %s key=%s (%d bytes)", topic, key, len(payload))
	return nil
}

func (k *KafkaPublisher) Close() error {
	for _, w := range k.writers {
		_ = w.Close()
	}
	return nil
}

// --- Kafka Subscriber ---

type KafkaSubscriber struct {
	brokers []string
	groupID string
}

func NewKafkaSubscriber(brokers, groupID string) *KafkaSubscriber {
	return &KafkaSubscriber{
		brokers: strings.Split(brokers, ","),
		groupID: groupID,
	}
}

func (k *KafkaSubscriber) Subscribe(ctx context.Context, topic string, handler func(string, []byte) error) error {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: k.brokers,
		Topic:   topic,
		GroupID: k.groupID,
	})
	defer r.Close()

	log.Printf("[kafka] subscribed to topic=%s group=%s", topic, k.groupID)
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // context cancelled, graceful shutdown
			}
			return err
		}
		if err := handler(string(m.Key), m.Value); err != nil {
			log.Printf("[kafka] handler error: %v", err)
			continue
		}
		if err := r.CommitMessages(ctx, m); err != nil {
			log.Printf("[kafka] commit error: %v", err)
		}
	}
}

func (k *KafkaSubscriber) Close() error {
	return nil
}
