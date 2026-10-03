package services

import (
	"log/slog"
	"sync"
)

// deviceQueue runs jobs one at a time per device, in the order they were
// enqueued. Jobs for different devices run concurrently. A worker goroutine
// exists only while a device has queued jobs.
type deviceQueue struct {
	mu   sync.Mutex
	jobs map[int64][]func()
}

func newDeviceQueue() *deviceQueue {
	return &deviceQueue{jobs: make(map[int64][]func())}
}

// enqueue appends job to deviceID's queue and starts a worker for the device
// if none is running. It never blocks on job execution.
func (q *deviceQueue) enqueue(deviceID int64, job func()) {
	q.mu.Lock()
	pending, running := q.jobs[deviceID]
	q.jobs[deviceID] = append(pending, job)
	q.mu.Unlock()
	if !running {
		go q.drain(deviceID)
	}
}

// drain runs deviceID's jobs until its queue is empty, then removes the
// device so the next enqueue starts a new worker.
func (q *deviceQueue) drain(deviceID int64) {
	for {
		q.mu.Lock()
		pending := q.jobs[deviceID]
		if len(pending) == 0 {
			delete(q.jobs, deviceID)
			q.mu.Unlock()
			return
		}
		job := pending[0]
		pending[0] = nil
		q.jobs[deviceID] = pending[1:]
		q.mu.Unlock()
		runJob(job)
	}
}

// runJob runs job and recovers a panic, so one failing job cannot stall the
// device's queue forever.
func runJob(job func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("device queue job panicked", slog.Any("panic", r))
		}
	}()
	job()
}
