package mcp

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"bingo/db"
)

// Session holds SSE client state
type SSESession struct {
	ID        string
	User      *db.User
	Server    *Server
	Writer    http.ResponseWriter
	Flusher   http.Flusher
	Done      chan struct{}
	CreatedAt time.Time
	Mu        sync.Mutex // serializes event-stream writes from concurrent POSTs
}

var (
	sessionsMu sync.RWMutex
	sessions   = make(map[string]*SSESession)
)

const (
	// maxSSESessions caps concurrent SSE streams (slow-connection DoS defense).
	maxSSESessions = 1000
	// maxSSESessionAge expires SSE sessions that never closed cleanly.
	maxSSESessionAge = 2 * time.Hour
)

// maxMCPBodyBytes caps MCP HTTP POST bodies, aligned with the configured
// upload limit plus JSON-RPC/base64 envelope overhead.
func maxMCPBodyBytes() int64 {
	mb := db.GetSettingInt("max_upload_size_mb", 50)
	if mb <= 0 {
		mb = 50
	}
	if mb > 10240 {
		mb = 10240
	}
	return int64(mb)*1024*1024 + 5*1024*1024
}

// pruneSessions removes expired SSE sessions. Must be called with write lock.
func pruneSessions() {
	now := time.Now()
	for id, sess := range sessions {
		if now.Sub(sess.CreatedAt) > maxSSESessionAge {
			delete(sessions, id)
		}
	}
}

// GetSession retrieves an active SSESession by ID
func GetSession(sessionID string) *SSESession {
	if sessionID == "" || len(sessionID) > 128 {
		return nil
	}
	sessionsMu.RLock()
	sess := sessions[sessionID]
	sessionsMu.RUnlock()
	if sess != nil && time.Since(sess.CreatedAt) > maxSSESessionAge {
		sessionsMu.Lock()
		// Re-check under write lock before deleting.
		if cur, ok := sessions[sessionID]; ok && time.Since(cur.CreatedAt) > maxSSESessionAge {
			delete(sessions, sessionID)
		}
		sessionsMu.Unlock()
		return nil
	}
	return sess
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HandleStdio runs MCP over standard input/output for local CLI clients (Claude Desktop, Cursor, etc.)
func HandleStdio(user *db.User, baseURL, uploadsDir string) {
	server := NewServer(user, baseURL, uploadsDir)
	scanner := bufio.NewScanner(os.Stdin)

	// Buffer up to 10MB per line for large payloads
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			errResp := Response{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &RPCError{
					Code:    CodeParseError,
					Message: fmt.Sprintf("Parse error: %v", err),
				},
			}
			out, _ := json.Marshal(errResp)
			fmt.Printf("%s\n", out)
			continue
		}

		resp := server.ProcessRequest(&req)
		if resp != nil {
			out, err := json.Marshal(resp)
			if err == nil {
				fmt.Printf("%s\n", out)
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("MCP Stdio scanner error: %v", err)
	}
}

// HandleHTTP handles universal HTTP POST and GET/SSE for MCP
func HandleHTTP(w http.ResponseWriter, r *http.Request, user *db.User, baseURL, uploadsDir string) {
	server := NewServer(user, baseURL, uploadsDir)

	// 1. Handle GET: Differentiate between SSE (text/event-stream) and standard HTTP GET (probe/metadata)
	if r.Method == http.MethodGet {
		accept := r.Header.Get("Accept")
		isSSE := strings.Contains(accept, "text/event-stream") ||
			r.URL.Query().Get("sse") == "true" ||
			strings.HasSuffix(r.URL.Path, "/sse")

		if !isSSE {
			// Standard HTTP GET probe or metadata check (e.g. Gemini Spark or browser)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Mcp-Session-Id", "bingo-session")
			w.Header().Set("X-Session-Id", "bingo-session")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":          "ready",
				"name":            "bingo",
				"version":         "1.0.0",
				"protocolVersion": "2024-11-05",
				"jsonrpc":         "2.0",
				"result": map[string]any{
					"status":          "ready",
					"name":            "bingo",
					"version":         "1.0.0",
					"protocolVersion": "2024-11-05",
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

		// Client explicitly requested SSE
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		sessionsMu.Lock()
		pruneSessions()
		if len(sessions) >= maxSSESessions {
			sessionsMu.Unlock()
			http.Error(w, "Too many streams, try again later", http.StatusServiceUnavailable)
			return
		}
		sessionID := generateSessionID()
		sess := &SSESession{
			ID:        sessionID,
			User:      user,
			Server:    server,
			Writer:    w,
			Flusher:   flusher,
			Done:      make(chan struct{}),
			CreatedAt: time.Now(),
		}
		sessions[sessionID] = sess
		sessionsMu.Unlock()

		defer func() {
			sessionsMu.Lock()
			delete(sessions, sessionID)
			sessionsMu.Unlock()
		}()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Mcp-Session-Id", sessionID)
		w.Header().Set("X-Session-Id", sessionID)

		// Send endpoint event according to MCP SSE specification.
		// NOTE: the API key is deliberately NOT embedded here. The session is
		// already bound to the authenticated user server-side, so clients only
		// need the sessionId. Embedding secrets in event streams leaks them
		// into logs and browser history.
		msgEndpoint := fmt.Sprintf("/mcp/messages?sessionId=%s", sessionID)
		fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", msgEndpoint)
		flusher.Flush()

		// Keep connection open until client disconnects
		<-r.Context().Done()
		return
	}

	// 2. Handle POST for Streamable HTTP or SSE message posting
	if r.Method == http.MethodPost {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Bound request body BEFORE decoding (DoS protection: previously
		// unlimited JSON decode, including huge base64 file payloads).
		r.Body = http.MaxBytesReader(w, r.Body, maxMCPBodyBytes())

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
		if sessionID == "" {
			sessionID = generateSessionID()
		}

		w.Header().Set("Mcp-Session-Id", sessionID)
		w.Header().Set("X-Session-Id", sessionID)
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type, X-Session-Id, Mcp-Session-Id")

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Response{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: CodeParseError, Message: "Parse error"},
			})
			return
		}

		// Check if this belongs to an active SSE session (e.g. POST /mcp/messages?sessionId=...)
		sessionsMu.RLock()
		sess, exists := sessions[sessionID]
		sessionsMu.RUnlock()

		if exists && sess != nil {
			resp := sess.Server.ProcessRequest(&req)
			if resp != nil {
				// Send response over SSE event stream (serialized: concurrent
				// POSTs for the same session must not interleave frames).
				respBytes, _ := json.Marshal(resp)
				sess.Mu.Lock()
				fmt.Fprintf(sess.Writer, "event: message\ndata: %s\n\n", string(respBytes))
				sess.Flusher.Flush()
				sess.Mu.Unlock()
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, "Accepted")
			return
		}

		// Streamable HTTP: Notification messages (no ID) return 202 Accepted per spec
		if req.ID == nil && (strings.HasPrefix(req.Method, "notifications/") || req.Method == "initialized") {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// Direct JSON-RPC POST request / response (standard Streamable HTTP transport)
		resp := server.ProcessRequest(&req)
		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}

	// 3. Handle DELETE (Streamable HTTP session close)
	if r.Method == http.MethodDelete {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		return
	}

	// 4. Handle OPTIONS
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, x-api-key, X-Session-Id, x-session-id, Mcp-Session-Id, mcp-session-id, Accept, Mcp-Method, Mcp-Name, X-Forwarded-Proto")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}
