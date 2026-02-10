package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// entry tracks a per-key rate limiter and when it was last used.
type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter is a keyed rate limiter — one rate.Limiter per key (IP or agent ID).
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	rate    rate.Limit
	burst   int
}

// NewLimiter creates a keyed rate limiter with the given rate and burst.
func NewLimiter(r rate.Limit, burst int) *Limiter {
	l := &Limiter{
		entries: make(map[string]*entry),
		rate:    r,
		burst:   burst,
	}
	go l.cleanup()
	return l
}

// Allow checks whether a request for the given key is allowed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = time.Now()
	l.mu.Unlock()
	return e.limiter.Allow()
}

// cleanup evicts entries idle for more than 30 minutes, every 10 minutes.
func (l *Limiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-30 * time.Minute)
		for k, e := range l.entries {
			if e.lastSeen.Before(cutoff) {
				delete(l.entries, k)
			}
		}
		l.mu.Unlock()
	}
}

// Named limiters — one per access tier/endpoint class.
var (
	// PublicRead: 60 req/min, burst 10, keyed by IP.
	PublicRead = NewLimiter(rate.Limit(60.0/60.0), 10)

	// AuthWrite: 20 req/min, burst 5, keyed by agent_id.
	AuthWrite = NewLimiter(rate.Limit(20.0/60.0), 5)

	// AuthWriteVerified: 60 req/min, burst 15, keyed by agent_id.
	AuthWriteVerified = NewLimiter(rate.Limit(60.0/60.0), 15)

	// DesignUpload: 10 req/min, burst 3, keyed by agent_id.
	DesignUpload = NewLimiter(rate.Limit(10.0/60.0), 3)

	// DesignUploadVerified: 30 req/min, burst 10, keyed by agent_id.
	DesignUploadVerified = NewLimiter(rate.Limit(30.0/60.0), 10)
)
