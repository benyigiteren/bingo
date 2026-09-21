package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"bingo/db"
	"bingo/middleware"
	"golang.org/x/crypto/bcrypt"
)

var (
	validUsernameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,30}[a-z0-9]$`)

	// dummyPasswordHash equalizes login timing when the username does not
	// exist, preventing user-enumeration via response time. Generated once at
	// startup with the same bcrypt cost as real passwords.
	dummyPasswordHash []byte
)

func init() {
	// Best effort: if hashing fails, fall back to a hard-coded DefaultCost hash.
	hash, err := bcrypt.GenerateFromPassword([]byte("bingo-nonexistent-user-dummy-compare"), bcrypt.DefaultCost)
	if err != nil {
		dummyPasswordHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")
		return
	}
	dummyPasswordHash = hash
}

// reservedUsernames cannot be registered because they collide with site routes
// (/api, /static, /mcp, ...) or filesystem conventions.
var reservedUsernames = map[string]bool{
	"api": true, "static": true, "dashboard": true, "login": true,
	"register": true, "logout": true, "mcp": true, "sse": true,
	"messages": true, "uploads": true, "data": true, "db": true,
	"assets": true, "templates": true, "well-known": true,
	"favicon.ico": true, "robots.txt": true, "manifest": true,
	"manifest.json": true, ".well-known": true,
}

// ValidateUsername checks format (3-32 chars, [a-z0-9._-]) and reserved words.
// Returns empty string when valid, otherwise a user-facing error message.
func ValidateUsername(username string) string {
	if len(username) < 3 || len(username) > 32 {
		return "Kullanıcı adı 3-32 karakter arasında olmalıdır."
	}
	if !validUsernameRegex.MatchString(username) {
		return "Kullanıcı adı küçük harf, rakam, nokta, alt çizgi veya tire içermeli; harf/rakamla başlayıp bitmelidir."
	}
	if reservedUsernames[username] {
		return "Bu kullanıcı adı sistem tarafından rezerve edilmiştir."
	}
	if strings.Contains(username, "..") {
		return "Kullanıcı adı '..' içeremez."
	}
	return ""
}

// ShowSetup ilk açılışta Süper Yönetici kurulum sayfasını gösterir
func ShowSetup(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := db.HasUsers()
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if hasUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	RenderTemplate(w, "setup.html", map[string]interface{}{
		"Title":         "İlk Kurulum",
		"FormCsrfToken": middleware.SetFormCSRF(w, r),
	})
}

// ProcessSetup ilk Süper Yöneticiyi oluşturur
func ProcessSetup(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := db.HasUsers()
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if hasUsers {
		http.Error(w, "Yasak - Kayıtlar kapalıdır", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	// Pre-auth CSRF: blocks cross-site planting of attacker-chosen admin
	// credentials on fresh instances.
	if !middleware.VerifyFormCSRF(r) {
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title":         "İlk Kurulum",
			"Error":         "Form süresi dolmuş veya geçersiz istek. Lütfen tekrar deneyin.",
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
		return
	}

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")

	if msg := ValidateUsername(username); msg != "" {
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title":         "İlk Kurulum",
			"Error":         msg,
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
		return
	}
	if len(password) < 6 || len(password) > 128 {
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title":         "İlk Kurulum",
			"Error":         "Şifre en az 6, en fazla 128 karakterden oluşmalıdır.",
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
		return
	}

	user, err := db.CreateUser(username, password, "super_admin")
	if err != nil {
		// Do not leak raw SQL errors (e.g. UNIQUE constraint) to the client.
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title":         "İlk Kurulum",
			"Error":         "Süper yönetici oluşturulamadı. Kullanıcı adı zaten kullanılıyor olabilir.",
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
		return
	}

	// Single-use form token + fixation rotation, then create the session.
	middleware.ClearFormCSRF(w, r)
	middleware.InvalidateSessionToken(middleware.SessionTokenFromRequest(r))
	// Oturum oluştur
	if middleware.CreateSession(w, r, user.ID) == "" {
		return // CreateSession already wrote 500
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ShowLogin giriş sayfasını gösterir
func ShowLogin(w http.ResponseWriter, r *http.Request) {
	// Oturum zaten varsa ve aktifse doğrudan panele yönlendir
	user := middleware.GetLoggedUser(r)
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	hasUsers, err := db.HasUsers()
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	// Kullanıcı yoksa kuruluma yönlendir
	if !hasUsers {
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	RenderTemplate(w, "login.html", map[string]interface{}{
		"Title":         "Giriş Yap",
		"FormCsrfToken": middleware.SetFormCSRF(w, r),
	})
}

// ProcessLogin giriş kimlik doğrulamalarını yapar
func ProcessLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	loginFailed := func() {
		RenderTemplate(w, "login.html", map[string]interface{}{
			"Title":         "Giriş Yap",
			"Error":         "Geçersiz giriş bilgileri veya devre dışı bırakılmış hesap.",
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
	}

	// Pre-auth CSRF: blocks login-CSRF (attacker logging the victim into the
	// attacker's account to track activity).
	if !middleware.VerifyFormCSRF(r) {
		RenderTemplate(w, "login.html", map[string]interface{}{
			"Title":         "Giriş Yap",
			"Error":         "Form süresi dolmuş veya geçersiz istek. Lütfen tekrar deneyin.",
			"FormCsrfToken": middleware.SetFormCSRF(w, r),
		})
		return
	}

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")

	// Cap lengths before any DB/crypto work (DoS protection on huge inputs).
	if len(username) == 0 || len(username) > 64 || len(password) == 0 || len(password) > 256 {
		loginFailed()
		return
	}

	user, err := db.GetUserByUsername(username)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if user == nil || !user.IsActive {
		// Constant-time dummy bcrypt comparison so that "unknown user" and
		// "wrong password" take the same amount of time (enumeration defense).
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		loginFailed()
		return
	}

	// Şifreyi doğrula (GetUserByUsername already populated PasswordHash).
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		loginFailed()
		return
	}

	// Single-use form token + fixation rotation: kill any pre-login session
	// before minting the authenticated one.
	middleware.ClearFormCSRF(w, r)
	middleware.InvalidateSessionToken(middleware.SessionTokenFromRequest(r))
	// Oturum oluştur
	if middleware.CreateSession(w, r, user.ID) == "" {
		return // CreateSession already wrote 500
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ProcessLogout oturumu kapatır
func ProcessLogout(w http.ResponseWriter, r *http.Request) {
	middleware.DestroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
