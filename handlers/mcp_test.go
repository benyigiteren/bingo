package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bingo/db"
	"bingo/handlers"
	"bingo/mcp"
	"bingo/middleware"
)

func setupTestDB(t *testing.T) (*db.User, func()) {
	tmpDir, err := os.MkdirTemp("", "bingo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	if err := db.InitDB(dbPath); err != nil {
		t.Fatalf("Failed to init test db: %v", err)
	}

	user, err := db.CreateUser("testuser", "testpass123", "super_admin")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	cleanup := func() {
		db.DB.Close()
		os.RemoveAll(tmpDir)
		os.RemoveAll("uploads/testuser")
	}

	return user, cleanup
}

func TestMCPInitializeAndTools(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Test MCP Initialize
	initReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion": "2024-11-05"}`),
	}
	body, _ := json.Marshal(initReq)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	handlers.MCPHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var initResp mcp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("Failed to decode MCP response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("MCP error: %v", initResp.Error)
	}

	// 2. Test MCP tools/list
	toolsReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	body, _ = json.Marshal(toolsReq)

	req = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("X-API-Key", user.APIKey)
	w = httptest.NewRecorder()

	handlers.MCPHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for tools/list, got %d", w.Code)
	}

	// 3. Test MCP bingo_share_paste tool execution
	shareReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "bingo_share_paste", "arguments": {"filename": "hello.go", "content": "package main\n\nfunc main() {}", "ttl": "1h"}}`),
	}
	body, _ = json.Marshal(shareReq)

	req = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("X-API-Key", user.APIKey)
	w = httptest.NewRecorder()

	handlers.MCPHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for tools/call, got %d", w.Code)
	}

	var callResp mcp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &callResp); err != nil {
		t.Fatalf("Failed to decode call tool response: %v", err)
	}
	if callResp.Error != nil {
		t.Fatalf("Call tool error: %v", callResp.Error)
	}

	// 4. Verify file was saved in DB
	fileMeta, err := db.GetFile("testuser", "hello.go")
	if err != nil || fileMeta == nil {
		t.Fatalf("Expected file to be created in db, err: %v", err)
	}
	if fileMeta.ExpiresAt == nil {
		t.Fatalf("Expected file to have TTL expiration")
	}

	// 5. Test MCP prompts/list
	promptsReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "prompts/list",
	}
	body, _ = json.Marshal(promptsReq)
	req = httptest.NewRequest(http.MethodPost, "/mcp?api_key="+user.APIKey, bytes.NewReader(body))
	w = httptest.NewRecorder()
	handlers.MCPHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for prompts/list, got %d", w.Code)
	}

	// 6. Test MCP ping
	pingReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "ping",
	}
	body, _ = json.Marshal(pingReq)
	req = httptest.NewRequest(http.MethodPost, "/mcp?api_key="+user.APIKey, bytes.NewReader(body))
	w = httptest.NewRecorder()
	handlers.MCPHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for ping, got %d", w.Code)
	}
}

func TestTTLAndPasswordProtection(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	exp := time.Now().Add(-10 * time.Minute) // already expired
	file, err := db.CreateFileWithOpts(user.ID, "expired.txt", "expired.txt", 12, "text/plain", db.FileOptions{
		ExpiresAt: &exp,
		Password:  "mysecret",
	})
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test Password check
	match, err := db.CheckFilePassword(file.ID, "mysecret")
	if err != nil || !match {
		t.Fatalf("Expected password to match")
	}

	wrongMatch, _ := db.CheckFilePassword(file.ID, "wrongpass")
	if wrongMatch {
		t.Fatalf("Expected wrong password to fail")
	}

	// Test Expired files discovery and cleanup
	expiredList, err := db.GetExpiredFiles()
	if err != nil {
		t.Fatalf("Failed to fetch expired files: %v", err)
	}
	if len(expiredList) != 1 {
		t.Fatalf("Expected 1 expired file, found %d", len(expiredList))
	}
}

func TestCreateTextWithExtension(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	// Simulate user typing filename without extension and selecting .go
	req := httptest.NewRequest("POST", "/dashboard/create-text", strings.NewReader("filename=mycode&extension=.go&content=package+main"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Inject user into context
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handlers.CreateTextHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("Expected 303 redirect, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify file was created in DB with .go extension
	files, err := db.GetFiles(user.ID, 10, 0)
	if err != nil || len(files) == 0 {
		t.Fatalf("Expected file to be found in db, err: %v", err)
	}

	found := false
	for _, f := range files {
		if f.Filename == "mycode.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected file to be named mycode.go, got: %s", files[0].Filename)
	}
}

func TestMCPSSEHandshakeAndMessageAuth(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Start SSE handshake with GET /mcp?api_key=...
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/mcp?api_key="+user.APIKey, nil).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	// Run SSE stream in background goroutine
	sseDone := make(chan struct{})
	go func() {
		defer close(sseDone)
		handlers.MCPHandler(w, req)
	}()

	// Wait briefly for endpoint event to be flushed
	time.Sleep(50 * time.Millisecond)

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "event: endpoint") {
		t.Fatalf("Expected event: endpoint in SSE stream, got: %s", bodyStr)
	}

	// Extract sessionId from /mcp/messages?sessionId=...
	idx := strings.Index(bodyStr, "sessionId=")
	if idx == -1 {
		t.Fatalf("Expected sessionId in endpoint event, got: %s", bodyStr)
	}
	sessionID := bodyStr[idx+len("sessionId="):]
	if ampIdx := strings.Index(sessionID, "&"); ampIdx != -1 {
		sessionID = sessionID[:ampIdx]
	} else if nlIdx := strings.Index(sessionID, "\n"); nlIdx != -1 {
		sessionID = sessionID[:nlIdx]
	}
	sessionID = strings.TrimSpace(sessionID)

	// 2. Now send a message to /mcp/messages?sessionId=... WITHOUT ANY API KEY HEADER!
	// This was previously failing with 401 Unauthorized
	initReq := mcp.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion": "2024-11-05"}`),
	}
	msgBody, _ := json.Marshal(initReq)

	msgReq := httptest.NewRequest(http.MethodPost, "/mcp/messages?sessionId="+sessionID, bytes.NewReader(msgBody))
	// Notice: NO X-API-Key or Authorization header is set!
	msgW := httptest.NewRecorder()
	handlers.MCPHandler(msgW, msgReq)

	if msgW.Code != http.StatusAccepted {
		t.Fatalf("Expected status 202 Accepted for SSE message POST, got %d: %s", msgW.Code, msgW.Body.String())
	}

	// Cancel SSE connection
	cancel()
	<-sseDone
}

func TestWebUploadWithCustomFilenameAndOpts(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Prepare multipart form with custom filename, TTL, is_burn, password
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "original_image.png")
	if err != nil {
		t.Fatalf("CreateFormFile error: %v", err)
	}
	part.Write([]byte("fake-png-content-data"))

	writer.WriteField("filename", "custom_named_report.png")
	writer.WriteField("ttl", "1h")
	writer.WriteField("is_burn", "true")
	writer.WriteField("password", "secret123")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/dashboard/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.WebUploadHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if resp["success"] != true {
		t.Fatalf("Expected success true, got: %v", resp)
	}

	// Verify file in DB
	fileObj, err := db.GetFile(user.Username, "custom_named_report.png")
	if err != nil || fileObj == nil {
		t.Fatalf("Expected file 'custom_named_report.png' in DB, got error: %v", err)
	}

	if fileObj.Filename != "custom_named_report.png" {
		t.Errorf("Expected filename 'custom_named_report.png', got '%s'", fileObj.Filename)
	}
	if fileObj.IsBurn != true {
		t.Errorf("Expected IsBurn true, got false")
	}
	if fileObj.PasswordHash == "" {
		t.Errorf("Expected PasswordHash to be set")
	}
	if fileObj.ExpiresAt == nil {
		t.Errorf("Expected ExpiresAt to be set for 1h TTL")
	}
}

func TestMCPProbeAndDiscovery(t *testing.T) {
	user, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Test unauthenticated HEAD probe
	reqHead := httptest.NewRequest(http.MethodHead, "/mcp", nil)
	wHead := httptest.NewRecorder()
	handlers.MCPHandler(wHead, reqHead)
	if wHead.Code != http.StatusOK {
		t.Fatalf("Expected HEAD /mcp to return 200 OK, got %d", wHead.Code)
	}
	if wHead.Header().Get("Mcp-Session-Id") == "" {
		t.Errorf("Expected Mcp-Session-Id header in HEAD response")
	}

	// 2. Test unauthenticated GET probe (Gemini Spark URL check)
	reqGet := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	wGet := httptest.NewRecorder()
	handlers.MCPHandler(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("Expected GET /mcp probe to return 200 OK, got %d: %s", wGet.Code, wGet.Body.String())
	}
	var probeResp map[string]any
	if err := json.Unmarshal(wGet.Body.Bytes(), &probeResp); err != nil {
		t.Fatalf("Failed to parse probe JSON: %v", err)
	}
	if probeResp["status"] != "ready" {
		t.Errorf("Expected status 'ready', got %v", probeResp["status"])
	}

	// 3. Test discovery endpoint /.well-known/mcp
	reqDisc := httptest.NewRequest(http.MethodGet, "/.well-known/mcp", nil)
	wDisc := httptest.NewRecorder()
	handlers.MCPDiscoveryHandler(wDisc, reqDisc)
	if wDisc.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for discovery, got %d", wDisc.Code)
	}
	var discResp map[string]any
	if err := json.Unmarshal(wDisc.Body.Bytes(), &discResp); err != nil {
		t.Fatalf("Failed to parse discovery JSON: %v", err)
	}
	if discResp["name"] != "bingo" {
		t.Errorf("Expected server name 'bingo', got %v", discResp["name"])
	}

	// 4. Test Streamable HTTP notification (202 Accepted)
	notifyReq := mcp.Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	notifyBytes, _ := json.Marshal(notifyReq)
	reqNotify := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(notifyBytes))
	reqNotify.Header.Set("X-API-Key", user.APIKey)
	wNotify := httptest.NewRecorder()
	handlers.MCPHandler(wNotify, reqNotify)
	if wNotify.Code != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted for notification, got %d: %s", wNotify.Code, wNotify.Body.String())
	}
}
