// Package pubsub provides an abstraction for cross-pod message broadcasting.
// The primary implementation uses Redis pub/sub to relay WebSocket messages
// between multiple Motus replicas so that clients connected to any pod
// receive GPS position updates, device status changes, and events.
package pubsub

import "context"

// Publisher can publish messages to a channel.
type Publisher interface {
	Publish(ctx context.Context, message interface{}) error
}

// Subscriber can subscribe to a channel and receive messages.
type Subscriber interface {
	Subscribe(ctx context.Context, handler func([]byte)) error
}

// PubSub combines publishing and subscribing capabilities.
type PubSub interface {
	Publisher
	Subscriber
	Close() error
}
