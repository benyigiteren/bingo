package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bingo/db"
	"bingo/middleware"
)

// ShowDashboard ana yönetim panelini gösterir
func ShowDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	var totalFiles int64
	var totalSize int64
	var totalViews int64
	var usersList []db.User
	var filesList []db.File

	var err error
	if user.Role == "super_admin" {
		stats, err := db.GetStats()
		if err != nil {
			http.Error(w, "İstatistikler getirilirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
			return
		}
		totalFiles = stats.TotalFiles
		totalSize = stats.TotalSize
		totalViews = stats.TotalViews

		usersList, err = db.GetUsers()
		if err != nil {
			http.Error(w, "Kullanıcı listesi getirilirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filesList, err = db.GetAllFiles(100, 0)
		if err != nil {
			http.Error(w, "Tüm dosyalar getirilirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		err = db.DB.QueryRow(
			"SELECT COUNT(*), COALESCE(SUM(file_size), 0), COALESCE(SUM(views), 0) FROM files WHERE user_id = ?",
			user.ID,
		).Scan(&totalFiles, &totalSize, &totalViews)
		if err != nil {
			http.Error(w, "Kişisel istatistikler getirilirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filesList, err = db.GetFiles(user.ID, 100, 0)
		if err != nil {
			http.Error(w, "Dosyalarınız getirilirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	type UIFile struct {
		db.File
		FormattedSize string
	}

	var uiFiles []UIFile
	for _, f := range filesList {
		uiFiles = append(uiFiles, UIFile{
			File:          f,
			FormattedSize: PrettySize(f.FileSize),
		})
	}

	csrfToken := middleware.GetCsrfToken(r)

	data := map[string]interface{}{
		"Title":      "Bingo - Yönetim Paneli",
		"User":       user,
		"Files":      uiFiles,
		"TotalFiles": totalFiles,
		"TotalSize":  PrettySize(totalSize),
		"TotalViews": totalViews,
		"CsrfToken":  csrfToken, // CSRF token injection
	}

	if user.Role == "super_admin" {
		data["Users"] = usersList
		data["IsAdmin"] = true
	}

	RenderTemplate(w, "dashboard.html", data)
}

// CreateUserHandler Süper Yönetici tarafından yeni kullanıcı ekleme işlemini yapar
func CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	admin := middleware.GetUserFromContext(r)
	if admin == nil || admin.Role != "super_admin" {
		http.Error(w, "Yetkisiz işlem", http.StatusForbidden)
		return
	}

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")

	if username == "" || len(password) < 6 {
		http.Error(w, "Geçersiz kullanıcı adı veya şifre (en az 6 karakter)", http.StatusBadRequest)
		return
	}

	existing, err := db.GetUserByUsername(username)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "Bu kullanıcı adı zaten kullanılmaktadır.", http.StatusConflict)
		return
	}

	_, err = db.CreateUser(username, password, "user")
	if err != nil {
		http.Error(w, "Kullanıcı oluşturulurken hata oluştu: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userDir := filepath.Join("uploads", username)
	_ = os.MkdirAll(userDir, 0755)

	http.Redirect(w, r, "/dashboard#users", http.StatusSeeOther)
}

// ToggleUserStatusHandler kullanıcının aktif/pasif durumunu değiştirir
func ToggleUserStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	admin := middleware.GetUserFromContext(r)
	if admin == nil || admin.Role != "super_admin" {
		http.Error(w, "Yetkisiz işlem", http.StatusForbidden)
		return
	}

	targetUserIDStr := r.FormValue("user_id")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Geçersiz kullanıcı kimliği", http.StatusBadRequest)
		return
	}

	if targetUserID == admin.ID {
		http.Error(w, "Kendi hesabınızın durumunu değiştiremezsiniz.", http.StatusBadRequest)
		return
	}

	targetUser, err := db.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "Kullanıcı bulunamadı", http.StatusNotFound)
		return
	}

	newStatus := !targetUser.IsActive
	err = db.ToggleUserStatus(targetUserID, newStatus)
	if err != nil {
		http.Error(w, "Hesap durumu güncellenirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard#users", http.StatusSeeOther)
}

// DeleteUserHandler kullanıcıyı ve ona ait dosyaları siler
func DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	admin := middleware.GetUserFromContext(r)
	if admin == nil || admin.Role != "super_admin" {
		http.Error(w, "Yetkisiz işlem", http.StatusForbidden)
		return
	}

	targetUserIDStr := r.FormValue("user_id")
	targetUserID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Geçersiz kullanıcı kimliği", http.StatusBadRequest)
		return
	}

	if targetUserID == admin.ID {
		http.Error(w, "Kendi yöneticilik hesabınızı silemezsiniz.", http.StatusBadRequest)
		return
	}

	targetUser, err := db.GetUserByID(targetUserID)
	if err != nil || targetUser == nil {
		http.Error(w, "Kullanıcı bulunamadı", http.StatusNotFound)
		return
	}

	userDir := filepath.Join("uploads", targetUser.Username)
	_ = os.RemoveAll(userDir)

	err = db.DeleteUser(targetUserID)
	if err != nil {
		http.Error(w, "Kullanıcı silinirken veritabanı hatası oluştu.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard#users", http.StatusSeeOther)
}

// RegenerateAPIKeyHandler kullanıcının API anahtarını yeniler
func RegenerateAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	targetUserID := user.ID
	targetUserIDStr := r.FormValue("user_id")
	if targetUserIDStr != "" {
		parsedID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
		if err == nil {
			if user.Role == "super_admin" {
				targetUserID = parsedID
			}
		}
	}

	newKey, err := db.RegenerateAPIKey(targetUserID)
	if err != nil {
		http.Error(w, "API anahtarı yenilenirken hata oluştu.", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"api_key": newKey,
		})
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// DeleteFileHandler paylaşılan bir dosyayı siler
func DeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	fileIDStr := r.FormValue("file_id")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Geçersiz dosya kimliği", http.StatusBadRequest)
		return
	}

	fileMeta, err := db.GetFileByID(fileID)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if fileMeta == nil {
		http.Error(w, "Dosya bulunamadı", http.StatusNotFound)
		return
	}

	if user.Role != "super_admin" && fileMeta.UserID != user.ID {
		http.Error(w, "Bu dosyayı silmek için yetkiniz yok.", http.StatusForbidden)
		return
	}

	targetPath := filepath.Join("uploads", fileMeta.Username, fileMeta.Filename)
	_ = os.Remove(targetPath)

	err = db.DeleteFile(fileID)
	if err != nil {
		http.Error(w, "Dosya kaydı silinirken hata oluştu: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
