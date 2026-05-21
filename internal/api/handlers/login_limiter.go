package handlers

import (
	"sync"
	"time"
)

const (
	lockoutMaxAttempts = 10
	lockoutWindow      = 15 * time.Minute
	lockoutDuration    = 15 * time.Minute
)

type lockEntry struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*lockEntry)}
}

func (l *loginLimiter) isLocked(email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[email]
	if !ok {
		return false
	}
	return time.Now().Before(e.lockedUntil)
}

func (l *loginLimiter) recordFailure(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e, ok := l.entries[email]
	if !ok {
		l.entries[email] = &lockEntry{count: 1, windowStart: now}
		return
	}
	if now.Sub(e.windowStart) > lockoutWindow {
		// Window expired — reset.
		e.count = 1
		e.windowStart = now
		e.lockedUntil = time.Time{}
		return
	}
	e.count++
	if e.count >= lockoutMaxAttempts {
		e.lockedUntil = now.Add(lockoutDuration)
	}
}

func (l *loginLimiter) reset(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, email)
}
