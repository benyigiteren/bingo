package handlers

import (
	"encoding/json"
	"gotree/db"
	"gotree/middleware"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

// AuthViewModel giriş ve kurulum ekranlarına hata mesajı aktarmak için kullanılır.
type AuthViewModel struct {
	Error string
}

// SetupGet kurulum ekranını gösterir.
func SetupGet(w http.ResponseWriter, r *http.Request) {
	has, err := db.HasUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if has {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	RenderTemplate(w, "setup.html", nil)
}

// SetupPost kurulum formunu işler ve ilk yöneticiyi oluşturur.
func SetupPost(w http.ResponseWriter, r *http.Request) {
	has, err := db.HasUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if has {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	if username == "" || password == "" {
		RenderTemplate(w, "setup.html", AuthViewModel{Error: "Kullanıcı adı ve şifre boş bırakılamaz."})
		return
	}

	if password != passwordConfirm {
		RenderTemplate(w, "setup.html", AuthViewModel{Error: "Şifreler birbiriyle uyuşmuyor."})
		return
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		RenderTemplate(w, "setup.html", AuthViewModel{Error: "Şifre işlenirken hata oluştu."})
		return
	}

	// İlk kayıt olan kullanıcı role = 'superadmin' olur.
	_, err = db.CreateUser(username, string(hashedBytes), "superadmin")
	if err != nil {
		RenderTemplate(w, "setup.html", AuthViewModel{Error: "Yönetici oluşturulurken hata: " + err.Error()})
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// LoginGet giriş ekranını gösterir.
func LoginGet(w http.ResponseWriter, r *http.Request) {
	has, err := db.HasUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !has {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	RenderTemplate(w, "login.html", nil)
}

// LoginPost giriş işlemini kontrol eder ve oturum oluşturur.
func LoginPost(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := db.GetUserByUsername(username)
	if err != nil {
		RenderTemplate(w, "login.html", AuthViewModel{Error: "Sistem hatası: " + err.Error()})
		return
	}

	if user == nil {
		RenderTemplate(w, "login.html", AuthViewModel{Error: "Geçersiz kullanıcı adı veya şifre."})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		RenderTemplate(w, "login.html", AuthViewModel{Error: "Geçersiz kullanıcı adı veya şifre."})
		return
	}

	// 24 saat geçerli oturum oluştur
	token, err := db.CreateSession(user.ID, 24*time.Hour)
	if err != nil {
		RenderTemplate(w, "login.html", AuthViewModel{Error: "Oturum oluşturulurken hata oluştu."})
		return
	}

	// Çerez ayarla
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   false, // HTTP üzerinden de yerelde çalışabilmesi için false
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// Logout oturumu kapatır ve çerezi siler.
func Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		_ = db.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ProfileUpdatePost kullanıcı adı ve/veya şifre değişikliğini işler.
func ProfileUpdatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Yetkisiz işlem"})
		return
	}

	newUsername := r.FormValue("username")
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	newPasswordConfirm := r.FormValue("new_password_confirm")

	// Kullanıcı adı değişikliği
	if newUsername != "" && newUsername != user.Username {
		existingUser, _ := db.GetUserByUsername(newUsername)
		if existingUser != nil {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bu kullanıcı adı zaten kullanılıyor"})
			return
		}
		err := db.UpdateUsername(user.ID, newUsername)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Kullanıcı adı güncellenemedi"})
			return
		}
		user.Username = newUsername
	}

	// Şifre değişikliği
	if newPassword != "" {
		if currentPassword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Mevcut şifrenizi girin"})
			return
		}
		if newPassword != newPasswordConfirm {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Yeni şifreler birbiriyle uyuşmuyor"})
			return
		}
		if len(newPassword) < 6 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Şifre en az 6 karakter olmalı"})
			return
		}

		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Mevcut şifre hatalı"})
			return
		}

		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Şifre işlenirken hata oluştu"})
			return
		}

		err = db.UpdatePassword(user.ID, string(hashedBytes))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Şifre güncellenemedi"})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Profil başarıyla güncellendi",
		"username": user.Username,
	})
}
