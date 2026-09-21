package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bingo/db"
	"bingo/middleware"

	"golang.org/x/crypto/bcrypt"
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
			http.Error(w, "İstatistikler getirilirken hata oluştu.", http.StatusInternalServerError)
			return
		}
		totalFiles = stats.TotalFiles
		totalSize = stats.TotalSize
		totalViews = stats.TotalViews

		usersList, err = db.GetUsers()
		if err != nil {
			http.Error(w, "Kullanıcı listesi getirilirken hata oluştu.", http.StatusInternalServerError)
			return
		}

		filesList, err = db.GetAllFiles(100, 0)
		if err != nil {
			http.Error(w, "Tüm dosyalar getirilirken hata oluştu.", http.StatusInternalServerError)
			return
		}
	} else {
		err = db.DB.QueryRow(
			"SELECT COUNT(*), COALESCE(SUM(file_size), 0), COALESCE(SUM(views), 0) FROM files WHERE user_id = ?",
			user.ID,
		).Scan(&totalFiles, &totalSize, &totalViews)
		if err != nil {
			http.Error(w, "Kişisel istatistikler getirilirken hata oluştu.", http.StatusInternalServerError)
			return
		}

		filesList, err = db.GetFiles(user.ID, 100, 0)
		if err != nil {
			http.Error(w, "Dosyalarınız getirilirken hata oluştu.", http.StatusInternalServerError)
			return
		}
	}

	type UIFile struct {
		db.File
		FormattedSize string
		ViewPercent   int
		IsExpired     bool
		Category      string
		FileExt       string
	}

	maxViews := 1
	for _, f := range filesList {
		if f.Views > maxViews {
			maxViews = f.Views
		}
	}
	if maxViews < 10 {
		maxViews = 10
	}

	now := time.Now()
	var uiFiles []UIFile
	for _, f := range filesList {
		ext := strings.ToLower(filepath.Ext(f.Filename))
		cat := "other"
		codeExts := map[string]bool{
			".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
			".rs": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true, ".java": true,
			".html": true, ".css": true, ".sh": true, ".bash": true, ".sql": true,
			".yaml": true, ".yml": true, ".json": true, ".php": true, ".rb": true,
		}
		textExts := map[string]bool{
			".md": true, ".txt": true, ".log": true, ".env": true, ".ini": true,
			".conf": true, ".csv": true, ".tsv": true, ".xml": true, ".pdf": true,
		}
		imgExts := map[string]bool{
			".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
			".svg": true, ".bmp": true, ".ico": true,
		}

		if codeExts[ext] {
			cat = "code"
		} else if textExts[ext] || ext == "" {
			cat = "text"
		} else if imgExts[ext] {
			cat = "images"
		}

		vPercent := 0
		if f.Views > 0 {
			vPercent = int(float64(f.Views) / float64(maxViews) * 100)
			if vPercent < 8 {
				vPercent = 8
			} else if vPercent > 100 {
				vPercent = 100
			}
		}

		isExpired := f.ExpiresAt != nil && f.ExpiresAt.Before(now)
		displayExt := strings.ToUpper(strings.TrimPrefix(ext, "."))
		if displayExt == "" {
			displayExt = "METİN"
		}

		uiFiles = append(uiFiles, UIFile{
			File:          f,
			FormattedSize: PrettySize(f.FileSize),
			ViewPercent:   vPercent,
			IsExpired:     isExpired,
			Category:      cat,
			FileExt:       displayExt,
		})
	}

	csrfToken := middleware.GetCsrfToken(r)

	// Host header is validated (injection-safe) inside BaseURL.
	baseURL := BaseURL(r)
	mcpURL := fmt.Sprintf("%s/mcp?api_key=%s", baseURL, user.APIKey)

	claudeConfig := fmt.Sprintf(`{
  "mcpServers": {
    "bingo": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "%s"]
    }
  }
}`, mcpURL)

	cursorConfig := fmt.Sprintf(`{
  "mcpServers": {
    "bingo": {
      "url": "%s"
    }
  }
}`, mcpURL)

	geminiCliCmd := fmt.Sprintf("gemini mcp add bingo %s", mcpURL)

	stdioConfig := fmt.Sprintf(`{
  "mcpServers": {
    "bingo": {
      "command": "bingo",
      "args": ["mcp", "--api-key=%s"]
    }
  }
}`, user.APIKey)

	data := map[string]interface{}{
		"Title":        "Çalışma Alanı",
		"User":         user,
		"Files":        uiFiles,
		"TotalFiles":   totalFiles,
		"TotalSize":    PrettySize(totalSize),
		"TotalViews":   totalViews,
		"MaxUploadMB":  GetMaxUploadSizeMB(),
		"CsrfToken":    csrfToken,
		"BaseURL":      baseURL,
		"MCPURL":       mcpURL,
		"ClaudeConfig": claudeConfig,
		"CursorConfig": cursorConfig,
		"GeminiCliCmd": geminiCliCmd,
		"StdioConfig":  stdioConfig,
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

	if msg := ValidateUsername(username); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if len(password) < 6 || len(password) > 128 {
		http.Error(w, "Şifre en az 6, en fazla 128 karakter olmalıdır.", http.StatusBadRequest)
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
		http.Error(w, "Kullanıcı oluşturulurken hata oluştu.", http.StatusInternalServerError)
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
		http.Error(w, "Hesap durumu güncellenirken hata oluştu.", http.StatusInternalServerError)
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

	// Only remove directories for safe usernames; a legacy unsafe name must
	// never cause deletion outside uploads/.
	if safeUserDirName(targetUser.Username) {
		userDir := filepath.Join("uploads", targetUser.Username)
		_ = os.RemoveAll(userDir)
	}

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

	http.Redirect(w, r, "/dashboard#mcp", http.StatusSeeOther)
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
		http.Error(w, "Dosya kaydı silinirken hata oluştu.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// UpdateSettingsHandler sistem ayarlarını günceller
func UpdateSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	admin := middleware.GetUserFromContext(r)
	if admin == nil || admin.Role != "super_admin" {
		http.Error(w, "Yetkisiz işlem", http.StatusForbidden)
		return
	}

	maxMBStr := strings.TrimSpace(r.FormValue("max_upload_size_mb"))
	maxMB, err := strconv.Atoi(maxMBStr)
	if err != nil || maxMB <= 0 || maxMB > 2048 {
		http.Error(w, "Geçersiz dosya boyutu sınırı (1 MB - 2048 MB arası olmalıdır)", http.StatusBadRequest)
		return
	}

	if err := db.SetSetting("max_upload_size_mb", strconv.Itoa(maxMB)); err != nil {
		http.Error(w, "Ayar kaydedilemedi.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard#settings", http.StatusSeeOther)
}

// ChangeSelfPasswordHandler kullanıcının kendi şifresini değiştirmesini sağlar
func ChangeSelfPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Error(w, "Oturum açılmamış", http.StatusUnauthorized)
		return
	}

	isJSON := strings.Contains(r.Header.Get("Accept"), "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest"

	sendResponse := func(success bool, msg string, statusCode int) {
		if isJSON {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": success,
				"message": msg,
				"error":   msg,
			})
			return
		}
		if !success {
			http.Error(w, msg, statusCode)
			return
		}
		http.Redirect(w, r, "/dashboard#settings", http.StatusSeeOther)
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if currentPassword == "" || newPassword == "" {
		sendResponse(false, "Mevcut şifre ve yeni şifre boş bırakılamaz.", http.StatusBadRequest)
		return
	}

	freshUser, err := db.GetUserByID(user.ID)
	if err != nil || freshUser == nil {
		sendResponse(false, "Kullanıcı kaydı bulunamadı.", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(freshUser.PasswordHash), []byte(currentPassword)); err != nil {
		sendResponse(false, "Mevcut şifreniz hatalı.", http.StatusBadRequest)
		return
	}

	if len(newPassword) < 6 || len(newPassword) > 128 {
		sendResponse(false, "Yeni şifre en az 6, en fazla 128 karakter olmalıdır.", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		sendResponse(false, "Yeni şifreler birbiriyle eşleşmiyor.", http.StatusBadRequest)
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		sendResponse(false, "Şifre hashlenirken bir hata oluştu.", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateUserPassword(user.ID, string(newHash)); err != nil {
		sendResponse(false, "Şifre güncellenirken hata oluştu.", http.StatusInternalServerError)
		return
	}

	// Invalidate all other sessions so a stolen session stops working; keep
	// the current one so the user is not logged out by their own change.
	middleware.InvalidateUserSessionsExcept(user.ID, middleware.SessionTokenFromRequest(r))

	sendResponse(true, "Şifreniz başarıyla değiştirildi.", http.StatusOK)
}
