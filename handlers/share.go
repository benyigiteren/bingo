package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bingo/db"
)

// fileAuthSecret signs password-unlock cookies. Set BINGO_COOKIE_SECRET to a
// long random value in production so unlock cookies survive restarts;
// otherwise a random key is generated at startup (cookies invalidate on restart).
var fileAuthSecret []byte

func init() {
	if s := strings.TrimSpace(os.Getenv("BINGO_COOKIE_SECRET")); s != "" {
		sum := sha256.Sum256([]byte(s))
		fileAuthSecret = sum[:]
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err == nil {
		fileAuthSecret = b
		return
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("bingo-fallback-%d", time.Now().UnixNano())))
	fileAuthSecret = sum[:]
}

// unlockToken computes the HMAC proving knowledge of the file's password hash.
func unlockToken(fileID int64, passwordHash string) string {
	mac := hmac.New(sha256.New, fileAuthSecret)
	fmt.Fprintf(mac, "%d|%s", fileID, passwordHash)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyUnlockToken constant-time compares the presented cookie value.
func verifyUnlockToken(presented string, fileID int64, passwordHash string) bool {
	if presented == "" || passwordHash == "" {
		return false
	}
	expected := unlockToken(fileID, passwordHash)
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}

// sanitizeDownloadName prepares a filename for Content-Disposition headers:
// no quotes, no CRLF, no control chars (header-injection defense).
func sanitizeDownloadName(name string) string {
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	var sb strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		sb.WriteRune(r)
	}
	res := strings.TrimSpace(sb.String())
	if res == "" || res == "." || res == ".." {
		res = "dosya"
	}
	if len(res) > 180 {
		res = res[:180]
	}
	return res
}

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

// setServeHeaders applies baseline hardening headers to every file response.
func setServeHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
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
		"api":           true,
		"static":        true,
		"dashboard":     true,
		"login":         true,
		"register":      true,
		"logout":        true,
		"mcp":           true,
		"sse":           true,
		"messages":      true,
		"uploads":       true,
		"data":          true,
		".well-known":   true,
		"favicon.ico":   true,
		"robots.txt":    true,
		"manifest.json": true,
	}
	if reserved[username] {
		http.NotFound(w, r)
		return
	}

	// Strict security validation to prevent path traversal
	if username == "" || filename == "" || len(username) > 32 || len(filename) > 255 {
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
			if c, err := r.Cookie(cookieName); err == nil {
				// HMAC-signed token: cannot be forged without the password hash.
				authenticated = verifyUnlockToken(c.Value, fileMeta.ID, fileMeta.PasswordHash)
			}
		} else {
			if len(enteredPass) > maxFilePasswordLen {
				enteredPass = enteredPass[:maxFilePasswordLen]
			}
			valid, _ := db.CheckFilePassword(fileMeta.ID, enteredPass)
			if valid {
				authenticated = true
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    unlockToken(fileMeta.ID, fileMeta.PasswordHash),
					Path:     r.URL.Path,
					MaxAge:   3600,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
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

	// 3. Burn After Reading: atomically claim the file BEFORE serving so that
	// concurrent requests cannot both read it. The DELETE is atomic in SQLite:
	// exactly one request observes RowsAffected == 1 and wins.
	isBurnClaimed := false
	if fileMeta.IsBurn {
		res, delErr := db.DB.Exec("DELETE FROM files WHERE id = ?", fileMeta.ID)
		if delErr != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			http.Error(w, "Bu tek seferlik paylaşım zaten görüntülendi (Already consumed)", http.StatusGone)
			return
		}
		isBurnClaimed = true
		defer func(path string) {
			_ = os.Remove(path)
		}(targetPath)
	} else {
		// Increment view count asynchronously
		go func(id int64) {
			_ = db.IncrementFileViews(id)
		}(fileMeta.ID)
	}

	// NOTE: setServeHeaders is applied only to raw file / image / download
	// responses below. The HTML viewer template responses are governed by the
	// global CSP in main.go (a restrictive CSP here would break page styling).
	ext := strings.ToLower(filepath.Ext(filename))
	tooLargeForViewer := info.Size() > maxInlineViewerBytes || fileMeta.FileSize > maxInlineViewerBytes

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
	if lang, isCode := codeExtensions[ext]; isCode && !tooLargeForViewer {
		if r.URL.Query().Get("raw") == "true" {
			setServeHeaders(w)
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
		// Defense in depth: re-check size after read (file may have grown).
		if int64(len(contentBytes)) > maxInlineViewerBytes {
			setServeHeaders(w)
			safeName := sanitizeDownloadName(fileMeta.OriginalName)
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeName))
			w.Write(contentBytes)
			return
		}

		views := fileMeta.Views
		if !isBurnClaimed {
			views = fileMeta.Views + 1
		}
		RenderTemplate(w, "viewer.html", map[string]interface{}{
			"Title":        fileMeta.OriginalName,
			"Filename":     fileMeta.Filename,
			"OriginalName": fileMeta.OriginalName,
			"Username":     fileMeta.Username,
			"FileSize":     PrettySize(fileMeta.FileSize),
			"Views":        views,
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
		setServeHeaders(w)
		http.ServeFile(w, r, targetPath)
		return
	}

	// Unsafe or binary file types are forced to download.
	// OriginalName is sanitized to block CRLF/quote header injection.
	setServeHeaders(w)
	safeName := sanitizeDownloadName(fileMeta.OriginalName)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeName))
	http.ServeFile(w, r, targetPath)
}
