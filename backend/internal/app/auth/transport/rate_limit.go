package transport

import (
	"net/http"
	"sync"
	"time"
)

type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

type rateEntry struct {
	count int
	reset time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{entries: make(map[string]*rateEntry), limit: limit, window: window}
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		l.mu.Lock()
		entry := l.entries[r.RemoteAddr]
		if entry == nil || now.After(entry.reset) {
			entry = &rateEntry{reset: now.Add(l.window)}
			l.entries[r.RemoteAddr] = entry
		}
		entry.count++
		allowed := entry.count <= l.limit
		l.mu.Unlock()
		if !allowed {
			writeContractError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many authentication attempts", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
