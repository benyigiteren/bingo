package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"bingo/db"
	"bingo/mcp"
)

// MCPHandler handles incoming Model Context Protocol requests over HTTP / SSE
func MCPHandler(w http.ResponseWriter, r *http.Request) {
	// CORS Preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.WriteHeader(http.StatusOK)
		return
	}

	// 1. Extract API Key from multiple sources (Headers or Query params)
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}
	if apiKey == "" {
		apiKey = r.URL.Query().Get("apiKey")
	}
	if apiKey == "" {
		apiKey = r.URL.Query().Get("key")
	}

	if apiKey == "" {
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

	// 2. Validate API Key against database
	user, err := db.GetUserByAPIKey(apiKey)
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

	if !user.IsActive {
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

	// 3. Resolve base URL for live share links
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// 4. Dispatch to MCP Transport
	mcp.HandleHTTP(w, r, user, baseURL, "uploads")
}
