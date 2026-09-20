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
	"sync"

	"bingo/db"
)

// Session holds SSE client state
type SSESession struct {
	ID      string
	User    *db.User
	Server  *Server
	Writer  http.ResponseWriter
	Flusher http.Flusher
	Done    chan struct{}
}

var (
	sessionsMu sync.RWMutex
	sessions   = make(map[string]*SSESession)
)

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

	// Handle GET for Server-Sent Events (SSE)
	if r.Method == http.MethodGet {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		sessionID := generateSessionID()
		sess := &SSESession{
			ID:      sessionID,
			User:    user,
			Server:  server,
			Writer:  w,
			Flusher: flusher,
			Done:    make(chan struct{}),
		}

		sessionsMu.Lock()
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
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Send endpoint event according to MCP SSE specification
		msgEndpoint := fmt.Sprintf("/mcp/messages?sessionId=%s", sessionID)
		fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", msgEndpoint)
		flusher.Flush()

		// Keep connection open until client disconnects
		<-r.Context().Done()
		return
	}

	// Handle POST for direct JSON-RPC or SSE message posting
	if r.Method == http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Response{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &RPCError{Code: CodeParseError, Message: "Parse error"},
			})
			return
		}

		// Check if this belongs to an active SSE session
		sessionID := r.URL.Query().Get("sessionId")
		if sessionID != "" {
			sessionsMu.RLock()
			sess, exists := sessions[sessionID]
			sessionsMu.RUnlock()

			if exists {
				resp := sess.Server.ProcessRequest(&req)
				if resp != nil {
					// Send response over SSE event stream
					respBytes, _ := json.Marshal(resp)
					fmt.Fprintf(sess.Writer, "event: message\ndata: %s\n\n", string(respBytes))
					sess.Flusher.Flush()
				}
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
				return
			}
		}

		// Direct JSON-RPC POST request / response (standard HTTP transport)
		resp := server.ProcessRequest(&req)
		if resp != nil {
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
