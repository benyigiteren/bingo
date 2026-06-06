package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ipLimit struct {
	tokens     float64
	lastAccess time.Time
}

var (
	// General rate limit bucket (120 req burst, 2/sec refill)
	generalLimits = make(map[string]*ipLimit)
	// Sensitive endpoint bucket (5 req burst, 0.1/sec refill = 1 per 10 sec)
	sensitiveLimits = make(map[string]*ipLimit)
	mu              sync.Mutex
)

func init() {
	// Goroutine to periodically clean up idle IP entries (prevents RAM leaks / keeps memory low)
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			mu.Lock()
			now := time.Now()
			for ip, limit := range generalLimits {
				if now.Sub(limit.lastAccess) > 10*time.Minute {
					delete(generalLimits, ip)
				}
			}
			for ip, limit := range sensitiveLimits {
				if now.Sub(limit.lastAccess) > 10*time.Minute {
					delete(sensitiveLimits, ip)
				}
			}
			mu.Unlock()
		}
	}()
}

// RateLimit middleware restricts requests based on IP address using separate buckets
// for sensitive endpoints (form submission, login) and general traffic.
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		// Support Cloudflare or proxy headers
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			ip = cfIP
		} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip = strings.TrimSpace(parts[0])
			}
		}

		path := r.URL.Path
		isSensitive := path == "/login" || path == "/setup" ||
			strings.HasPrefix(path, "/form/submit/") ||
			strings.HasPrefix(path, "/admin/users")

		mu.Lock()
		now := time.Now()

		var (
			limit     *ipLimit
			exists    bool
			maxTokens float64
			refill    float64
			bucketKey string
		)

		if isSensitive {
			maxTokens = 5.0
			refill = 0.1 // 1 token per 10 seconds
			bucketKey = "s:" + ip
			limit, exists = sensitiveLimits[bucketKey]
		} else {
			maxTokens = 120.0
			refill = 2.0 // 2 tokens per second
			bucketKey = ip
			limit, exists = generalLimits[bucketKey]
		}

		if !exists {
			limit = &ipLimit{
				tokens:     maxTokens,
				lastAccess: now,
			}
			if isSensitive {
				sensitiveLimits[bucketKey] = limit
			} else {
				generalLimits[bucketKey] = limit
			}
		} else {
			// Refill tokens based on elapsed time
			duration := now.Sub(limit.lastAccess).Seconds()
			limit.tokens += duration * refill
			if limit.tokens > maxTokens {
				limit.tokens = maxTokens
			}
			limit.lastAccess = now
		}

		if limit.tokens < 1.0 {
			mu.Unlock()
			if isSensitive {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"Çok fazla istek. Lütfen birkaç dakika bekleyin."}`))
			} else {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`
				<!DOCTYPE html>
				<html>
				<head>
					<meta charset="utf-8">
					<title>Çok Fazla İstek</title>
					<meta name="viewport" content="width=device-width, initial-scale=1">
					<style>
						body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #FAF7F2; color: #50311f; }
						.card { background: white; padding: 2.5rem; border-radius: 1.5rem; border: 1px solid #EAE6DF; box-shadow: 0 4px 20px rgba(0,0,0,0.02); max-width: 400px; text-align: center; }
						h1 { font-size: 1.5rem; margin-bottom: 1rem; font-weight: 700; }
						p { font-size: 0.95rem; color: #706a60; line-height: 1.5; margin-bottom: 1.5rem; }
						.btn { background: #50311f; color: white; border: none; padding: 0.75rem 1.5rem; border-radius: 9999px; font-weight: 600; cursor: pointer; text-decoration: none; font-size: 0.9rem; }
					</style>
				</head>
				<body>
					<div class="card">
						<h1>Çok Fazla İstek (Rate Limit)</h1>
						<p>Güvenliğimiz için kısa sürede çok fazla istek gönderdiniz. Lütfen birkaç dakika bekleyip tekrar deneyin.</p>
						<a href="javascript:location.reload()" class="btn">Sayfayı Yenile</a>
					</div>
				</body>
				</html>
			`))
			}
			return
		}

		limit.tokens -= 1.0
		mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
