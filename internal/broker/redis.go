package broker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// --- Redis Streams Publisher ---

type RedisPublisher struct {
	client *redis.Client
}

func NewRedisPublisher(addr string) *RedisPublisher {
	return &RedisPublisher{
		client: redis.NewClient(&redis.Options{Addr: addr}),
	}
}

func (r *RedisPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	// topic acts as the stream name.
	// we store the payload and key (for reference) in the stream entry.
	err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		Values: map[string]interface{}{
			"payload": payload,
			"key":     key,
		},
	}).Err()
	if err != nil {
		return err
	}
	log.Printf("[redis-streams] published to stream=%s (%d bytes)", topic, len(payload))
	return nil
}

func (r *RedisPublisher) Close() error {
	return r.client.Close()
}

// --- Redis Streams Subscriber ---

type RedisSubscriber struct {
	client  *redis.Client
	groupID string
	name    string
}

func NewRedisSubscriber(addr, groupID, name string) *RedisSubscriber {
	return &RedisSubscriber{
		client:  redis.NewClient(&redis.Options{Addr: addr}),
		groupID: groupID,
		name:    name,
	}
}

func (r *RedisSubscriber) Subscribe(ctx context.Context, topic string, handler func(string, []byte) error) error {
	// Create consumer group if it doesn't exist
	// "$" means only new messages, "0" would mean all existing.
	err := r.client.XGroupCreateMkStream(ctx, topic, r.groupID, "0").Err()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Printf("[redis-streams] warning: XGroupCreate error: %v", err)
	}

	log.Printf("[redis-streams] subscribed as group=%s name=%s to stream=%s", r.groupID, r.name, topic)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			// Read from the consumer group. ">" means messages never delivered to other consumers in group.
			streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    r.groupID,
				Consumer: r.name,
				Streams:  []string{topic, ">"},
				Count:    1,
				Block:    5 * time.Second,
			}).Result()

			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue // Timeout, no new messages
				}
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("[redis-streams] XReadGroup error: %v", err)
				time.Sleep(time.Second)
				continue
			}

			for _, stream := range streams {
				for _, msg := range stream.Messages {
					key, _ := msg.Values["key"].(string)
					payload, ok := msg.Values["payload"].(string)
					if !ok {
						// Fallback: Debezium Server Redis sink uses "value" natively
						payload, _ = msg.Values["value"].(string)
					}

					if err := handler(key, []byte(payload)); err != nil {
						log.Printf("[redis-streams] handler error: %v", err)
						continue
					}

					// Acknowledge the message
					r.client.XAck(ctx, topic, r.groupID, msg.ID)
				}
			}
		}
	}
}

func (r *RedisSubscriber) Close() error {
	return r.client.Close()
}
