package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bingo/db"
	"bingo/middleware"
)

const MaxUploadSize = 100 * 1024 * 1024 // 100 MB varsayılan limit

var (
	// Güvenli dosya adı kontrolü
	safeFilenameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
)

// Dosya adını temizleme yardımcısı
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "_")

	// Güvensiz karakterleri filtrele
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if res == "" {
		res = "dosya_" + fmt.Sprintf("%d", time.Now().Unix())
	}
	return res
}

// Çakışmayan benzersiz dosya adı bulma yardımcısı
func getUniqueFilename(userID int64, username, originalName string) string {
	ext := filepath.Ext(originalName)
	base := originalName[:len(originalName)-len(ext)]
	base = sanitizeFilename(base)
	if base == "" {
		base = "dosya"
	}
	ext = sanitizeFilename(ext)

	filename := base + ext
	counter := 1

	for {
		existing, err := db.GetFile(username, filename)
		if err != nil || existing == nil {
			break
		}
		filename = fmt.Sprintf("%s_%d%s", base, counter, ext)
		counter++
	}
	return filename
}

// API İsteğini Doğrulama Yardımcısı
func authenticateAPIRequest(r *http.Request) (*db.User, error) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("API anahtarı eksik (missing API key)")
	}

	user, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, fmt.Errorf("geçersiz API anahtarı (invalid API key)")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("kullanıcı hesabı devre dışı bırakılmış (user account is deactivated)")
	}

	return user, nil
}

// WebUploadHandler panelden dosya yükleme işlemlerini yönetir
func WebUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r)
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success": false, "error": "Yetkisiz işlem (Unauthorized)"}`))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	err := r.ParseMultipartForm(MaxUploadSize)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success": false, "error": "Dosya boyutu 100MB sınırını aşıyor"}`))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success": false, "error": "Yüklenecek dosya seçilmedi"}`))
		return
	}
	defer file.Close()

	userDir := filepath.Join("uploads", user.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success": false, "error": "Dizin oluşturulamadı"}`))
		return
	}

	filename := getUniqueFilename(user.ID, user.Username, header.Filename)
	targetPath := filepath.Join(userDir, filename)

	outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success": false, "error": "Dosya kaydedilemedi"}`))
		return
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success": false, "error": "Dosya yazılamadı"}`))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	ttlStr := r.FormValue("ttl")
	expiresAt := parseTTLDuration(ttlStr)
	isBurn := r.FormValue("is_burn") == "1" || r.FormValue("is_burn") == "true"
	password := strings.TrimSpace(r.FormValue("password"))

	dbFile, err := db.CreateFileWithOpts(user.ID, filename, header.Filename, written, mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
		IsBurn:    isBurn,
		Password:  password,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success": false, "error": "Veritabanı hatası: ` + err.Error() + `"}`))
		return
	}

	publicURL := fmt.Sprintf("/%s/%s", user.Username, dbFile.Filename)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"url":     publicURL,
		"file":    dbFile,
	})
}

// CreateTextHandler paneldeki metin editöründen gelen paylaşımları kaydeder
func CreateTextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Yöntem izin verilmedi", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	filename := strings.TrimSpace(r.FormValue("filename"))
	content := r.FormValue("content")

	if filename == "" || content == "" {
		http.Error(w, "Dosya adı veya metin içeriği boş olamaz.", http.StatusBadRequest)
		return
	}

	// Eğer kullanıcı dosya adına uzantı yazmadıysa, arayüzden seçilen uzantıyı ekle
	ext := strings.ToLower(filepath.Ext(filename))
	selectedExt := strings.ToLower(strings.TrimSpace(r.FormValue("extension")))
	if ext == "" && selectedExt != "" {
		if !strings.HasPrefix(selectedExt, ".") {
			selectedExt = "." + selectedExt
		}
		filename = filename + selectedExt
		ext = selectedExt
	}

	// Dosya adını sanitize et
	filename = sanitizeFilename(filename)
	userDir := filepath.Join("uploads", user.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		http.Error(w, "Dizin oluşturulamadı", http.StatusInternalServerError)
		return
	}

	filename = getUniqueFilename(user.ID, user.Username, filename)
	targetPath := filepath.Join(userDir, filename)

	err := os.WriteFile(targetPath, []byte(content), 0644)
	if err != nil {
		http.Error(w, "Metin dosyası kaydedilemedi", http.StatusInternalServerError)
		return
	}

	mimeType := "text/plain"
	if ext == ".md" {
		mimeType = "text/markdown"
	} else if ext == ".json" {
		mimeType = "application/json"
	} else if ext == ".html" {
		mimeType = "text/html"
	}

	ttlStr := r.FormValue("ttl")
	expiresAt := parseTTLDuration(ttlStr)
	isBurn := r.FormValue("is_burn") == "1" || r.FormValue("is_burn") == "true"
	password := strings.TrimSpace(r.FormValue("password"))

	_, err = db.CreateFileWithOpts(user.ID, filename, filename, int64(len([]byte(content))), mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
		IsBurn:    isBurn,
		Password:  password,
	})
	if err != nil {
		http.Error(w, "Veritabanı hatası: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

type APIUploadResponse struct {
	Success   bool    `json:"success"`
	Filename  string  `json:"filename"`
	URL       string  `json:"url"`
	Size      int64   `json:"size"`
	MimeType  string  `json:"mime_type"`
	Views     int     `json:"views"`
	CreatedAt string  `json:"created_at"`
	Error     string  `json:"error,omitempty"`
}

func parseTTLDuration(ttlStr string) *time.Time {
	if ttlStr == "" || ttlStr == "forever" {
		return nil
	}
	var d time.Duration
	switch strings.ToLower(ttlStr) {
	case "10m":
		d = 10 * time.Minute
	case "1h":
		d = 1 * time.Hour
	case "1d":
		d = 24 * time.Hour
	case "1w":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		parsed, err := time.ParseDuration(ttlStr)
		if err == nil && parsed > 0 {
			d = parsed
		} else {
			return nil
		}
	}
	exp := time.Now().Add(d)
	return &exp
}

type JSONUploadRequest struct {
	Text     string `json:"text"`
	Filename string `json:"filename"`
	TTL      string `json:"ttl"`
	IsBurn   bool   `json:"is_burn"`
	Password string `json:"password"`
}

// APIUploadHandler harici API anahtarı ile yapılan yüklemeleri yönetir
func APIUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Yöntem izin verilmedi"})
		return
	}

	user, err := authenticateAPIRequest(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: err.Error()})
		return
	}

	userDir := filepath.Join("uploads", user.Username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Dizin oluşturulamadı"})
		return
	}

	contentType := r.Header.Get("Content-Type")

	var filename string
	var mimeType string
	var size int64
	var originalName string
	var fileOpts db.FileOptions

	if strings.HasPrefix(contentType, "application/json") {
		var req JSONUploadRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Geçersiz JSON yükü"})
			return
		}

		if req.Text == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Metin içeriği boş"})
			return
		}

		originalName = req.Filename
		if originalName == "" {
			originalName = "metin_" + fmt.Sprintf("%d", time.Now().Unix()) + ".txt"
		}

		filename = getUniqueFilename(user.ID, user.Username, originalName)
		targetPath := filepath.Join(userDir, filename)

		err = os.WriteFile(targetPath, []byte(req.Text), 0644)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Metin kaydedilemedi"})
			return
		}

		size = int64(len([]byte(req.Text)))
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".md" {
			mimeType = "text/markdown"
		} else if ext == ".json" {
			mimeType = "application/json"
		} else {
			mimeType = "text/plain"
		}

		fileOpts = db.FileOptions{
			ExpiresAt: parseTTLDuration(req.TTL),
			IsBurn:    req.IsBurn,
			Password:  req.Password,
		}

	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
		err := r.ParseMultipartForm(MaxUploadSize)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Dosya boyutu 100MB sınırını aşıyor"})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Yüklenecek 'file' parametresi bulunamadı"})
			return
		}
		defer file.Close()

		originalName = header.Filename
		filename = getUniqueFilename(user.ID, user.Username, originalName)
		targetPath := filepath.Join(userDir, filename)

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Dosya kaydedilemedi"})
			return
		}
		defer outFile.Close()

		size, err = io.Copy(outFile, file)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Dosya yazılamadı"})
			return
		}

		mimeType = header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		fileOpts = db.FileOptions{
			ExpiresAt: parseTTLDuration(r.FormValue("ttl")),
			IsBurn:    r.FormValue("is_burn") == "1" || r.FormValue("is_burn") == "true",
			Password:  strings.TrimSpace(r.FormValue("password")),
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "İstek gövdesi okunamadı"})
			return
		}

		if len(bodyBytes) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "İstek gövdesi boş"})
			return
		}

		originalName = "metin_" + fmt.Sprintf("%d", time.Now().Unix()) + ".txt"
		filename = getUniqueFilename(user.ID, user.Username, originalName)
		targetPath := filepath.Join(userDir, filename)

		err = os.WriteFile(targetPath, bodyBytes, 0644)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Dosya kaydedilemedi"})
			return
		}

		size = int64(len(bodyBytes))
		mimeType = "text/plain"

		fileOpts = db.FileOptions{
			ExpiresAt: parseTTLDuration(r.Header.Get("X-TTL")),
			IsBurn:    r.Header.Get("X-Burn") == "1" || r.Header.Get("X-Burn") == "true",
			Password:  strings.TrimSpace(r.Header.Get("X-Password")),
		}
	}

	dbFile, err := db.CreateFileWithOpts(user.ID, filename, originalName, size, mimeType, fileOpts)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Veritabanı hatası: " + err.Error()})
		return
	}

	publicURL := fmt.Sprintf("/%s/%s", user.Username, dbFile.Filename)

	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	absoluteURL := fmt.Sprintf("%s://%s%s", scheme, host, publicURL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIUploadResponse{
		Success:   true,
		Filename:  dbFile.Filename,
		URL:       absoluteURL,
		Size:      dbFile.FileSize,
		MimeType:  dbFile.MimeType,
		Views:     dbFile.Views,
		CreatedAt: dbFile.CreatedAt.Format(time.RFC3339),
	})
}
