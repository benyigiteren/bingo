package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type limiter struct {
	tokens     float64
	lastRefill time.Time
}

var (
	limiters   = make(map[string]*limiter)
	limitersMu sync.Mutex
)

// CleanUpLimiters removes inactive rate limiters from memory
func CleanUpLimiters() {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for range ticker.C {
			limitersMu.Lock()
			now := time.Now()
			for key, lim := range limiters {
				// If no requests in the last 15 minutes, remove entry
				if now.Sub(lim.lastRefill) > 15*time.Minute {
					delete(limiters, key)
				}
			}
			limitersMu.Unlock()
		}
	}()
}

func getClientIP(r *http.Request) string {
	// Check headers first (behind proxy like Nginx / Cloudflare)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xip := r.Header.Get("X-Real-IP"); xip != "" {
		return xip
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RateLimit middleware
// - Public IP: max 45 requests burst, refill 1.5/sec (90 req/min)
// - API Key: max 90 requests burst, refill 3.0/sec (180 req/min)
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Identify user/client key
		var clientKey string
		var limit float64
		var burst float64

		// Check if it's an API request with a key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey != "" {
			clientKey = "api_" + apiKey
			limit = 3.0 // 3.0 tokens per second (180/min)
			burst = 90.0
		} else {
			clientKey = "ip_" + getClientIP(r)
			limit = 1.5 // 1.5 tokens per second (90/min)
			burst = 45.0
		}

		limitersMu.Lock()
		lim, exists := limiters[clientKey]
		now := time.Now()

		if !exists {
			lim = &limiter{
				tokens:     burst,
				lastRefill: now,
			}
			limiters[clientKey] = lim
		} else {
			// Calculate refilled tokens
			elapsed := now.Sub(lim.lastRefill).Seconds()
			lim.tokens += elapsed * limit
			if lim.tokens > burst {
				lim.tokens = burst
			}
			lim.lastRefill = now
		}

		if lim.tokens >= 1.0 {
			lim.tokens -= 1.0
			limitersMu.Unlock()
			next.ServeHTTP(w, r)
		} else {
			limitersMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Too many requests. Please slow down."}`))
		}
	})
}
