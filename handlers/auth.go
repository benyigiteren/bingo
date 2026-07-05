package handlers

import (
	"net/http"
	"strings"

	"bingo/db"
	"bingo/middleware"
	"golang.org/x/crypto/bcrypt"
)

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
		"Title": "Bingo - İlk Kurulum",
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

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")

	if username == "" || len(password) < 6 {
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title": "Bingo - İlk Kurulum",
			"Error": "Kullanıcı adı boş olamaz ve şifre en az 6 karakterden oluşmalıdır.",
		})
		return
	}

	user, err := db.CreateUser(username, password, "super_admin")
	if err != nil {
		RenderTemplate(w, "setup.html", map[string]interface{}{
			"Title": "Bingo - İlk Kurulum",
			"Error": "Süper yönetici oluşturulamadı: " + err.Error(),
		})
		return
	}

	// Oturum oluştur
	middleware.CreateSession(w, user.ID)

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
		"Title": "Bingo - Giriş Yap",
	})
}

// ProcessLogin giriş kimlik doğrulamalarını yapar
func ProcessLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")

	if username == "" || password == "" {
		RenderTemplate(w, "login.html", map[string]interface{}{
			"Title": "Bingo - Giriş Yap",
			"Error": "Kullanıcı adı ve şifre alanları zorunludur.",
		})
		return
	}

	user, err := db.GetUserByUsername(username)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if user == nil || !user.IsActive {
		RenderTemplate(w, "login.html", map[string]interface{}{
			"Title": "Bingo - Giriş Yap",
			"Error": "Geçersiz giriş bilgileri veya devre dışı bırakılmış hesap.",
		})
		return
	}

	// Şifreyi doğrula
	err = db.DB.QueryRow("SELECT password_hash FROM users WHERE id = ?", user.ID).Scan(&user.PasswordHash)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		RenderTemplate(w, "login.html", map[string]interface{}{
			"Title": "Bingo - Giriş Yap",
			"Error": "Geçersiz giriş bilgileri veya devre dışı bırakılmış hesap.",
		})
		return
	}

	// Oturum oluştur
	middleware.CreateSession(w, user.ID)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ProcessLogout oturumu kapatır
func ProcessLogout(w http.ResponseWriter, r *http.Request) {
	middleware.DestroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
