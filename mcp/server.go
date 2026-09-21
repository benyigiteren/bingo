package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bingo/db"
)

type Server struct {
	User       *db.User
	BaseURL    string
	UploadsDir string
}

func NewServer(user *db.User, baseURL, uploadsDir string) *Server {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if uploadsDir == "" {
		uploadsDir = "uploads"
	}
	return &Server{
		User:       user,
		BaseURL:    baseURL,
		UploadsDir: uploadsDir,
	}
}

// ProcessRequest executes incoming JSON-RPC request and returns Response
func (s *Server) ProcessRequest(req *Request) *Response {
	// Notifications (no ID)
	if req.ID == nil {
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized", "initialized":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "ping":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "resources/templates/list":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resourceTemplates": []any{}}}
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptsGet(req)
	case "logging/setLevel":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "completion/complete":
		return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"completion": map[string]any{"values": []string{}}}}
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	protoVer := "2024-11-05"
	if req.Params != nil {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(req.Params, &p); err == nil && p.ProtocolVersion != "" {
			protoVer = p.ProtocolVersion
		}
	}

	result := InitializeResult{
		ProtocolVersion: protoVer,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
			Prompts:   &PromptsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    "bingo",
			Version: "1.0.0",
		},
		Instructions: "Bingo is a self-hosted Pastebin & File Vault. Use bingo_share_paste to instantly publish code snippets, debug logs, markdown notes, and files with optional TTL expiration, burn-after-reading, or password protection. Every upload returns an immediate, formatted live link.",
	}
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) handlePromptsList(req *Request) *Response {
	prompts := []map[string]any{
		{
			"name":        "share_code_snippet",
			"description": "Share a code snippet or solution on Bingo and get a shareable link",
			"arguments": []map[string]any{
				{
					"name":        "code",
					"description": "The source code to share",
					"required":    true,
				},
				{
					"name":        "language",
					"description": "Programming language (e.g. go, python, javascript)",
					"required":    false,
				},
			},
		},
	}
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": prompts}}
}

func (s *Server) handlePromptsGet(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"description": "Share code snippet prompt",
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": "Please share the provided code snippet to Bingo using bingo_share_paste.",
					},
				},
			},
		},
	}
}

func (s *Server) handleToolsList(req *Request) *Response {
	tools := []Tool{
		{
			Name:        "bingo_share_paste",
			Description: "Share text, markdown, log, or source code on Bingo. Returns a public, live shareable link with optional TTL, password, and burn-after-reading.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"content": {
						Type:        "string",
						Description: "The raw text, code snippet, markdown, or log data to share.",
					},
					"filename": {
						Type:        "string",
						Description: "Optional filename with extension (e.g. 'notes.md', 'solution.py', 'dump.json', 'app.log'). If omitted, defaults to a timestamped .txt file.",
					},
					"ttl": {
						Type:        "string",
						Description: "Optional time-to-live duration before automatic deletion. Options: '10m', '1h', '1d', '1w', '30d', or 'forever'. Default: 'forever'.",
						Enum:        []string{"10m", "1h", "1d", "1w", "30d", "forever"},
					},
					"is_burn": {
						Type:        "boolean",
						Description: "Set to true for 'burn after reading' (destroyed immediately after the first view).",
					},
					"password": {
						Type:        "string",
						Description: "Optional password required to view the shared paste.",
					},
				},
				Required: []string{"content"},
			},
		},
		{
			Name:        "bingo_upload_file",
			Description: "Upload a binary or text file (e.g. image, PDF, zip) encoded as base64 to Bingo. Returns a live shareable link.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"filename": {
						Type:        "string",
						Description: "Target filename with extension (e.g. 'chart.png', 'report.pdf').",
					},
					"content_base64": {
						Type:        "string",
						Description: "File content encoded as Base64 string.",
					},
					"ttl": {
						Type:        "string",
						Description: "Optional expiration duration ('10m', '1h', '1d', '1w', '30d', 'forever').",
						Enum:        []string{"10m", "1h", "1d", "1w", "30d", "forever"},
					},
				},
				Required: []string{"filename", "content_base64"},
			},
		},
		{
			Name:        "bingo_get_paste",
			Description: "Fetch and read the raw text content and metadata of a shared file or paste stored in Bingo.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"filename": {
						Type:        "string",
						Description: "The filename of the paste to retrieve (e.g. 'not.md').",
					},
				},
				Required: []string{"filename"},
			},
		},
		{
			Name:        "bingo_list_pastes",
			Description: "List recent pastes and files belonging to the authenticated user on Bingo.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"limit": {
						Type:        "integer",
						Description: "Maximum number of items to return (default 20, max 100).",
					},
				},
			},
		},
		{
			Name:        "bingo_search_pastes",
			Description: "Search pastes and files by name or keywords.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"query": {
						Type:        "string",
						Description: "Search query or filename fragment.",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "bingo_delete_paste",
			Description: "Permanently delete a paste or file from Bingo.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]PropertyDef{
					"filename": {
						Type:        "string",
						Description: "Filename of the paste to delete.",
					},
				},
				Required: []string{"filename"},
			},
		},
	}

	return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools}}
}

func (s *Server) handleToolsCall(req *Request) *Response {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    CodeInvalidParams,
				Message: "Invalid call tool parameters",
			},
		}
	}

	var result CallToolResult
	switch params.Name {
	case "bingo_share_paste":
		result = s.toolSharePaste(params.Arguments)
	case "bingo_upload_file":
		result = s.toolUploadFile(params.Arguments)
	case "bingo_get_paste":
		result = s.toolGetPaste(params.Arguments)
	case "bingo_list_pastes":
		result = s.toolListPastes(params.Arguments)
	case "bingo_search_pastes":
		result = s.toolSearchPastes(params.Arguments)
	case "bingo_delete_paste":
		result = s.toolDeletePaste(params.Arguments)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    CodeMethodNotFound,
				Message: fmt.Sprintf("Tool not found: %s", params.Name),
			},
		}
	}

	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func parseTTL(ttlStr string) *time.Time {
	if ttlStr == "" || ttlStr == "forever" {
		return nil
	}
	// Absolute upper bound: 30 days (same as REST API).
	const maxTTL = 30 * 24 * time.Hour
	var d time.Duration
	switch strings.ToLower(ttlStr) {
	case "10m":
		d = 10 * time.Minute
	case "1h":
		d = 1 * time.Hour
	case "1d":
		d = 24 * time.Hour
	case "1w":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		parsed, err := time.ParseDuration(ttlStr)
		if err == nil && parsed > 0 {
			d = parsed
		} else {
			return nil
		}
	}
	if d > maxTTL {
		d = maxTTL
	}
	exp := time.Now().Add(d)
	return &exp
}

const (
	// maxMCPPasteBytes caps inline text pastes via MCP (DoS protection).
	maxMCPPasteBytes = 5 * 1024 * 1024
	// maxMCPReadBytes caps how much file content is inlined into a tool
	// response; larger files are summarized instead of dumped.
	maxMCPReadBytes = 1024 * 1024
	// maxMCPFilenameLen bounds filenames accepted via MCP tools.
	maxMCPFilenameLen = 255
	// maxMCPPasswordLen bounds file passwords via MCP (bcrypt input).
	maxMCPPasswordLen = 128
	// maxMCPSearchLen bounds search queries.
	maxMCPSearchLen = 200
)

// maxMCPDecodedBytes caps base64-decoded file uploads via MCP, aligned with
// the admin-configured upload limit (default 50 MB) plus headroom.
func maxMCPDecodedBytes() int64 {
	mb := 50
	if v := getUploadLimitMB(); v > 0 {
		mb = v
	}
	if mb > 10240 {
		mb = 10240
	}
	return int64(mb) * 1024 * 1024
}

func getUploadLimitMB() int {
	return db.GetSettingInt("max_upload_size_mb", 50)
}

// sanitizeMCPFilename strips path components and unsafe runes, mirroring the
// REST upload sanitizer (kept local to avoid an import cycle with handlers).
func sanitizeMCPFilename(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > maxMCPFilenameLen {
		name = name[:maxMCPFilenameLen]
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "_")
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if res == "" || res == "." || res == ".." {
		res = fmt.Sprintf("paste_%d.txt", time.Now().Unix())
	}
	return res
}

// uniqueMCPFilename avoids silently overwriting an existing file: if the name
// is taken, a numeric suffix is appended (same UX as the web uploader).
func uniqueMCPFilename(username, filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	candidate := filename
	for i := 1; i <= 10000; i++ {
		existing, err := db.GetFile(username, candidate)
		if err != nil || existing == nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
	return fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
}

func (s *Server) toolSharePaste(args map[string]any) CallToolResult {
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: content cannot be empty."}}}
	}
	if len(content) > maxMCPPasteBytes {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error: content exceeds %d MB MCP paste limit.", maxMCPPasteBytes/1024/1024)}}}
	}

	filename, _ := args["filename"].(string)
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = fmt.Sprintf("paste_%d.txt", time.Now().Unix())
	} else {
		filename = sanitizeMCPFilename(filename)
	}

	ttlStr, _ := args["ttl"].(string)
	expiresAt := parseTTL(ttlStr)

	isBurn, _ := args["is_burn"].(bool)
	password, _ := args["password"].(string)
	if len(password) > maxMCPPasswordLen {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: password exceeds 128 characters."}}}
	}

	// Ensure user directory
	userDir := filepath.Join(s.UploadsDir, s.User.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error creating user directory: %v", err)}}}
	}

	// Never overwrite an existing file; pick a unique name instead.
	filename = uniqueMCPFilename(s.User.Username, filename)

	// Write content to disk
	targetPath := filepath.Join(userDir, filename)
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error saving paste: %v", err)}}}
	}

	// Save to DB
	mimeType := "text/plain; charset=utf-8"
	if strings.HasSuffix(filename, ".md") {
		mimeType = "text/markdown; charset=utf-8"
	} else if strings.HasSuffix(filename, ".json") {
		mimeType = "application/json"
	}

	fileRecord, err := db.CreateFileWithOpts(s.User.ID, filename, filename, int64(len(content)), mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
		IsBurn:    isBurn,
		Password:  password,
	})
	if err != nil {
		_ = os.Remove(targetPath)
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Database error: %v", err)}}}
	}

	shareURL := fmt.Sprintf("%s/%s/%s", s.BaseURL, s.User.Username, fileRecord.Filename)
	rawURL := fmt.Sprintf("%s?raw=true", shareURL)

	info := fmt.Sprintf("Paste successfully published!\n\n- Public URL: %s\n- Raw URL: %s\n- Filename: %s\n- Size: %d bytes", shareURL, rawURL, fileRecord.Filename, len(content))
	if expiresAt != nil {
		info += fmt.Sprintf("\n- Expires At: %s", expiresAt.Format(time.RFC3339))
	}
	if isBurn {
		info += "\n- Burn After Reading: Enabled (will self-destruct after 1st view)"
	}
	if password != "" {
		info += "\n- Protected with Password: Yes"
	}

	return CallToolResult{Content: []ToolContent{{Type: "text", Text: info}}}
}

func (s *Server) toolUploadFile(args map[string]any) CallToolResult {
	filename, _ := args["filename"].(string)
	filename = sanitizeMCPFilename(filename)
	if filename == "" || filename == "." {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: filename is required."}}}
	}

	contentB64, _ := args["content_base64"].(string)
	if contentB64 == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: content_base64 is required."}}}
	}
	// Reject absurd payloads before decoding: base64 inflates ~4/3.
	maxB64 := maxMCPDecodedBytes() * 4 / 3
	if int64(len(contentB64)) > maxB64 {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: file exceeds the configured upload size limit."}}}
	}

	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Invalid Base64 content: %v", err)}}}
	}
	if int64(len(data)) > maxMCPDecodedBytes() {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: file exceeds the configured upload size limit."}}}
	}

	ttlStr, _ := args["ttl"].(string)
	expiresAt := parseTTL(ttlStr)

	userDir := filepath.Join(s.UploadsDir, s.User.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Directory error: %v", err)}}}
	}

	filename = uniqueMCPFilename(s.User.Username, filename)
	targetPath := filepath.Join(userDir, filename)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error writing file: %v", err)}}}
	}

	mimeType := http.DetectContentType(data)
	fileRecord, err := db.CreateFileWithOpts(s.User.ID, filename, filename, int64(len(data)), mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
	})
	if err != nil {
		_ = os.Remove(targetPath)
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Database error: %v", err)}}}
	}

	shareURL := fmt.Sprintf("%s/%s/%s", s.BaseURL, s.User.Username, fileRecord.Filename)
	return CallToolResult{
		Content: []ToolContent{{
			Type: "text",
			Text: fmt.Sprintf("File uploaded successfully!\n- URL: %s\n- Size: %d bytes\n- MIME: %s", shareURL, len(data), mimeType),
		}},
	}
}

func (s *Server) toolGetPaste(args map[string]any) CallToolResult {
	filename, _ := args["filename"].(string)
	filename = sanitizeMCPFilename(filename)
	if filename == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: filename is required."}}}
	}

	meta, err := db.GetFile(s.User.Username, filename)
	if err != nil || meta == nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Paste not found: %s", filename)}}}
	}

	// Cap inlined content size (DoS protection); larger files are summarized.
	// NOTE: the caller is authenticated as the file owner, so reading own
	// password-protected files here is legitimate (protection applies to
	// public share links, not the owner).
	if meta.FileSize > maxMCPReadBytes {
		return CallToolResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("=== %s (Views: %d, Size: %d bytes) ===\nFile too large to inline (>1 MB). Download it from %s/%s/%s", filename, meta.Views, meta.FileSize, s.BaseURL, s.User.Username, filename)}}}
	}

	targetPath := filepath.Join(s.UploadsDir, s.User.Username, filename)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error reading file content: %v", err)}}}
	}
	if len(data) > maxMCPReadBytes {
		data = data[:maxMCPReadBytes]
	}

	return CallToolResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("=== %s (Views: %d, Size: %d B) ===\n\n%s", filename, meta.Views, meta.FileSize, string(data)),
			},
		},
	}
}

func (s *Server) toolListPastes(args map[string]any) CallToolResult {
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}

	files, err := db.GetFiles(s.User.ID, limit, 0)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Database error: %v", err)}}}
	}

	if len(files) == 0 {
		return CallToolResult{Content: []ToolContent{{Type: "text", Text: "No pastes or files found on Bingo."}}}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d items:\n\n", len(files)))
	for _, f := range files {
		url := fmt.Sprintf("%s/%s/%s", s.BaseURL, s.User.Username, f.Filename)
		flags := ""
		if f.IsBurn {
			flags += " [🔥 Burn]"
		}
		if f.HasPassword {
			flags += " [🔒 Protected]"
		}
		if f.ExpiresAt != nil {
			flags += fmt.Sprintf(" [⏳ Expires: %s]", f.ExpiresAt.Format("02.01.2006 15:04"))
		}
		sb.WriteString(fmt.Sprintf("- %s (%d bytes, %d views)%s -> %s\n", f.Filename, f.FileSize, f.Views, flags, url))
	}

	return CallToolResult{Content: []ToolContent{{Type: "text", Text: sb.String()}}}
}

func (s *Server) toolSearchPastes(args map[string]any) CallToolResult {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: query cannot be empty."}}}
	}
	if len(query) > maxMCPSearchLen {
		query = query[:maxMCPSearchLen]
	}

	files, err := db.SearchFiles(s.User.ID, s.User.Role == "super_admin", query, 30)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Search error: %v", err)}}}
	}

	if len(files) == 0 {
		return CallToolResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("No pastes found matching query '%s'", query)}}}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching items for '%s':\n\n", len(files), query))
	for _, f := range files {
		url := fmt.Sprintf("%s/%s/%s", s.BaseURL, f.Username, f.Filename)
		sb.WriteString(fmt.Sprintf("- %s (%d bytes, @%s) -> %s\n", f.Filename, f.FileSize, f.Username, url))
	}

	return CallToolResult{Content: []ToolContent{{Type: "text", Text: sb.String()}}}
}

func (s *Server) toolDeletePaste(args map[string]any) CallToolResult {
	filename, _ := args["filename"].(string)
	filename = sanitizeMCPFilename(filename)
	if filename == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: filename is required."}}}
	}

	meta, err := db.GetFile(s.User.Username, filename)
	if err != nil || meta == nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Paste not found: %s", filename)}}}
	}

	targetPath := filepath.Join(s.UploadsDir, s.User.Username, filename)
	_ = os.Remove(targetPath)

	if err := db.DeleteFile(meta.ID); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Failed to delete database record: %v", err)}}}
	}

	return CallToolResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Paste '%s' successfully deleted.", filename)}}}
}

func (s *Server) handleResourcesList(req *Request) *Response {
	files, err := db.GetFiles(s.User.ID, 50, 0)
	if err != nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInternalError, Message: err.Error()}}
	}

	var resources []Resource
	for _, f := range files {
		resources = append(resources, Resource{
			URI:         fmt.Sprintf("bingo://pastes/%s", f.Filename),
			Name:        f.Filename,
			Description: fmt.Sprintf("Shared on %s, size %d bytes", f.CreatedAt.Format(time.RFC3339), f.FileSize),
			MimeType:    f.MimeType,
		})
	}

	return &Response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": resources}}
}

func (s *Server) handleResourcesRead(req *Request) *Response {
	var params ReadResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInvalidParams, Message: "Invalid resource read params"}}
	}

	uri := strings.TrimPrefix(params.URI, "bingo://pastes/")
	filename := sanitizeMCPFilename(uri)

	meta, err := db.GetFile(s.User.Username, filename)
	if err != nil || meta == nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInvalidParams, Message: "Resource not found"}}
	}
	if meta.FileSize > maxMCPReadBytes {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInvalidParams, Message: "Resource too large to read inline"}}
	}

	targetPath := filepath.Join(s.UploadsDir, s.User.Username, filename)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInternalError, Message: "Could not read file"}}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ReadResourceResult{
			Contents: []ResourceContent{
				{
					URI:      params.URI,
					MimeType: meta.MimeType,
					Text:     string(data),
				},
			},
		},
	}
}
