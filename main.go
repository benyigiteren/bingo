package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bingo/db"
	"bingo/handlers"
	"bingo/mcp"
	"bingo/middleware"
)

//go:embed static templates
var embeddedFS embed.FS

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

	// 3.1 Check for MCP CLI mode: 'bingo mcp', 'bingo --mcp', etc.
	isMCPCli := false
	for _, arg := range os.Args[1:] {
		if arg == "mcp" || arg == "--mcp" || arg == "-mcp" {
			isMCPCli = true
			break
		}
	}

	if isMCPCli {
		apiKey := ""
		for i, arg := range os.Args {
			if strings.HasPrefix(arg, "--api-key=") {
				apiKey = strings.TrimPrefix(arg, "--api-key=")
			} else if (arg == "--api-key" || arg == "-api-key" || arg == "--key" || arg == "-key" || arg == "-k") && i+1 < len(os.Args) {
				apiKey = os.Args[i+1]
			} else if strings.HasPrefix(arg, "--key=") || strings.HasPrefix(arg, "-key=") {
				apiKey = strings.TrimPrefix(strings.TrimPrefix(arg, "--key="), "-key=")
			}
		}
		if apiKey == "" {
			apiKey = os.Getenv("BINGO_API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("MCP_API_KEY")
		}

		var user *db.User
		var err error
		if apiKey != "" {
			user, err = db.GetUserByAPIKey(apiKey)
			if err != nil || user == nil {
				fmt.Fprintf(os.Stderr, "Error: Invalid API key: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Auto-fallback: if only 1 user exists in local database, use it automatically for local stdio
			users, err := db.GetUsers()
			if err == nil && len(users) > 0 {
				user = &users[0]
			} else {
				fmt.Fprintln(os.Stderr, "Error: Missing API key. Provide via --api-key=<key> or BINGO_API_KEY environment variable.")
				os.Exit(1)
			}
		}

		baseURL := os.Getenv("BINGO_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:" + port
		}

		mcp.HandleStdio(user, baseURL, "uploads")
		return
	}

	// 4. Initialize HTML templates (embedded filesystem for zero-disk dependency)
	if err := handlers.InitEmbeddedTemplates(embeddedFS, "templates"); err != nil {
		log.Printf("Embedded templates init fallback to disk: %v", err)
		if err := handlers.InitTemplates("templates"); err != nil {
			log.Fatalf("Failed to initialize templates: %v", err)
		}
	}

	// 5. Start background memory and TTL cleanups
	middleware.CleanupSessions()
	middleware.CleanUpLimiters()

	// 5.1 Start background TTL file cleanup (runs every 1 minute)
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_, _ = db.DeleteExpiredFiles("uploads")
		}
	}()

	// 6. Router Setup (Go 1.22+ Standard Mux Routing)
	mux := http.NewServeMux()

	// Static Assets Server (Embedded filesystem with fallback to disk)
	staticSub, err := fs.Sub(embeddedFS, "static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	} else {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	}

	// Setup & Admin Initialization
	mux.HandleFunc("GET /register", handlers.ShowSetup)
	mux.HandleFunc("POST /register", handlers.ProcessSetup)

	// Auth Actions
	mux.HandleFunc("GET /login", handlers.ShowLogin)
	mux.HandleFunc("POST /login", handlers.ProcessLogin)
	mux.HandleFunc("GET /logout", handlers.ProcessLogout)

	// Developer / API Specs & MCP Endpoints
	mux.HandleFunc("GET /api", handlers.ShowAPIDocs)
	mux.Handle("POST /api/upload", middleware.RateLimit(http.HandlerFunc(handlers.APIUploadHandler)))

	// Model Context Protocol (MCP) Endpoints (universal HTTP SSE, Streamable HTTP & direct JSON-RPC)
	mux.HandleFunc("GET /mcp", handlers.MCPHandler)
	mux.HandleFunc("POST /mcp", handlers.MCPHandler)
	mux.HandleFunc("HEAD /mcp", handlers.MCPHandler)
	mux.HandleFunc("DELETE /mcp", handlers.MCPHandler)
	mux.HandleFunc("GET /sse", handlers.MCPHandler)
	mux.HandleFunc("GET /mcp/sse", handlers.MCPHandler)
	mux.HandleFunc("POST /mcp/messages", handlers.MCPHandler)
	mux.HandleFunc("POST /messages", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp/messages", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /sse", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp/sse", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /messages", handlers.MCPHandler)

	// MCP Discovery & Well-Known Endpoints (RFC 8615 & MCP Specification)
	mux.HandleFunc("GET /.well-known/mcp", handlers.MCPDiscoveryHandler)
	mux.HandleFunc("GET /.well-known/mcp.json", handlers.MCPDiscoveryHandler)
	mux.HandleFunc("GET /mcp/manifest.json", handlers.MCPDiscoveryHandler)
	mux.HandleFunc("OPTIONS /.well-known/mcp", handlers.MCPDiscoveryHandler)
	mux.HandleFunc("OPTIONS /.well-known/mcp.json", handlers.MCPDiscoveryHandler)

	// User Workspace & Dashboard Actions
	mux.Handle("GET /dashboard", middleware.RequireAuth(http.HandlerFunc(handlers.ShowDashboard)))
	mux.Handle("POST /dashboard/upload", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.WebUploadHandler))))
	mux.Handle("POST /dashboard/create-text", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateTextHandler))))
	mux.Handle("POST /dashboard/files/delete", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteFileHandler))))
	mux.Handle("POST /dashboard/users/create", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateUserHandler)))))
	mux.Handle("POST /dashboard/users/toggle", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.ToggleUserStatusHandler)))))
	mux.Handle("POST /dashboard/users/delete", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteUserHandler)))))
	mux.Handle("POST /dashboard/users/regenerate-key", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.RegenerateAPIKeyHandler))))
	mux.Handle("POST /dashboard/user/change-password", middleware.RequireAuth(middleware.RequireCSRF(http.HandlerFunc(handlers.ChangeSelfPasswordHandler))))
	mux.Handle("POST /dashboard/settings/update", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RequireCSRF(http.HandlerFunc(handlers.UpdateSettingsHandler)))))

	// Catch-all Root Route
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://unpkg.com; font-src 'self' https://fonts.gstatic.com https://unpkg.com data:; script-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https:;")
		}

		mux.ServeHTTP(w, r)
	})

	// 7. Start Server
	fmt.Printf("Bingo platform running on :%s...\n", port)
	log.Fatal(http.ListenAndServe(":"+port, serverHandler))
}
