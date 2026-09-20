package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// 1. Check TTL Expiration
	if fileMeta.ExpiresAt != nil && time.Now().After(*fileMeta.ExpiresAt) {
		_ = os.Remove(targetPath)
		_ = db.DeleteFile(fileMeta.ID)
		http.Error(w, "Bu paylaşımın süresi dolmuştur (Expired)", http.StatusGone)
		return
	}

	// 2. Check Password Protection
	if fileMeta.HasPassword {
		authenticated := false
		enteredPass := r.URL.Query().Get("pass")
		if enteredPass == "" && r.Method == http.MethodPost {
			enteredPass = r.FormValue("password")
		}

		cookieName := fmt.Sprintf("bg_auth_%d", fileMeta.ID)
		if enteredPass == "" {
			if c, err := r.Cookie(cookieName); err == nil && c.Value == "unlocked" {
				authenticated = true
			}
		} else {
			valid, _ := db.CheckFilePassword(fileMeta.ID, enteredPass)
			if valid {
				authenticated = true
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "unlocked",
					Path:     r.URL.Path,
					MaxAge:   3600,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
			}
		}

		if !authenticated {
			if r.URL.Query().Get("raw") == "true" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("401 Unauthorized: Parola korumalı paylaşım (Password required)"))
				return
			}

			RenderTemplate(w, "viewer.html", map[string]interface{}{
				"Title":        "Parola Korumalı Paylaşım",
				"Filename":     fileMeta.Filename,
				"OriginalName": fileMeta.OriginalName,
				"Username":     fileMeta.Username,
				"FileSize":     PrettySize(fileMeta.FileSize),
				"CreatedAt":    fileMeta.CreatedAt.Format("02.01.2006 15:04"),
				"IsLocked":     true,
				"WrongPass":    enteredPass != "",
			})
			return
		}
	}

	// 3. Burn After Reading: Destroy immediately after serving
	if fileMeta.IsBurn {
		defer func(id int64, path string) {
			_ = os.Remove(path)
			_ = db.DeleteFile(id)
		}(fileMeta.ID, targetPath)
	}

	// Increment view count asynchronously
	go func(id int64) {
		_ = db.IncrementFileViews(id)
	}(fileMeta.ID)

	ext := strings.ToLower(filepath.Ext(filename))

	// Code & text file detection for syntax viewer
	codeExtensions := map[string]string{
		".go":   "go",
		".py":   "python",
		".js":   "javascript",
		".ts":   "typescript",
		".jsx":  "javascript",
		".tsx":  "typescript",
		".rs":   "rust",
		".c":    "c",
		".cpp":  "cpp",
		".h":    "c",
		".java": "java",
		".html": "html",
		".css":  "css",
		".sh":   "bash",
		".bash": "bash",
		".sql":  "sql",
		".yaml": "yaml",
		".yml":  "yaml",
		".xml":  "xml",
		".csv":  "text",
		".log":  "text",
		".env":  "bash",
		".ini":  "ini",
		".conf": "text",
		".txt":  "text",
		".md":   "markdown",
		".json": "json",
	}

	// Text / Code / Markdown Viewer
	if lang, isCode := codeExtensions[ext]; isCode {
		if r.URL.Query().Get("raw") == "true" {
			if ext == ".json" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}
			http.ServeFile(w, r, targetPath)
			return
		}

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
			"Language":     lang,
			"IsBurn":       fileMeta.IsBurn,
			"ExpiresAt":    fileMeta.ExpiresAt,
			"HasPassword":  fileMeta.HasPassword,
		})
		return
	}

	// Safe Images served directly inline
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

	// Unsafe or binary file types are forced to download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileMeta.OriginalName))
	http.ServeFile(w, r, targetPath)
}
