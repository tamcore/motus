package websocket

// invalidationEnvelope is the Redis message payload for cross-pod cache
// invalidation. It mirrors the structure of redisEnvelope but carries no
// TraccarMessage — only the device ID that must be evicted from each pod's
// local access cache.
type invalidationEnvelope struct {
	OriginPodID string `json:"originPodId,omitempty"`
	DeviceID    int64  `json:"deviceId"`
}
