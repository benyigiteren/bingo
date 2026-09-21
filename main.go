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

	// Static Assets Server (Embedded filesystem with fallback to disk).
	// Directory listings are disabled: directory paths return 404 instead of
	// exposing the asset tree.
	noListFileServer := func(root http.FileSystem) http.Handler {
		srv := http.FileServer(root)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/") {
				http.NotFound(w, r)
				return
			}
			srv.ServeHTTP(w, r)
		})
	}
	staticSub, err := fs.Sub(embeddedFS, "static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", noListFileServer(http.FS(staticSub))))
	} else {
		mux.Handle("/static/", http.StripPrefix("/static/", noListFileServer(http.Dir("./static"))))
	}

	// Setup & Admin Initialization
	mux.HandleFunc("GET /register", handlers.ShowSetup)
	mux.Handle("POST /register", middleware.RateLimitLogin(http.HandlerFunc(handlers.ProcessSetup)))

	// Auth Actions (login POST strictly rate-limited against brute-force)
	mux.HandleFunc("GET /login", handlers.ShowLogin)
	mux.Handle("POST /login", middleware.RateLimitLogin(http.HandlerFunc(handlers.ProcessLogin)))
	// Logout is POST-only with CSRF (GET would allow cross-site forced logout).
	mux.Handle("POST /logout", middleware.RequireCSRF(http.HandlerFunc(handlers.ProcessLogout)))

	// Developer / API Specs & MCP Endpoints
	mux.HandleFunc("GET /api", handlers.ShowAPIDocs)
	mux.Handle("POST /api/upload", middleware.RateLimit(http.HandlerFunc(handlers.APIUploadHandler)))

	// Model Context Protocol (MCP) Endpoints (universal HTTP SSE, Streamable HTTP & direct JSON-RPC)
	// All MCP routes share a dedicated limiter (expensive JSON-RPC + file I/O).
	mcpLimited := middleware.RateLimitMCP(http.HandlerFunc(handlers.MCPHandler))
	mux.Handle("GET /mcp", mcpLimited)
	mux.Handle("POST /mcp", mcpLimited)
	mux.Handle("HEAD /mcp", mcpLimited)
	mux.Handle("DELETE /mcp", mcpLimited)
	mux.Handle("GET /sse", mcpLimited)
	mux.Handle("GET /mcp/sse", mcpLimited)
	mux.Handle("POST /mcp/messages", mcpLimited)
	mux.Handle("POST /messages", mcpLimited)
	// CORS preflights are cheap and must not consume rate-limit budget.
	mux.HandleFunc("OPTIONS /mcp", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp/messages", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /sse", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /mcp/sse", handlers.MCPHandler)
	mux.HandleFunc("OPTIONS /messages", handlers.MCPHandler)

	// MCP Discovery & Well-Known Endpoints (RFC 8615 & MCP Specification)
	mcpDiscoveryLimited := middleware.RateLimitMCP(http.HandlerFunc(handlers.MCPDiscoveryHandler))
	mux.Handle("GET /.well-known/mcp", mcpDiscoveryLimited)
	mux.Handle("GET /.well-known/mcp.json", mcpDiscoveryLimited)
	mux.Handle("GET /mcp/manifest.json", mcpDiscoveryLimited)
	mux.HandleFunc("OPTIONS /.well-known/mcp", handlers.MCPDiscoveryHandler)
	mux.HandleFunc("OPTIONS /.well-known/mcp.json", handlers.MCPDiscoveryHandler)

	// User Workspace & Dashboard Actions
	// Authenticated writes pass through a dedicated limiter (abuse/DoS shield).
	mux.Handle("GET /dashboard", middleware.RequireAuth(http.HandlerFunc(handlers.ShowDashboard)))
	mux.Handle("POST /dashboard/upload", middleware.RequireAuth(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.WebUploadHandler)))))
	mux.Handle("POST /dashboard/create-text", middleware.RequireAuth(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateTextHandler)))))
	mux.Handle("POST /dashboard/files/delete", middleware.RequireAuth(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteFileHandler)))))
	mux.Handle("POST /dashboard/users/create", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.CreateUserHandler))))))
	mux.Handle("POST /dashboard/users/toggle", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.ToggleUserStatusHandler))))))
	mux.Handle("POST /dashboard/users/delete", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.DeleteUserHandler))))))
	mux.Handle("POST /dashboard/users/regenerate-key", middleware.RequireAuth(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.RegenerateAPIKeyHandler)))))
	mux.Handle("POST /dashboard/user/change-password", middleware.RequireAuth(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.ChangeSelfPasswordHandler)))))
	mux.Handle("POST /dashboard/settings/update", middleware.RequireAuth(middleware.RequireSuperAdmin(middleware.RateLimitWrite(middleware.RequireCSRF(http.HandlerFunc(handlers.UpdateSettingsHandler))))))

	// Catch-all Root Route (public file shares are rate-limited to slow
	// password-guessing and scraping).
	fileShareHandler := middleware.RateLimitFile(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	mux.Handle("/", fileShareHandler)

	// Coarse global rate limit so no endpoint is ever fully unprotected.
	limitedMux := middleware.RateLimitGlobal(mux)

	// sanitizeLogValue strips CR/LF to block log-injection via crafted paths.
	sanitizeLogValue := func(s string) string {
		s = strings.ReplaceAll(s, "\r", "")
		s = strings.ReplaceAll(s, "\n", "")
		if len(s) > 512 {
			s = s[:512]
		}
		return s
	}

	// Wrap server with a simple global logging and security header middleware
	serverHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TRACE/TRACK have no legitimate use here and enable cross-site
		// tracing (XST) cookie theft in legacy clients.
		if r.Method == http.MethodTrace || strings.ToUpper(r.Method) == "TRACK" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Log requests briefly (path sanitized against log injection)
		log.Printf("%s %s from %s", sanitizeLogValue(r.Method), sanitizeLogValue(r.URL.Path), sanitizeLogValue(r.RemoteAddr))

		// Set default security headers
		w.Header().Set("Server", "Bingo")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// HSTS only makes sense over HTTPS; behind a TLS-terminating proxy the
		// header is set when the original scheme was https.
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Uniform CSP for HTML pages. File-download responses ignore CSP;
		// static assets are exempt so caching/CDN behavior is untouched.
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://unpkg.com; font-src 'self' https://fonts.gstatic.com https://unpkg.com data:; script-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		}

		limitedMux.ServeHTTP(w, r)
	})

	// 7. Start Server with timeouts (Slowloris / slow-read DoS protection)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           serverHandler,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}
	fmt.Printf("Bingo platform running on :%s...\n", port)
	log.Fatal(srv.ListenAndServe())
}
