package api

import (
	"sync"
	"time"
)

type Limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewLimiter() *Limiter { return &Limiter{hits: map[string][]time.Time{}} }

func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	items := l.hits[key]
	start := 0
	for start < len(items) && now.Sub(items[start]) > window {
		start++
	}
	items = items[start:]
	if len(items) >= limit {
		l.hits[key] = items
		return false
	}
	items = append(items, now)
	l.hits[key] = items
	return true
}

func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = map[string][]time.Time{}
}
