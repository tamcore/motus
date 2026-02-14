package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisPubSub implements PubSub using Redis pub/sub for cross-pod broadcasting.
type RedisPubSub struct {
	client  *redis.Client
	sub     *redis.PubSub
	channel string
}

// NewRedisPubSub creates a new Redis pub/sub client. The redisURL should be a
// valid Redis connection string (e.g. "redis://localhost:6379"). The channel
// parameter is the Redis pub/sub channel name used for broadcasting messages.
func NewRedisPubSub(redisURL, channel string) (*RedisPubSub, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify the connection is usable.
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisPubSub{
		client:  client,
		channel: channel,
	}, nil
}

// Publish serialises message as JSON and publishes it to the Redis channel.
func (r *RedisPubSub) Publish(ctx context.Context, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := r.client.Publish(ctx, r.channel, data).Err(); err != nil {
		return fmt.Errorf("redis publish: %w", err)
	}

	return nil
}

// Subscribe starts listening on the Redis channel. Incoming messages are
// passed to handler as raw JSON bytes. The goroutine exits when ctx is
// cancelled. This method must be called at most once.
func (r *RedisPubSub) Subscribe(ctx context.Context, handler func([]byte)) error {
	r.sub = r.client.Subscribe(ctx, r.channel)

	// Wait for confirmation that the subscription is active.
	if _, err := r.sub.Receive(ctx); err != nil {
		_ = r.sub.Close()
		r.sub = nil
		return fmt.Errorf("redis subscribe: %w", err)
	}

	ch := r.sub.Channel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg != nil {
					handler([]byte(msg.Payload))
				}
			}
		}
	}()

	return nil
}

// Close tears down the Redis subscription and client connection.
func (r *RedisPubSub) Close() error {
	if r.sub != nil {
		if err := r.sub.Close(); err != nil {
			slog.Warn("redis pubsub close error", slog.Any("error", err))
		}
	}
	return r.client.Close()
}
