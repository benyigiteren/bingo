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
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
			Prompts:   &PromptsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    "bingo-mcp",
			Version: "1.0.0",
		},
		Instructions: "Bingo is a self-hosted Pastebin & File Vault. Use bingo_share_paste to instantly publish code snippets, debug logs, markdown notes, and files with optional TTL expiration, burn-after-reading, or password protection. Every upload returns an immediate, formatted live link.",
	}
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
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
	exp := time.Now().Add(d)
	return &exp
}

func (s *Server) toolSharePaste(args map[string]any) CallToolResult {
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: content cannot be empty."}}}
	}

	filename, _ := args["filename"].(string)
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = fmt.Sprintf("paste_%d.txt", time.Now().Unix())
	} else {
		filename = filepath.Base(filename)
	}

	ttlStr, _ := args["ttl"].(string)
	expiresAt := parseTTL(ttlStr)

	isBurn, _ := args["is_burn"].(bool)
	password, _ := args["password"].(string)

	// Ensure user directory
	userDir := filepath.Join(s.UploadsDir, s.User.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error creating user directory: %v", err)}}}
	}

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
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: filename is required."}}}
	}

	contentB64, _ := args["content_base64"].(string)
	if contentB64 == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: content_base64 is required."}}}
	}

	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Invalid Base64 content: %v", err)}}}
	}

	ttlStr, _ := args["ttl"].(string)
	expiresAt := parseTTL(ttlStr)

	userDir := filepath.Join(s.UploadsDir, s.User.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Directory error: %v", err)}}}
	}

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
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "Error: filename is required."}}}
	}

	meta, err := db.GetFile(s.User.Username, filename)
	if err != nil || meta == nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Paste not found: %s", filename)}}}
	}

	targetPath := filepath.Join(s.UploadsDir, s.User.Username, filename)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		return CallToolResult{IsError: true, Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error reading file content: %v", err)}}}
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
	filename = strings.TrimSpace(filepath.Base(filename))
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
	filename := filepath.Base(uri)

	meta, err := db.GetFile(s.User.Username, filename)
	if err != nil || meta == nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: &RPCError{Code: CodeInvalidParams, Message: "Resource not found"}}
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
