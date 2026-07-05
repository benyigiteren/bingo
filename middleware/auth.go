package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"bingo/db"
)

type contextKey string

const UserContextKey contextKey = "user"
const SessionCookieName = "bingo_session"

// Simple thread-safe in-memory session store
var (
	sessions   = make(map[string]sessionInfo)
	sessionMux sync.RWMutex
)

type sessionInfo struct {
	UserID    int64
	ExpiresAt time.Time
	CsrfToken string
}

func GenerateSessionToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func CreateSession(w http.ResponseWriter, userID int64) string {
	token := GenerateSessionToken()
	csrfToken := GenerateSessionToken()
	expires := time.Now().Add(24 * time.Hour)

	sessionMux.Lock()
	sessions[token] = sessionInfo{
		UserID:    userID,
		ExpiresAt: expires,
		CsrfToken: csrfToken,
	}
	sessionMux.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   false, // Set to true in prod with HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	return token
}

func DestroySession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err == nil {
		sessionMux.Lock()
		delete(sessions, cookie.Value)
		sessionMux.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// CleanupSessions runs periodically to remove expired sessions
func CleanupSessions() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			sessionMux.Lock()
			now := time.Now()
			for token, info := range sessions {
				if now.After(info.ExpiresAt) {
					delete(sessions, token)
				}
			}
			sessionMux.Unlock()
		}
	}()
}

// RequireAuth middleware checks session cookie and populates context with user
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		sessionMux.RLock()
		info, exists := sessions[cookie.Value]
		sessionMux.RUnlock()

		if !exists || time.Now().After(info.ExpiresAt) {
			if exists {
				sessionMux.Lock()
				delete(sessions, cookie.Value)
				sessionMux.Unlock()
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := db.GetUserByID(info.UserID)
		if err != nil || user == nil || !user.IsActive {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSuperAdmin checks if the logged-in user is a super admin
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*db.User)
		if !ok || user == nil || user.Role != "super_admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext helper function
func GetUserFromContext(r *http.Request) *db.User {
	if user, ok := r.Context().Value(UserContextKey).(*db.User); ok {
		return user
	}
	return nil
}

// GetLoggedUser checks session cookie and fetches user if session is valid
func GetLoggedUser(r *http.Request) *db.User {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil
	}

	sessionMux.RLock()
	info, exists := sessions[cookie.Value]
	sessionMux.RUnlock()

	if !exists || time.Now().After(info.ExpiresAt) {
		return nil
	}

	user, err := db.GetUserByID(info.UserID)
	if err != nil || user == nil || !user.IsActive {
		return nil
	}

	return user
}

// GetCsrfToken retrieves the CSRF token associated with the request's session
func GetCsrfToken(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}

	sessionMux.RLock()
	info, exists := sessions[cookie.Value]
	sessionMux.RUnlock()

	if !exists || time.Now().After(info.ExpiresAt) {
		return ""
	}

	return info.CsrfToken
}

// VerifyCSRF checks if the CSRF token in the request matches the session's CSRF token
func VerifyCSRF(r *http.Request) bool {
	// Skip validation for safe HTTP methods
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || r.Method == http.MethodTrace {
		return true
	}

	expectedToken := GetCsrfToken(r)
	if expectedToken == "" {
		return false
	}

	actualToken := r.FormValue("csrf_token")
	if actualToken == "" {
		actualToken = r.Header.Get("X-CSRF-Token")
	}

	return actualToken != "" && actualToken == expectedToken
}

// RequireCSRF middleware blocks requests that do not pass CSRF check
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !VerifyCSRF(r) {
			http.Error(w, "Yasaklı İstek - CSRF Doğrulama Hatası (Forbidden - CSRF Token Mismatch)", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
