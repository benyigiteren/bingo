package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
	"time"

	"bingo/db"
)

type contextKey string

const UserContextKey contextKey = "user"
const SessionCookieName = "bingo_session"

// FormCSRFCookieName carries the pre-authentication CSRF token for the login
// and initial-setup forms (which have no session yet). Login/setup CSRF would
// otherwise let an attacker plant attacker-chosen credentials (notably a fresh
// instance's first super-admin) via an auto-submitted cross-site form.
const FormCSRFCookieName = "bingo_form_csrf"

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
	if _, err := rand.Read(bytes); err != nil {
		// Fail closed: callers must handle empty token (no session created)
		// rather than falling back to predictable values.
		log.Printf("CRITICAL: crypto/rand failed for session token: %v", err)
		return ""
	}
	return hex.EncodeToString(bytes)
}

// isSecureRequest reports whether the request was received over HTTPS
// (direct TLS or via a trusted proxy header).
func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	// X-Forwarded-Proto is only meaningful behind a proxy that sets it.
	// We accept it for the Secure-cookie decision because the worst case of
	// trusting a spoofed header here is setting Secure on plain HTTP (which
	// just makes the cookie not sent) — fail-safe direction.
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	return false
}

func CreateSession(w http.ResponseWriter, r *http.Request, userID int64) string {
	var token, csrfToken string
	// Retry on (extremely unlikely) entropy failure; fail closed if it persists.
	for i := 0; i < 3; i++ {
		token = GenerateSessionToken()
		csrfToken = GenerateSessionToken()
		if token != "" && csrfToken != "" {
			break
		}
	}
	if token == "" || csrfToken == "" {
		http.Error(w, "Oturum oluşturulamadı, lütfen tekrar deneyin", http.StatusInternalServerError)
		return ""
	}
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
		Secure:   isSecureRequest(r),
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
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// InvalidateUserSessions removes all sessions belonging to a user.
// Used after password change so stolen sessions stop working.
func InvalidateUserSessions(userID int64) {
	InvalidateUserSessionsExcept(userID, "")
}

// InvalidateUserSessionsExcept removes all sessions of a user except the one
// holding keepToken (typically the current session, so the user is not logged
// out by their own password change).
func InvalidateUserSessionsExcept(userID int64, keepToken string) {
	sessionMux.Lock()
	defer sessionMux.Unlock()
	for token, info := range sessions {
		if info.UserID == userID && token != keepToken {
			delete(sessions, token)
		}
	}
}

// SessionTokenFromRequest extracts the raw session cookie value, or "".
func SessionTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	if len(cookie.Value) > 256 {
		return ""
	}
	return cookie.Value
}

// InvalidateSessionToken deletes one session by its raw token (used to rotate
// away a pre-login session and kill fixation attempts).
func InvalidateSessionToken(token string) {
	if token == "" {
		return
	}
	sessionMux.Lock()
	delete(sessions, token)
	sessionMux.Unlock()
}

// SetFormCSRF issues a short-lived pre-auth CSRF token: the token is stored in
// a SameSite cookie and must be echoed back in the form body. Returns the token
// to embed in the rendered form.
func SetFormCSRF(w http.ResponseWriter, r *http.Request) string {
	token := GenerateSessionToken()
	if token == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     FormCSRFCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   600, // 10 minutes: enough to fill the form, short for replay
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

// VerifyFormCSRF constant-time compares the form token against the cookie.
func VerifyFormCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(FormCSRFCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	actual := r.FormValue("csrf_token")
	if actual == "" {
		actual = r.Header.Get("X-CSRF-Token")
	}
	if actual == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(cookie.Value)) == 1
}

// ClearFormCSRF removes the pre-auth token cookie (single-use after success).
func ClearFormCSRF(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     FormCSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
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

	if actualToken == "" {
		return false
	}
	// Constant-time comparison to avoid leaking token prefix via timing.
	return subtle.ConstantTimeCompare([]byte(actualToken), []byte(expectedToken)) == 1
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
