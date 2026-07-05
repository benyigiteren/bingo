package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bingo/db"
)

// PrettySize formats file size in bytes to a human readable string
func PrettySize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024.0
	if kb < 1024 {
		return fmt.Sprintf("%.2f KB", kb)
	}
	mb := kb / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.2f MB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.2f GB", gb)
}

// ServeFile handles requests for site.domain/{username}/{filename}
func ServeFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	username := parts[0]
	filename := parts[1]

	// Prevent matching system reserved words as usernames
	reserved := map[string]bool{
		"api":       true,
		"static":    true,
		"dashboard": true,
		"login":     true,
		"register":  true,
		"logout":    true,
	}
	if reserved[username] {
		http.NotFound(w, r)
		return
	}

	// Strict security validation to prevent path traversal
	if username == "" || filename == "" {
		http.NotFound(w, r)
		return
	}

	cleanUsername := filepath.Clean(username)
	cleanFilename := filepath.Clean(filename)

	if cleanUsername != username || cleanFilename != filename ||
		strings.Contains(username, "..") || strings.Contains(filename, "..") ||
		strings.Contains(username, "/") || strings.Contains(filename, "/") ||
		strings.Contains(username, "\\") || strings.Contains(filename, "\\") {
		http.Error(w, "Forbidden path", http.StatusForbidden)
		return
	}

	// Fetch file metadata from DB
	fileMeta, err := db.GetFile(username, filename)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if fileMeta == nil {
		http.NotFound(w, r)
		return
	}

	// Build target path
	targetPath := filepath.Join("uploads", username, filename)

	// Check if file exists on disk
	info, err := os.Stat(targetPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Increment view count asynchronously
	go func(id int64) {
		_ = db.IncrementFileViews(id)
	}(fileMeta.ID)

	ext := strings.ToLower(filepath.Ext(filename))

	// 1. JSON served raw directly
	if ext == ".json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.ServeFile(w, r, targetPath)
		return
	}

	// 2. Text and Markdown formats
	if ext == ".md" || ext == ".txt" {
		// If raw parameter is set, serve raw content
		if r.URL.Query().Get("raw") == "true" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.ServeFile(w, r, targetPath)
			return
		}

		// Read content for the viewer
		contentBytes, err := os.ReadFile(targetPath)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}

		RenderTemplate(w, "viewer.html", map[string]interface{}{
			"Title":        fileMeta.OriginalName,
			"Filename":     fileMeta.Filename,
			"OriginalName": fileMeta.OriginalName,
			"Username":     fileMeta.Username,
			"FileSize":     PrettySize(fileMeta.FileSize),
			"Views":        fileMeta.Views + 1,
			"CreatedAt":    fileMeta.CreatedAt.Format("02.01.2006 15:04"),
			"Content":      string(contentBytes),
			"IsMarkdown":   ext == ".md",
		})
		return
	}

	// 3. Safe Images served directly inline
	safeImages := map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".webp": true,
		".gif":  true,
	}

	if safeImages[ext] {
		http.ServeFile(w, r, targetPath)
		return
	}

	// 4. Unsafe file types are forced to download (Stored XSS mitigation)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileMeta.OriginalName))
	http.ServeFile(w, r, targetPath)
}
