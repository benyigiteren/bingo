package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
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

