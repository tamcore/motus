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
	client     *redis.Client
	sub        *redis.PubSub
	channel    string
	ownsClient bool // true when this instance created the client and must close it
}

// NewRedisClient creates and verifies a Redis client from a connection URL.
// The caller is responsible for closing the returned client.
func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return client, nil
}

// NewRedisPubSub creates a new Redis pub/sub client from a connection URL.
// The channel parameter is the Redis pub/sub channel name used for broadcasting.
func NewRedisPubSub(redisURL, channel string) (*RedisPubSub, error) {
	client, err := NewRedisClient(redisURL)
	if err != nil {
		return nil, err
	}

	return &RedisPubSub{
		client:     client,
		channel:    channel,
		ownsClient: true,
	}, nil
}

// NewRedisPubSubFromClient creates a Redis pub/sub instance from an existing
// client. The caller retains ownership of the client; Close on the returned
// RedisPubSub will only tear down the subscription, not the client.
func NewRedisPubSubFromClient(client *redis.Client, channel string) (*RedisPubSub, error) {
	return &RedisPubSub{
		client:     client,
		channel:    channel,
		ownsClient: false,
	}, nil
}

// Publish serialises message as JSON and publishes it to the Redis channel.
func (r *RedisPubSub) Publish(ctx context.Context, message any) error {
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

// Close tears down the Redis subscription and, if this instance owns the
// client (created via NewRedisPubSub), the client connection as well.
func (r *RedisPubSub) Close() error {
	if r.sub != nil {
		if err := r.sub.Close(); err != nil {
			slog.Warn("redis pubsub close error", slog.Any("error", err))
		}
	}
	if r.ownsClient {
		return r.client.Close()
	}
	return nil
}
