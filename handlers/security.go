package handlers

import (
	"net"
	"net/http"
	"strings"
)

// SafeHost returns a validated Host for rendering absolute URLs. A malicious
// Host header (e.g. containing quotes or script) would otherwise be reflected
// into pages, API responses and share links (cache-poisoning / phishing).
// Anything outside hostname / IP / port charset falls back to "localhost".
func SafeHost(r *http.Request) string {
	h := strings.TrimSpace(r.Host)
	if h == "" || len(h) > 253 {
		return "localhost"
	}

	host, port := h, ""
	if strings.HasPrefix(h, "[") {
		// IPv6 literal, optionally with port: [::1]:8080
		if hp, p, err := net.SplitHostPort(h); err == nil {
			host, port = hp, p
		} else {
			host = strings.Trim(h, "[]")
		}
	} else if strings.Count(h, ":") == 1 {
		if hp, p, err := net.SplitHostPort(h); err == nil {
			host, port = hp, p
		}
	}

	if host == "" || len(host) > 253 {
		return "localhost"
	}
	// Hostname / IPv4 / IPv6 charset only. This rejects quotes, angle
	// brackets, spaces, slashes and control characters used in XSS and
	// header-injection payloads.
	for _, c := range host {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '-' || c == ':' {
			continue
		}
		return "localhost"
	}
	if port != "" {
		for _, c := range port {
			if c < '0' || c > '9' {
				return "localhost"
			}
		}
		return host + ":" + port
	}
	return host
}

// BaseURL builds the absolute base URL with a validated host.
func BaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + SafeHost(r)
}
