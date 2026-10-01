// Package pubsub provides an abstraction for cross-pod message broadcasting.
// The primary implementation uses Redis pub/sub to relay WebSocket messages
// between multiple Motus replicas so that clients connected to any pod
// receive GPS position updates, device status changes, and events.
package pubsub

import "context"

// PubSub combines publishing and subscribing capabilities.
type PubSub interface {
	Publish(ctx context.Context, message any) error
	Subscribe(ctx context.Context, handler func([]byte)) error
	Close() error
}
