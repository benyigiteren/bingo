package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strconv"
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

const (
	// maxLimiterEntries bounds memory usage. An attacker cycling API keys or
	// spoofed identifiers cannot grow the map unboundedly (DoS protection).
	maxLimiterEntries = 20000
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

// trustProxy reports whether X-Forwarded-For / X-Real-IP headers may be trusted.
// Default is FALSE to prevent IP-spoofing bypass of rate limits. Set
// TRUST_PROXY=1 (or "true") only when the app is actually behind a trusted
// reverse proxy (Nginx / Cloudflare / Traefik) that sanitizes these headers.
func trustProxy() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TRUST_PROXY")))
	return v == "1" || v == "true" || v == "yes"
}

func getClientIP(r *http.Request) string {
	// Only trust proxy headers when explicitly enabled. Otherwise an attacker
	// can bypass all IP-based rate limits by sending a fake X-Forwarded-For.
	if trustProxy() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			candidate := strings.TrimSpace(parts[0])
			if ip := net.ParseIP(candidate); ip != nil {
				return candidate
			}
		}
		if xip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xip != "" {
			if ip := net.ParseIP(xip); ip != nil {
				return xip
			}
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may already be a bare IP (notably in tests).
		if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
			return parsed.String()
		}
		// Never use raw attacker-influenced input unbounded: truncate.
		ra := strings.TrimSpace(r.RemoteAddr)
		if len(ra) > 64 {
			ra = ra[:64]
		}
		if ra == "" {
			return "unknown"
		}
		return ra
	}
	if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
		return parsed.String()
	}
	if len(ip) > 64 {
		ip = ip[:64]
	}
	if ip == "" {
		return "unknown"
	}
	return ip
}

// hashAPIKey avoids storing raw API secrets as map keys (they would linger in
// memory and could leak via heap dumps) and bounds key length.
func hashAPIKey(apiKey string) string {
	if len(apiKey) > 256 {
		apiKey = apiKey[:256]
	}
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// allow checks the token bucket for key. limit = tokens/sec refill,
// burst = max bucket size. Returns (allowed, retryAfterSeconds).
func allow(key string, limit, burst float64) (bool, int) {
	limitersMu.Lock()
	defer limitersMu.Unlock()

	now := time.Now()
	lim, exists := limiters[key]
	if !exists {
		// Evict oldest entry if the map is full (DoS protection against
		// cardinality exhaustion via rotating keys/IPs).
		if len(limiters) >= maxLimiterEntries {
			var oldestKey string
			var oldestTime time.Time
			first := true
			for k, v := range limiters {
				if first || v.lastRefill.Before(oldestTime) {
					oldestKey = k
					oldestTime = v.lastRefill
					first = false
				}
			}
			if oldestKey != "" {
				delete(limiters, oldestKey)
			}
		}
		limiters[key] = &limiter{tokens: burst - 1.0, lastRefill: now}
		return true, 0
	}

	elapsed := now.Sub(lim.lastRefill).Seconds()
	lim.tokens += elapsed * limit
	if lim.tokens > burst {
		lim.tokens = burst
	}
	lim.lastRefill = now

	if lim.tokens >= 1.0 {
		lim.tokens -= 1.0
		return true, 0
	}
	// How long until one token is available?
	need := 1.0 - lim.tokens
	secs := int(need/limit) + 1
	if secs < 1 {
		secs = 1
	}
	if secs > 60 {
		secs = 60
	}
	return false, secs
}

func tooManyRequests(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error": "Too many requests. Please slow down."}`))
}

// rateLimitWith is the shared token-bucket implementation.
func rateLimitWith(next http.Handler, limit, burst float64, keyPrefix func(r *http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := keyPrefix(r)
		ok, retryAfter := allow(key, limit, burst)
		if !ok {
			tooManyRequests(w, retryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimit middleware
// - Public IP: max 45 requests burst, refill 1.5/sec (90 req/min)
// - API Key: max 90 requests burst, refill 3.0/sec (180 req/min)
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var clientKey string
		var limit float64
		var burst float64

		// Check if it's an API request with a key (headers only; query keys
		// are not trusted for tiering to avoid log-leak abuse).
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			} else if strings.HasPrefix(auth, "bearer ") {
				apiKey = strings.TrimPrefix(auth, "bearer ")
			}
		}

		if apiKey != "" {
			clientKey = "api_" + hashAPIKey(apiKey)
			limit = 3.0 // 3.0 tokens per second (180/min)
			burst = 90.0
		} else {
			clientKey = "ip_" + getClientIP(r)
			limit = 1.5 // 1.5 tokens per second (90/min)
			burst = 45.0
		}

		ok, retryAfter := allow(clientKey, limit, burst)
		if !ok {
			tooManyRequests(w, retryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitLogin is a strict limiter for authentication endpoints
// (login / register). ~10 req/min per IP with burst 10. Blocks credential
// stuffing and brute-force while staying usable for humans.
func RateLimitLogin(next http.Handler) http.Handler {
	return rateLimitWith(next, 10.0/60.0, 10.0, func(r *http.Request) string {
		// Per-IP bucket; username-based bucketing is added at the handler
		// layer via LoginAttempt helpers if needed.
		return "login_ip_" + getClientIP(r)
	})
}

// RateLimitMCP protects Model Context Protocol endpoints (expensive JSON-RPC
// + file I/O). ~120 req/min per IP, higher tier per API key hash.
func RateLimitMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.Header.Get("x-api-key")
		}
		if apiKey == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			} else if strings.HasPrefix(auth, "bearer ") {
				apiKey = strings.TrimPrefix(auth, "bearer ")
			}
		}
		var key string
		var limit, burst float64
		if apiKey != "" {
			key = "mcp_api_" + hashAPIKey(apiKey)
			limit = 4.0 // 240/min
			burst = 120.0
		} else {
			key = "mcp_ip_" + getClientIP(r)
			limit = 2.0 // 120/min
			burst = 60.0
		}
		ok, retryAfter := allow(key, limit, burst)
		if !ok {
			tooManyRequests(w, retryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitFile protects public file serving + password-guessing surface.
// Generous for legitimate viewers, tight enough to slow password brute-force.
func RateLimitFile(next http.Handler) http.Handler {
	return rateLimitWith(next, 5.0, 200.0, func(r *http.Request) string {
		return "file_ip_" + getClientIP(r)
	})
}

// RateLimitGlobal is a coarse last-resort shield applied to every request so
// no endpoint is completely unprotected. Very generous: 600 req/min per IP.
func RateLimitGlobal(next http.Handler) http.Handler {
	return rateLimitWith(next, 10.0, 600.0, func(r *http.Request) string {
		return "global_ip_" + getClientIP(r)
	})
}

// RateLimitWrite protects authenticated write endpoints (upload / text /
// delete / user management): 60 req/min per IP.
func RateLimitWrite(next http.Handler) http.Handler {
	return rateLimitWith(next, 1.0, 60.0, func(r *http.Request) string {
		return "write_ip_" + getClientIP(r)
	})
}
