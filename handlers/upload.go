package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bingo/db"
	"bingo/middleware"
)

// GetMaxUploadSizeMB sistemde yapılandırılmış MB cinsinden limiti döner (varsayılan: 50 MB)
func GetMaxUploadSizeMB() int {
	mb := db.GetSettingInt("max_upload_size_mb", 50)
	if mb <= 0 {
		return 50
	}
	return mb
}

// GetMaxUploadSize sistemde yapılandırılmış bayt cinsinden limiti döner
func GetMaxUploadSize() int64 {
	return int64(GetMaxUploadSizeMB()) * 1024 * 1024
}

var (
	// Güvenli dosya adı kontrolü
	safeFilenameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

	// maxOriginalNameLen bounds the display/original name stored in DB and
	// reflected in HTML + Content-Disposition headers.
	maxOriginalNameLen = 255
	// maxFilePasswordLen bounds bcrypt input for file passwords.
	maxFilePasswordLen = 128
	// maxInlineViewerBytes caps how much content is read into memory for the
	// HTML code viewer; larger files are forced to download.
	maxInlineViewerBytes = int64(5 * 1024 * 1024)
)

// sanitizeOriginalName cleans a user-supplied display name: strips path,
// control characters and CRLF (header-injection defense), truncates to 255.
func sanitizeOriginalName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	var sb strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		sb.WriteRune(r)
	}
	res := strings.TrimSpace(sb.String())
	if res == "" || res == "." || res == ".." {
		res = "dosya_" + fmt.Sprintf("%d", time.Now().Unix())
	}
	if len(res) > maxOriginalNameLen {
		ext := filepath.Ext(res)
		if len(ext) > 20 {
			ext = ext[:20]
		}
		base := res[:maxOriginalNameLen-len(ext)]
		res = base + ext
	}
	return res
}

// safeUserDirName reports whether a username is safe to embed in a filesystem
// path (defense for legacy DB rows created before strict validation).
func safeUserDirName(username string) bool {
	if username == "" || len(username) > 32 {
		return false
	}
	if username != filepath.Clean(username) {
		return false
	}
	if strings.Contains(username, "..") || strings.ContainsAny(username, `/\`) {
		return false
	}
	return true
}

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
	if len(originalName) > maxOriginalNameLen {
		originalName = originalName[:maxOriginalNameLen]
	}
	ext := filepath.Ext(originalName)
	base := originalName[:len(originalName)-len(ext)]
	base = sanitizeFilename(base)
	if base == "" {
		base = "dosya"
	}
	ext = sanitizeFilename(ext)
	if len(ext) > 20 {
		ext = ext[:20]
	}

	filename := base + ext
	counter := 1

	for {
		existing, err := db.GetFile(username, filename)
		if err != nil || existing == nil {
			break
		}
		filename = fmt.Sprintf("%s_%d%s", base, counter, ext)
		counter++
		if counter > 10000 {
			// Extremely unlikely; fall back to timestamp to avoid infinite loop.
			filename = fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
			break
		}
	}
	return filename
}

// API İsteğini Doğrulama Yardımcısı
func authenticateAPIRequest(r *http.Request) (*db.User, error) {
	apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if apiKey == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("API anahtarı eksik (missing API key)")
	}
	if len(apiKey) > 256 {
		return nil, fmt.Errorf("geçersiz API anahtarı (invalid API key)")
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
	if !safeUserDirName(user.Username) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success": false, "error": "Hesap adı geçersiz, yönetici ile iletişime geçin"}`))
		return
	}

	maxUploadSize := GetMaxUploadSize()
	maxMB := GetMaxUploadSizeMB()
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	err := r.ParseMultipartForm(maxUploadSize)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(`{"success": false, "error": "Dosya boyutu %d MB sınırını aşıyor"}`, maxMB)))
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

	chosenName := header.Filename
	if custom := strings.TrimSpace(r.FormValue("filename")); custom != "" {
		if filepath.Ext(custom) == "" && filepath.Ext(header.Filename) != "" {
			custom += filepath.Ext(header.Filename)
		}
		chosenName = custom
	}
	chosenName = sanitizeOriginalName(chosenName)

	filename := getUniqueFilename(user.ID, user.Username, chosenName)
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
	if mimeType == "" || mimeType == "application/octet-stream" {
		if ext := filepath.Ext(filename); ext != "" {
			if m := mime.TypeByExtension(ext); m != "" {
				mimeType = m
			}
		}
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	ttlStr := r.FormValue("ttl")
	expiresAt := parseTTLDuration(ttlStr)
	isBurn := r.FormValue("is_burn") == "1" || r.FormValue("is_burn") == "true"
	password := strings.TrimSpace(r.FormValue("password"))
	if len(password) > maxFilePasswordLen {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success": false, "error": "Şifre çok uzun (en fazla 128 karakter)"}`))
		return
	}

	dbFile, err := db.CreateFileWithOpts(user.ID, filename, chosenName, written, mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
		IsBurn:    isBurn,
		Password:  password,
	})
	if err != nil {
		log.Printf("WebUploadHandler DB error user=%d file=%s: %v", user.ID, filename, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success": false, "error": "Veritabanı hatası, lütfen tekrar deneyin"}`))
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
	if !safeUserDirName(user.Username) {
		http.Error(w, "Hesap adı geçersiz, yönetici ile iletişime geçin", http.StatusForbidden)
		return
	}

	maxUploadSize := GetMaxUploadSize()
	// Bound form memory/CPU for text pastes (DoS protection).
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1024*1024)

	filename := strings.TrimSpace(r.FormValue("filename"))
	content := r.FormValue("content")

	if filename == "" || content == "" {
		http.Error(w, "Dosya adı veya metin içeriği boş olamaz.", http.StatusBadRequest)
		return
	}
	if int64(len(content)) > maxUploadSize {
		http.Error(w, fmt.Sprintf("Metin içeriği %d MB sınırını aşıyor", GetMaxUploadSizeMB()), http.StatusRequestEntityTooLarge)
		return
	}
	if len(filename) > maxOriginalNameLen {
		http.Error(w, "Dosya adı çok uzun (en fazla 255 karakter).", http.StatusBadRequest)
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
	filename = sanitizeOriginalName(filename)
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
	if len(password) > maxFilePasswordLen {
		http.Error(w, "Şifre çok uzun (en fazla 128 karakter).", http.StatusBadRequest)
		return
	}

	_, err = db.CreateFileWithOpts(user.ID, filename, filename, int64(len([]byte(content))), mimeType, db.FileOptions{
		ExpiresAt: expiresAt,
		IsBurn:    isBurn,
		Password:  password,
	})
	if err != nil {
		log.Printf("CreateTextHandler DB error user=%d file=%s: %v", user.ID, filename, err)
		http.Error(w, "Veritabanı hatası, lütfen tekrar deneyin", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

type APIUploadResponse struct {
	Success   bool   `json:"success"`
	Filename  string `json:"filename"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
	Views     int    `json:"views"`
	CreatedAt string `json:"created_at"`
	Error     string `json:"error,omitempty"`
}

func parseTTLDuration(ttlStr string) *time.Time {
	if ttlStr == "" || ttlStr == "forever" {
		return nil
	}
	// Absolute upper bound: 30 days. Prevents abuse via huge custom durations
	// like "100000h" that would pin storage effectively forever.
	const maxTTL = 30 * 24 * time.Hour
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
	if d > maxTTL {
		d = maxTTL
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
	if !safeUserDirName(user.Username) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Hesap adı geçersiz"})
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
		maxUploadSize := GetMaxUploadSize()
		// Bound JSON body (DoS protection: previously unlimited).
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1024*1024)
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
		if int64(len(req.Text)) > maxUploadSize {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: fmt.Sprintf("Metin içeriği %d MB sınırını aşıyor", GetMaxUploadSizeMB())})
			return
		}
		if len(req.Password) > maxFilePasswordLen {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Şifre çok uzun (en fazla 128 karakter)"})
			return
		}

		originalName = sanitizeOriginalName(req.Filename)
		if strings.TrimSpace(req.Filename) == "" {
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
		maxUploadSize := GetMaxUploadSize()
		maxMB := GetMaxUploadSizeMB()
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		err := r.ParseMultipartForm(maxUploadSize)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: fmt.Sprintf("Dosya boyutu %d MB sınırını aşıyor", maxMB)})
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
		if custom := strings.TrimSpace(r.FormValue("filename")); custom != "" {
			if filepath.Ext(custom) == "" && filepath.Ext(header.Filename) != "" {
				custom += filepath.Ext(header.Filename)
			}
			originalName = custom
		}
		originalName = sanitizeOriginalName(originalName)

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
		if mimeType == "" || mimeType == "application/octet-stream" {
			if ext := filepath.Ext(filename); ext != "" {
				if m := mime.TypeByExtension(ext); m != "" {
					mimeType = m
				}
			}
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		fileOpts = db.FileOptions{
			ExpiresAt: parseTTLDuration(r.FormValue("ttl")),
			IsBurn:    r.FormValue("is_burn") == "1" || r.FormValue("is_burn") == "true",
			Password:  strings.TrimSpace(r.FormValue("password")),
		}
		if len(fileOpts.Password) > maxFilePasswordLen {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Şifre çok uzun (en fazla 128 karakter)"})
			return
		}
	} else {
		maxUploadSize := GetMaxUploadSize()
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
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
		if len(fileOpts.Password) > maxFilePasswordLen {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Şifre çok uzun (en fazla 128 karakter)"})
			return
		}
	}

	dbFile, err := db.CreateFileWithOpts(user.ID, filename, originalName, size, mimeType, fileOpts)
	if err != nil {
		log.Printf("APIUploadHandler DB error user=%d file=%s: %v", user.ID, filename, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIUploadResponse{Success: false, Error: "Veritabanı hatası, lütfen tekrar deneyin"})
		return
	}

	publicURL := fmt.Sprintf("/%s/%s", user.Username, dbFile.Filename)

	// Host header is validated (injection-safe) inside BaseURL.
	absoluteURL := BaseURL(r) + publicURL

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
