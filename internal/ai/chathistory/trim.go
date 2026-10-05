package chathistory

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/redis/go-redis/v9"
	"github.com/tamcore/motus/internal/ai/chat"
)

const (
	// MaxTurns is the maximum number of user turns to retain.
	MaxTurns = 30
	// MaxBytes is the maximum total serialised size of the history list.
	MaxBytes = 64 * 1024
)

// trimKey enforces the MaxTurns and MaxBytes caps by dropping oldest entries
// from the head. It always leaves the head at a "user" or "assistant" message
// so the list remains a valid conversation for replay.
func trimKey(ctx context.Context, rdb redis.Cmdable, k string) error {
	vals, err := rdb.LRange(ctx, k, 0, -1).Result()
	if err != nil {
		return err
	}
	roles := make([]string, len(vals))
	for i, v := range vals {
		var m chat.Message
		if json.Unmarshal([]byte(v), &m) == nil {
			roles[i] = m.Role
		}
	}
	start := cutIndex(vals, roles)
	if start == 0 {
		return nil
	}
	return rdb.LTrim(ctx, k, int64(start), -1).Err()
}

// cutIndex returns the smallest head index whose suffix fits both caps and
// starts at a "user" or "assistant" message (len(vals) when none does).
func cutIndex(vals, roles []string) int {
	totalBytes, turns := 0, 0
	for i, val := range slices.Backward(vals) {
		totalBytes += len(val)
		if roles[i] == "user" {
			turns++
		}
		if turns <= MaxTurns && totalBytes <= MaxBytes {
			continue
		}
		cut := i + 1
		for cut < len(vals) && roles[cut] != "user" && roles[cut] != "assistant" {
			cut++
		}
		return cut
	}
	return 0
}
