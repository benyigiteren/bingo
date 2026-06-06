package middleware

import (
	"context"
	"net/http"
	"strings"
	"gotree/db"
)

type contextKey string

const UserKey contextKey = "user"

// GetUserContext istek context'inden kullanıcıyı döner.
func GetUserContext(r *http.Request) *db.User {
	u, _ := r.Context().Value(UserKey).(*db.User)
	return u
}

// Authenticate hem oturum çerezini hem de X-API-Key başlığını/parametresini kontrol eder.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var apiKey string
		if headerKey := r.Header.Get("X-API-Key"); headerKey != "" {
			apiKey = headerKey
		} else if queryKey := r.URL.Query().Get("api_key"); queryKey != "" {
			apiKey = queryKey
		}

		var user *db.User
		var err error
		isAPIRequest := strings.HasPrefix(r.URL.Path, "/api/")

		if apiKey != "" {
			user, err = db.GetUserByAPIKey(apiKey)
			if err != nil || user == nil {
				// Sadece API isteklerinde yetkisiz hatası dön, web isteklerinde hata gösterme
				if isAPIRequest {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error":"Geçersiz veya süresi dolmuş API Anahtarı"}`))
					return
				}
			}
		}

		// API anahtarı bulunamadı veya geçersiz ise ve istek API isteği değilse normal oturum çerezini kontrol et
		if user == nil {
			cookie, err := r.Cookie("session_token")
			if err == nil {
				user, _ = db.GetUserBySessionToken(cookie.Value)
				if user == nil {
					// Geçersiz çerezi sil
					http.SetCookie(w, &http.Cookie{
						Name:     "session_token",
						Value:    "",
						Path:     "/",
						MaxAge:   -1,
						HttpOnly: true,
					})
				}
			}
		}

		if user != nil {
			ctx := context.WithValue(r.Context(), UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAuth oturum açmış kullanıcıları veya API isteği ise API ile kimlik doğrulamış kullanıcıları zorunlu kılar.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserContext(r)
		if user == nil {
			isAPIRequest := strings.HasPrefix(r.URL.Path, "/api/")
			if isAPIRequest {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"Oturum açmanız veya X-API-Key belirtmeniz gerekir"}`))
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSuperAdmin sadece 'superadmin' rolündekilerin girmesine izin verir.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserContext(r)
		if user == nil {
			isAPIRequest := strings.HasPrefix(r.URL.Path, "/api/")
			if isAPIRequest {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"Yetkisiz işlem: Sadece Super Admin erişebilir"}`))
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if user.Role != "superadmin" {
			isAPIRequest := strings.HasPrefix(r.URL.Path, "/api/")
			if isAPIRequest {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"Yetkisiz işlem: Sadece Super Admin erişebilir"}`))
				return
			}
			http.Error(w, "Yetkisiz Erişim (Sadece Super Admin)", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
