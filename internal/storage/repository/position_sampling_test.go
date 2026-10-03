package repository

import (
	"maps"
	"testing"
)

// kept returns how many rows rn%stride==0 keeps out of count.
func kept(count, stride int64) int64 {
	return (count + stride - 1) / stride
}

func TestDeviceStrides(t *testing.T) {
	tests := []struct {
		name        string
		counts      map[int64]int64
		limit       int
		want        map[int64]int64
		wantSampled bool
	}{
		{"within limit keeps everything", map[int64]int64{1: 3, 2: 2}, 10, map[int64]int64{1: 1, 2: 1}, false},
		{"quiet device keeps all, busy shares the rest", map[int64]int64{1: 20, 2: 2}, 6, map[int64]int64{1: 5, 2: 1}, true},
		{"limit below device count keeps one row of the quietest", map[int64]int64{1: 9, 2: 5, 3: 5}, 2, map[int64]int64{2: 5, 3: 5}, true},
		{"busy devices share evenly", map[int64]int64{1: 100, 2: 100, 3: 1}, 11, map[int64]int64{1: 20, 2: 20, 3: 1}, true},
		{"zero count is ignored", map[int64]int64{1: 0, 2: 3}, 5, map[int64]int64{2: 1}, false},
		{"zero limit keeps nothing", map[int64]int64{1: 3}, 0, map[int64]int64{}, true},
		{"empty", map[int64]int64{}, 5, map[int64]int64{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, sampled := deviceStrides(tt.counts, tt.limit)
			if !maps.Equal(got, tt.want) || sampled != tt.wantSampled {
				t.Errorf("deviceStrides(%v, %d) = %v, %v; want %v, %v", tt.counts, tt.limit, got, sampled, tt.want, tt.wantSampled)
			}
		})
	}
}

func TestDeviceStrides_NeverExceedsLimit(t *testing.T) {
	cases := []map[int64]int64{
		{1: 10000, 2: 1, 3: 50},
		{1: 7, 2: 7, 3: 7, 4: 7},
		{1: 123456, 2: 98765, 3: 3},
	}
	for _, counts := range cases {
		for _, limit := range []int{1, 2, 3, 10, 100, 10000} {
			strides, _ := deviceStrides(counts, limit)
			var total int64
			for id, stride := range strides {
				total += kept(counts[id], stride)
			}
			if total > int64(limit) {
				t.Errorf("deviceStrides(%v, %d) keeps %d rows, want <= %d", counts, limit, total, limit)
			}
		}
	}
}
