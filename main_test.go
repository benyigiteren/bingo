package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"bingo/handlers"
	"bingo/middleware"
)

func TestRouterInitializationAndStaticServing(t *testing.T) {
	// Initialize embedded templates
	if err := handlers.InitEmbeddedTemplates(embeddedFS, "templates"); err != nil {
		t.Fatalf("InitEmbeddedTemplates failed: %v", err)
	}

	// Build router matching main.go
	mux := http.NewServeMux()

	staticSub, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub static failed: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /register", handlers.ShowSetup)
	mux.HandleFunc("POST /register", handlers.ProcessSetup)
	mux.HandleFunc("GET /login", handlers.ShowLogin)
	mux.HandleFunc("POST /login", handlers.ProcessLogin)
	mux.HandleFunc("GET /logout", handlers.ProcessLogout)
	mux.HandleFunc("GET /api", handlers.ShowAPIDocs)
	mux.Handle("POST /api/upload", middleware.RateLimit(http.HandlerFunc(handlers.APIUploadHandler)))

	mux.HandleFunc("GET /mcp", handlers.MCPHandler)
	mux.HandleFunc("POST /mcp", handlers.MCPHandler)
	mux.HandleFunc("POST /mcp/messages", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp/messages", handlers.MCPHandler)

	mux.Handle("GET /dashboard", middleware.RequireAuth(http.HandlerFunc(handlers.ShowDashboard)))
	mux.Handle("POST /dashboard/upload", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.WebUploadHandler))))
	mux.Handle("POST /dashboard/create-text", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateTextHandler))))
	mux.Handle("POST /dashboard/files/delete", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteFileHandler))))
	mux.Handle("POST /dashboard/users/create", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateUserHandler)))))
	mux.Handle("POST /dashboard/users/toggle", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.ToggleUserStatusHandler)))))
	mux.Handle("POST /dashboard/users/delete", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteUserHandler)))))
	mux.Handle("POST /dashboard/users/regenerate-key", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.RegenerateAPIKeyHandler))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Test 1: GET /static/css/style.css should return 200 and CSS content
	req := httptest.NewRequest("GET", "/static/css/style.css", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 for /static/css/style.css, got %d", rr.Code)
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Fatalf("Expected non-empty CSS body")
	}

	// Test 2: GET / should return 303 redirect to /login
	reqRoot := httptest.NewRequest("GET", "/", nil)
	rrRoot := httptest.NewRecorder()
	mux.ServeHTTP(rrRoot, reqRoot)

	if rrRoot.Code != http.StatusSeeOther {
		t.Fatalf("Expected 303 for root, got %d", rrRoot.Code)
	}
}
