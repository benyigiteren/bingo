package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"bingo/db"
	"bingo/mcp"
)

// MCPDiscoveryHandler handles /.well-known/mcp.json and /mcp/manifest.json discovery
func MCPDiscoveryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type, X-Session-Id, Mcp-Session-Id")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Host header is validated (injection-safe) inside BaseURL.
	baseURL := BaseURL(r)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":            "bingo",
		"version":         "1.0.0",
		"description":     "Bingo is a self-hosted Pastebin & File Vault with Universal Model Context Protocol (MCP) support.",
		"protocolVersion": "2024-11-05",
		"endpoint":        baseURL + "/mcp",
		"transports":      []string{"streamable-http", "sse", "stdio"},
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": false},
			"resources": map[string]any{"subscribe": false, "listChanged": false},
			"prompts":   map[string]any{"listChanged": false},
		},
		"auth": map[string]any{
			"type":        "api_key",
			"header":      "X-API-Key",
			"query_param": "api_key",
		},
	})
}

// MCPHandler handles incoming Model Context Protocol requests over HTTP / SSE
func MCPHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Universal CORS Headers for all MCP clients (Web, IDE, CLI, Gemini, Claude, Cursor, ChatGPT)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, x-api-key, X-Session-Id, x-session-id, Mcp-Session-Id, mcp-session-id, Accept, Mcp-Method, Mcp-Name, X-Forwarded-Proto")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type, X-Session-Id, Mcp-Session-Id")

	// Handle CORS Preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Handle HEAD requests (health and reachability checks)
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "bingo-session")
		w.WriteHeader(http.StatusOK)
		return
	}

	// 2. Check if this request belongs to an active SSE session (e.g. POST /mcp/messages?sessionId=...)
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		sessionID = r.URL.Query().Get("session_id")
	}
	if sessionID == "" {
		sessionID = r.Header.Get("X-Session-Id")
	}
	if sessionID == "" {
		sessionID = r.Header.Get("Mcp-Session-Id")
	}

	var user *db.User
	if sessionID != "" {
		if sess := mcp.GetSession(sessionID); sess != nil && sess.User != nil {
			user = sess.User
		}
	}

	// 3. Extract API Key from Headers or Query params
	if user == nil {
		apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(r.Header.Get("x-api-key"))
		}
		if apiKey == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				apiKey = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			} else if strings.HasPrefix(authHeader, "bearer ") {
				apiKey = strings.TrimSpace(strings.TrimPrefix(authHeader, "bearer "))
			}
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(r.URL.Query().Get("api_key"))
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(r.URL.Query().Get("apiKey"))
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(r.URL.Query().Get("key"))
		}
		// Bound key length before DB lookup (DoS protection on huge inputs).
		if len(apiKey) > 256 {
			apiKey = apiKey[:256]
		}

		if apiKey != "" {
			var err error
			user, err = db.GetUserByAPIKey(apiKey)
			if err != nil || user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"error": map[string]any{
						"code":    -32000,
						"message": "Unauthorized: Invalid API Key.",
					},
				})
				return
			}
		} else {
			// No API key provided in request.
			// If it's a GET probe (not asking for SSE text/event-stream), return 200 OK with server info so URL verification succeeds!
			accept := r.Header.Get("Accept")
			if r.Method == http.MethodGet && !strings.Contains(accept, "text/event-stream") {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Mcp-Session-Id", "bingo-session")
				w.Header().Set("X-Session-Id", "bingo-session")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":          "ready",
					"name":            "bingo",
					"version":         "1.0.0",
					"protocolVersion": "2024-11-05",
					"authentication":  "api_key_required",
					"jsonrpc":         "2.0",
					"result": map[string]any{
						"status":          "ready",
						"name":            "bingo",
						"version":         "1.0.0",
						"protocolVersion": "2024-11-05",
						"authentication":  "api_key_required",
						"capabilities": map[string]any{
							"tools":     map[string]any{"listChanged": false},
							"resources": map[string]any{"subscribe": false, "listChanged": false},
							"prompts":   map[string]any{"listChanged": false},
						},
						"instructions": "Bingo is a self-hosted Pastebin & File Vault with Universal MCP support.",
					},
				})
				return
			}

			// If it's POST or SSE stream without an API key, return 401 Unauthorized
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"error": map[string]any{
					"code":    -32000,
					"message": "Unauthorized: Missing API Key. Provide via X-API-Key header, Authorization Bearer, or ?api_key= query parameter.",
				},
			})
			return
		}
	}

	if user != nil && !user.IsActive {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"error": map[string]any{
				"code":    -32000,
				"message": "Forbidden: User account is deactivated.",
			},
		})
		return
	}

	// 4. Resolve base URL for live share links (host validated)
	baseURL := BaseURL(r)

	// 5. Dispatch to MCP Transport
	mcp.HandleHTTP(w, r, user, baseURL, "uploads")
}
