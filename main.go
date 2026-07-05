package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bingo/db"
	"bingo/handlers"
	"bingo/middleware"
)

func main() {
	// 1. Load Configurations from Env
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "bingo.db")
	}

	// 2. Ensure Directories exist
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatalf("Failed to create uploads directory: %v", err)
	}

	// 3. Initialize SQLite database (WAL enabled, schema created)
	if err := db.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.DB.Close()

	// 4. Initialize HTML templates
	if err := handlers.InitTemplates("templates"); err != nil {
		log.Fatalf("Failed to initialize templates: %v", err)
	}

	// 5. Start background memory cleanups
	middleware.CleanupSessions()
	middleware.CleanUpLimiters()

	// 6. Router Setup (Go 1.22+ Standard Mux Routing)
	mux := http.NewServeMux()

	// Static Assets Server
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))

	// Setup & Admin Initialization
	mux.HandleFunc("GET /register", handlers.ShowSetup)
	mux.HandleFunc("POST /register", handlers.ProcessSetup)

	// Auth Actions
	mux.HandleFunc("GET /login", handlers.ShowLogin)
	mux.HandleFunc("POST /login", handlers.ProcessLogin)
	mux.HandleFunc("GET /logout", handlers.ProcessLogout)

	// Developer / API Specs
	mux.HandleFunc("GET /api", handlers.ShowAPIDocs)
	mux.Handle("POST /api/upload", middleware.RateLimit(http.HandlerFunc(handlers.APIUploadHandler)))

	// User Workspace & Dashboard Actions
	mux.Handle("GET /dashboard", middleware.RequireAuth(http.HandlerFunc(handlers.ShowDashboard)))
	mux.Handle("POST /dashboard/upload", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.WebUploadHandler))))
	mux.Handle("POST /dashboard/create-text", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateTextHandler))))
	mux.Handle("POST /dashboard/files/delete", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteFileHandler))))
	mux.Handle("POST /dashboard/users/create", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateUserHandler)))))
	mux.Handle("POST /dashboard/users/toggle", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.ToggleUserStatusHandler)))))
	mux.Handle("POST /dashboard/users/delete", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteUserHandler)))))
	mux.Handle("POST /dashboard/users/regenerate-key", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.RegenerateAPIKeyHandler))))

	// Catch-all Root Route
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// Check if this is a user file share request (e.g. /username/filename.ext)
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) == 2 {
				handlers.ServeFile(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		// If user is already logged in, redirect to workspace. Else, login.
		user := middleware.GetLoggedUser(r)
		if user != nil {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Wrap server with a simple global logging and security header middleware
	serverHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log requests briefly
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Set default security headers
		w.Header().Set("Server", "Bingo")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		
		// If request is for a user file, don't restrict content types via CSP too harshly (e.g. scripts/styles might be served raw if desired)
		if !strings.HasPrefix(r.URL.Path, "/static/") && !strings.Contains(r.URL.Path, ".") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data: https:;")
		}

		mux.ServeHTTP(w, r)
	})

	// 7. Start Server
	fmt.Printf("Bingo platform running on :%s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, serverHandler))
}
