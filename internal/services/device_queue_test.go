package services

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestDeviceQueue_RunsJobsInOrderPerDevice(t *testing.T) {
	q := newDeviceQueue()
	var mu sync.Mutex
	var got []int
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		q.enqueue(1, func() {
			defer wg.Done()
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
		})
	}
	wg.Wait()
	want := make([]int, 50)
	for i := range want {
		want[i] = i
	}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestDeviceQueue_PanicDoesNotStallDevice(t *testing.T) {
	q := newDeviceQueue()
	done := make(chan struct{})
	q.enqueue(1, func() { panic("boom") })
	q.enqueue(1, func() { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job after a panicking job never ran")
	}
}

func TestDeviceQueue_WorkerRestartsAfterDrain(t *testing.T) {
	q := newDeviceQueue()
	first := make(chan struct{})
	q.enqueue(1, func() { close(first) })
	<-first
	// Wait for the worker to exit and remove the device.
	deadline := time.Now().Add(2 * time.Second)
	for {
		q.mu.Lock()
		_, running := q.jobs[1]
		q.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not exit after draining")
		}
		time.Sleep(time.Millisecond)
	}
	second := make(chan struct{})
	q.enqueue(1, func() { close(second) })
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("job enqueued after drain never ran")
	}
}
