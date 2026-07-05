package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bingo/middleware"
)

// ShowAPIDocs serves the API instructions page
func ShowAPIDocs(w http.ResponseWriter, r *http.Request) {
	// If it's a web browser request, serve a beautiful HTML documentation page
	// If it's an API client asking for JSON (Accept: application/json), return a JSON summary of routes
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":        "Bingo API",
			"description": "Minimalist dosya ve metin paylaşım platformu API'si",
			"endpoints": []map[string]string{
				{
					"path":        "/api/upload",
					"method":      "POST",
					"description": "Dosya veya düz metin yükler. X-API-Key başlığı veya Bearer token ile doğrulanır.",
				},
				{
					"path":        "/api/stats",
					"method":      "GET",
					"description": "Sistem istatistiklerini döndürür. Sadece Süper Yönetici.",
				},
			},
		})
		return
	}

	// For browser, render the template
	// Check if user is logged in to show appropriate navigation links
	loggedInUser := middleware.GetLoggedUser(r)

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	RenderTemplate(w, "api_docs.html", map[string]interface{}{
		"Title":   "Bingo - API Dokümantasyonu",
		"User":    loggedInUser,
		"BaseURL": baseURL,
	})
}

// We need a helper to read sessions map for navigation checks in template render
// Let's duplicate or make a link to the session checks. In handlers/auth.go we already have session helpers,
// but let's make session checking clean.
// Since sessions and sessionMux are in middleware package, let's expose them or keep them internal.
// Wait, in middleware/auth.go, sessions is package-level and private.
// We can expose a helper in middleware/auth.go like `GetSessionUser(r *http.Request) (*db.User)` to check if a user is logged in.
// Let's check middleware/auth.go: we already have `GetUserFromContext(r *http.Request)`!
// Wait, `GetUserFromContext` only works if the RequireAuth middleware ran.
// Let's implement a middleware/auth.go helper that checks session cookie directly, e.g., `GetLoggedUser(r *http.Request) (*db.User)`.
// Actually, let's just make it check the cookie and lookup.
// Let's add a function to `middleware/auth.go` using a code replace, or check if we can write a simple custom session check in auth.go.
// Let's write `GetLoggedUser` in middleware/auth.go to make it clean!
// Wait! Let's check if we can modify middleware/auth.go to add `GetLoggedUser` so we don't have to duplicate the map access.
