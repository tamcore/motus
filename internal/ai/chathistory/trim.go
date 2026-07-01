package chathistory

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/ai/chat"
)

const (
	// MaxTurns is the maximum number of user turns to retain.
	MaxTurns = 30
	// MaxBytes is the maximum total serialised size of the history list.
	MaxBytes = 64 * 1024
)

// trimKey enforces the MaxTurns and MaxBytes caps by popping oldest entries
// from the head. It always leaves the head at a "user" or "assistant" message
// so the list remains a valid conversation for replay.
func trimKey(ctx context.Context, rdb redis.Cmdable, k string) error {
	for {
		vals, err := rdb.LRange(ctx, k, 0, -1).Result()
		if err != nil || len(vals) == 0 {
			return err
		}
		if withinLimits(vals) {
			return nil
		}
		toDrop := dropCount(vals)
		for range toDrop {
			if err := rdb.LPop(ctx, k).Err(); err != nil && err != redis.Nil {
				return err
			}
		}
	}
}

// withinLimits returns true when both caps are satisfied.
func withinLimits(vals []string) bool {
	totalBytes := 0
	turns := 0
	for _, v := range vals {
		totalBytes += len(v)
		var m chat.Message
		if json.Unmarshal([]byte(v), &m) == nil && m.Role == "user" {
			turns++
		}
	}
	return turns <= MaxTurns && totalBytes <= MaxBytes
}

// dropCount returns how many entries to pop from the head so that the new
// head is either a "user" or "assistant" message (never an orphaned "tool").
// Always drops at least 1 entry.
func dropCount(vals []string) int {
	n := 1
	for n < len(vals) {
		var m chat.Message
		if json.Unmarshal([]byte(vals[n]), &m) == nil {
			if m.Role == "user" || m.Role == "assistant" {
				return n
			}
		}
		n++
	}
	return n
}
