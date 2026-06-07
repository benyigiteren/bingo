package handlers

import (
	"gotree/db"
	"gotree/middleware"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AdminViewModel admin paneli görünüm verisidir.
type AdminViewModel struct {
	User            *db.User
	Page            *db.Page
	Blocks          []db.Block
	AllUsers        []db.User // Sadece Super Admin için doldurulur
	FormSubmissions []db.FormSubmission
	IsPrimary       bool
	Error           string
	Success         string
	APIKey          string
}

// AdminGet ana yönetim paneli arayüzünü yükler.
func AdminGet(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	vm := AdminViewModel{
		User: user,
	}

	if user.APIKey.Valid {
		vm.APIKey = user.APIKey.String
	}

	// Kullanıcı mesajlarını al
	if err := r.ParseForm(); err == nil {
		if r.FormValue("error") != "" {
			vm.Error = r.FormValue("error")
		}
		if r.FormValue("success") != "" {
			vm.Success = r.FormValue("success")
		}
	}

	// 1. Kullanıcının sayfasını çek
	page, err := db.GetPageByUserID(user.ID)
	if err != nil {
		vm.Error = "Sayfa yüklenirken veritabanı hatası oluştu."
	}
	vm.Page = page

	if page != nil {
		// 2. Sayfaya ait blokları çek
		blocks, err := db.GetBlocks(page.ID)
		if err != nil {
			vm.Error = "Bloklar yüklenirken hata oluştu."
		}
		vm.Blocks = blocks

		// 3. Bu sayfa birincil sayfa mı kontrol et
		primarySlug, _ := db.GetSetting("primary_slug")
		vm.IsPrimary = (primarySlug == page.Slug)

		// 3.5. Form mesajlarını çek
		subs, err := db.GetFormSubmissions(page.ID)
		if err == nil {
			vm.FormSubmissions = subs
		} else {
			vm.FormSubmissions = []db.FormSubmission{}
		}
	}

	// 4. Eğer Super Admin ise diğer kullanıcıları da listele
	if user.Role == "superadmin" {
		users, err := db.GetUsers()
		if err == nil {
			vm.AllUsers = users
		}
	}

	RenderTemplate(w, "admin.html", vm)
}

// PageCreatePost ilk kez sayfa oluşturma isteğini işler.
func PageCreatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	slug := r.FormValue("slug")
	if slug == "" {
		http.Redirect(w, r, "/admin?error=Slug+alanı+boş+bırakılamaz", http.StatusSeeOther)
		return
	}

	// Slug'ın eşsiz olduğunu doğrula
	existing, _ := db.GetPageBySlug(slug)
	if existing != nil {
		http.Redirect(w, r, "/admin?error=Bu+slug+zaten+kullanılıyor", http.StatusSeeOther)
		return
	}

	_, err := db.CreatePage(user.ID, slug, user.Username, "Gotree sayfama hoş geldiniz!", "", "minimal_cream")
	if err != nil {
		http.Redirect(w, r, "/admin?error=Sayfa+oluşturulurken+hata", http.StatusSeeOther)
		return
	}

	// Eğer sistemdeki tek sayfa ise otomatik olarak birincil (primary) yap
	prim, _ := db.GetSetting("primary_slug")
	if prim == "" {
		_ = db.SetSetting("primary_slug", slug)
	}

	http.Redirect(w, r, "/admin?success=Sayfa+başarıyla+oluşturuldu", http.StatusSeeOther)
}

// PageUpdatePost sayfa genel bilgilerini günceller.
func PageUpdatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		http.Redirect(w, r, "/admin?error=Sayfa+bulunamadı", http.StatusSeeOther)
		return
	}

	slug := r.FormValue("slug")
	title := r.FormValue("title")
	bio := r.FormValue("bio")
	theme := r.FormValue("theme")
	isPrimary := r.FormValue("is_primary") == "1"

	if slug == "" || title == "" {
		http.Redirect(w, r, "/admin?error=Slug+ve+Başlık+boş+bırakılamaz", http.StatusSeeOther)
		return
	}

	// Başka birinin slug'ını işgal etmediğini kontrol et
	existing, _ := db.GetPageBySlug(slug)
	if existing != nil && existing.ID != page.ID {
		http.Redirect(w, r, "/admin?error=Bu+slug+başka+bir+sayfa+tarafından+kullanılıyor", http.StatusSeeOther)
		return
	}

	// Avatar yükleme işlemini kontrol et
	avatarURL := page.AvatarURL
	file, header, err := r.FormFile("avatar")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		filename := fmt.Sprintf("avatar_%d_%d%s", page.ID, time.Now().Unix(), ext)
		
		uploadPath := "./uploads"
		_ = os.MkdirAll(uploadPath, 0755)

		out, err := os.Create(filepath.Join(uploadPath, filename))
		if err == nil {
			defer out.Close()
			_, _ = io.Copy(out, file)
			avatarURL = "/uploads/" + filename
		}
	}

	// Arkaplan görsel yükleme işlemini kontrol et
	backgroundImageURL := page.BackgroundImageURL
	if r.FormValue("clear_background") == "1" {
		backgroundImageURL = ""
	} else {
		bgFile, bgHeader, err := r.FormFile("background_image")
		if err == nil {
			defer bgFile.Close()
			ext := filepath.Ext(bgHeader.Filename)
			filename := fmt.Sprintf("bg_%d_%d%s", page.ID, time.Now().Unix(), ext)

			uploadPath := "./uploads"
			_ = os.MkdirAll(uploadPath, 0755)

			out, err := os.Create(filepath.Join(uploadPath, filename))
			if err == nil {
				defer out.Close()
				_, _ = io.Copy(out, bgFile)
				backgroundImageURL = "/uploads/" + filename
			}
		}
	}

	// Sayfayı güncelle
	err = db.UpdatePage(page.ID, slug, title, bio, avatarURL, theme, backgroundImageURL)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Güncelleme+başarısız", http.StatusSeeOther)
		return
	}

	// Birincil sayfa ayarı (YALNIZCA Super Admin yapabilir!)
	if user.Role == "superadmin" {
		currentPrimary, _ := db.GetSetting("primary_slug")
		if isPrimary {
			_ = db.SetSetting("primary_slug", slug)
		} else if currentPrimary == page.Slug {
			_ = db.SetSetting("primary_slug", "")
		}
	}

	http.Redirect(w, r, "/admin?success=Sayfa+bilgileri+güncellendi", http.StatusSeeOther)
}

// PageAutosave sayfa genel bilgilerini otomatik kaydeder (Alpine.js AJAX).
func PageAutosave(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Oturum açmalısınız"}`))
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Sayfa bulunamadı"}`))
		return
	}

	var req struct {
		Title string `json:"title"`
		Bio   string `json:"bio"`
		Theme string `json:"theme"`
		Slug  string `json:"slug"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Geçersiz istek gövdesi"}`))
		return
	}

	// Slug güncelleme kontrolü
	if req.Slug != "" && req.Slug != page.Slug {
		existing, _ := db.GetPageBySlug(req.Slug)
		if existing != nil {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"Bu slug zaten kullanılıyor"}`))
			return
		}
		page.Slug = req.Slug
	}

	if req.Title != "" {
		page.Title = req.Title
	}
	page.Bio = req.Bio
	if req.Theme != "" {
		page.Theme = req.Theme
	}

	err = db.UpdatePage(page.ID, page.Slug, page.Title, page.Bio, page.AvatarURL, page.Theme, page.BackgroundImageURL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Veritabanı güncelleme hatası"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// BlockCreatePost yeni bir blok ekler.
func BlockCreatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		http.Redirect(w, r, "/admin?error=Önce+sayfa+oluşturmalısınız", http.StatusSeeOther)
		return
	}

	blockType := r.FormValue("type")
	if blockType != "link" && blockType != "text" && blockType != "social_links" && blockType != "form" {
		http.Redirect(w, r, "/admin?error=Geçersiz+blok+tipi", http.StatusSeeOther)
		return
	}

	// Sosyal Medya İkon Sınırı: Sayfada en fazla 1 sosyal blok bulunabilir!
	if blockType == "social_links" {
		var count int
		_ = db.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE page_id = ? AND type = 'social_links'", page.ID).Scan(&count)
		if count > 0 {
			http.Redirect(w, r, "/admin?error=En+fazla+1+adet+sosyal+medya+bloğu+ekleyebilirsiniz", http.StatusSeeOther)
			return
		}
	}

	title := "Yeni Blok"
	content := ""

	if blockType == "link" {
		title = "Yeni Link"
		content = "https://"
	} else if blockType == "text" {
		title = "Başlık Yazısı"
		content = "İçerik açıklaması buraya gelecek."
	} else if blockType == "social_links" {
		title = "Sosyal Medya"
		content = `{"instagram":"","twitter":"","github":"","youtube":"","linkedin":"","email":"","discord":"","mastodon":""}`
	} else if blockType == "form" {
		title = "İletişim Formu"
		content = `{"discord_webhook_url":"","telegram_bot_token":"","telegram_chat_id":"","smtp_host":"","smtp_port":"","smtp_user":"","smtp_pass":"","smtp_to":""}`
	}

	_, err = db.CreateBlock(page.ID, blockType, title, content)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Blok+eklenirken+hata", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Blok+başarıyla+eklendi", http.StatusSeeOther)
}

// BlockUpdatePost bloğu günceller.
func BlockUpdatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	
	var pageUserID int64
	err := db.DB.QueryRow(
		"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
		id,
	).Scan(&pageUserID)

	if err != nil || pageUserID != user.ID {
		http.Redirect(w, r, "/admin?error=Yetkisiz+erişim", http.StatusSeeOther)
		return
	}

	title := r.FormValue("title")
	content := r.FormValue("content")
	style := r.FormValue("style")
	icon := r.FormValue("icon")
	iconSize := r.FormValue("icon_size")

	if style == "" {
		style = "standard"
	}
	if iconSize == "" {
		iconSize = "small"
	}

	// Eğer sosyal medya ise form verilerinden JSON oluştur
	if r.FormValue("is_social") == "1" {
		socials := map[string]string{
			"instagram": r.FormValue("social_instagram"),
			"twitter":   r.FormValue("social_twitter"),
			"github":    r.FormValue("social_github"),
			"youtube":   r.FormValue("social_youtube"),
			"linkedin":  r.FormValue("social_linkedin"),
			"email":     r.FormValue("social_email"),
			"discord":   r.FormValue("social_discord"),
			"mastodon":  r.FormValue("social_mastodon"),
		}
		jsonBytes, err := json.Marshal(socials)
		if err == nil {
			content = string(jsonBytes)
		}
	}

	// Eğer form bloğu ise form verilerinden JSON oluştur
	if r.FormValue("is_form") == "1" {
		formSettings := map[string]string{
			"discord_webhook_url": r.FormValue("form_discord_webhook_url"),
			"telegram_bot_token":  r.FormValue("form_telegram_bot_token"),
			"telegram_chat_id":    r.FormValue("form_telegram_chat_id"),
			"smtp_host":           r.FormValue("form_smtp_host"),
			"smtp_port":           r.FormValue("form_smtp_port"),
			"smtp_user":           r.FormValue("form_smtp_user"),
			"smtp_pass":           r.FormValue("form_smtp_pass"),
			"smtp_to":             r.FormValue("form_smtp_to"),
		}
		jsonBytes, err := json.Marshal(formSettings)
		if err == nil {
			content = string(jsonBytes)
		}
	}

	err = db.UpdateBlock(id, title, content, style, icon, iconSize)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Blok+güncellenemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Blok+güncellendi", http.StatusSeeOther)
}

// BlockAutosave bir bloğu otomatik kaydeder (Alpine.js AJAX).
func BlockAutosave(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Oturum açmalısınız"}`))
		return
	}

	var req struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Style    string `json:"style"`
		Icon     string `json:"icon"`
		IconSize string `json:"icon_size"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Geçersiz istek gövdesi"}`))
		return
	}

	// Yetki kontrolü
	var pageUserID int64
	err := db.DB.QueryRow(
		"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
		req.ID,
	).Scan(&pageUserID)

	if err != nil || pageUserID != user.ID {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"Bu blok üzerinde yetkiniz yok"}`))
		return
	}

	if req.Style == "" {
		req.Style = "standard"
	}
	if req.IconSize == "" {
		req.IconSize = "small"
	}

	err = db.UpdateBlock(req.ID, req.Title, req.Content, req.Style, req.Icon, req.IconSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Veritabanı güncelleme hatası"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// BlockDeletePost bloğu siler.
func BlockDeletePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Error(w, "Yetkisiz işlem", http.StatusUnauthorized)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	var pageUserID int64
	err := db.DB.QueryRow(
		"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
		id,
	).Scan(&pageUserID)

	if err != nil || pageUserID != user.ID {
		http.Redirect(w, r, "/admin?error=Yetkisiz+erişim", http.StatusSeeOther)
		return
	}

	err = db.DeleteBlock(id)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Blok+silinemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Blok+silindi", http.StatusSeeOther)
}

// BlockReorderPost sıralamayı günceller.
func BlockReorderPost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Yetkisiz işlem"}`))
		return
	}

	var req struct {
		IDs []int64 `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Geçersiz veri"}`))
		return
	}

	orders := make(map[int64]int)
	for index, id := range req.IDs {
		var pageUserID int64
		err := db.DB.QueryRow(
			"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
			id,
		).Scan(&pageUserID)

		if err != nil || pageUserID != user.ID {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"Sıralama yetkisi yok"}`))
			return
		}

		orders[id] = index + 1
	}

	if err := db.UpdateBlockOrders(orders); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Sıralama kaydedilemedi"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// APIKeyRegeneratePost yeni bir API anahtarı üretir.
func APIKeyRegeneratePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	_, err := db.GenerateAPIKey(user.ID)
	if err != nil {
		http.Redirect(w, r, "/admin?error=API+Anahtarı+yenilenemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=API+Anahtarı+başarıyla+yenilendi", http.StatusSeeOther)
}

// UserCreatePost Super Admin'in yeni kullanıcı eklemesini sağlar.
func UserCreatePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil || user.Role != "superadmin" {
		http.Redirect(w, r, "/admin?error=Yetkisiz+işlem", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")

	if username == "" || password == "" {
		http.Redirect(w, r, "/admin?error=Kullanıcı+adı+ve+şifre+boş+bırakılamaz", http.StatusSeeOther)
		return
	}

	if role != "admin" && role != "user" {
		role = "user"
	}

	existing, _ := db.GetUserByUsername(username)
	if existing != nil {
		http.Redirect(w, r, "/admin?error=Bu+kullanıcı+adı+zaten+alınmış", http.StatusSeeOther)
		return
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Şifre+oluşturma+hatası", http.StatusSeeOther)
		return
	}

	_, err = db.CreateUser(username, string(hashedBytes), role)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Kullanıcı+eklenemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Kullanıcı+başarıyla+eklendi", http.StatusSeeOther)
}

// UserDeletePost Super Admin'in bir kullanıcıyı silmesini sağlar.
func UserDeletePost(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil || user.Role != "superadmin" {
		http.Redirect(w, r, "/admin?error=Yetkisiz+işlem", http.StatusSeeOther)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if id == user.ID {
		http.Redirect(w, r, "/admin?error=Kendi+hesabınızı+silemezsiniz", http.StatusSeeOther)
		return
	}

	err := db.DeleteUser(id)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Kullanıcı+silinemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Kullanıcı+silindi", http.StatusSeeOther)
}

// BlockUploadImage bloğa ait özel ikon görselini yükler.
func BlockUploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Oturum açmalısınız"})
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Geçersiz blok ID"})
		return
	}

	// Yetki kontrolü
	var pageUserID int64
	err = db.DB.QueryRow(
		"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
		id,
	).Scan(&pageUserID)

	if err != nil || pageUserID != user.ID {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bu blok üzerinde yetkiniz yok"})
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Dosya yüklenemedi"})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("block_%d_%d%s", id, time.Now().Unix(), ext)
	uploadPath := "./uploads"
	_ = os.MkdirAll(uploadPath, 0755)

	out, err := os.Create(filepath.Join(uploadPath, filename))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Dosya oluşturulamadı"})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Dosya yazma hatası"})
		return
	}

	imageURL := "/uploads/" + filename

	// Veritabanını güncelle
	_, err = db.DB.Exec("UPDATE blocks SET icon = ? WHERE id = ?", imageURL, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Veritabanı güncellenemedi"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"path":    imageURL,
	})
}

// PageUploadAvatar uploads and updates page avatar (AJAX).
func PageUploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Oturum açmalısınız"})
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sayfa bulunamadı"})
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Dosya alınamadı"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Yalnızca görsel dosyaları yüklenebilir"})
		return
	}

	filename := fmt.Sprintf("avatar_%d_%d%s", page.ID, time.Now().Unix(), ext)
	uploadPath := "./uploads"
	_ = os.MkdirAll(uploadPath, 0755)

	out, err := os.Create(filepath.Join(uploadPath, filename))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Dosya kaydedilemedi"})
		return
	}
	defer out.Close()

	_, _ = io.Copy(out, file)
	avatarURL := "/uploads/" + filename

	// Update DB
	err = db.UpdatePage(page.ID, page.Slug, page.Title, page.Bio, avatarURL, page.Theme, page.BackgroundImageURL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Veritabanı güncellenemedi"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"url":     avatarURL,
	})
}

// PageUploadBg uploads and updates page background image (AJAX).
func PageUploadBg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Oturum açmalısınız"})
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sayfa bulunamadı"})
		return
	}

	clearBg := r.FormValue("clear_background") == "1"
	backgroundImageURL := page.BackgroundImageURL

	if clearBg {
		backgroundImageURL = ""
	} else {
		file, header, err := r.FormFile("background_image")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Dosya alınamadı"})
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Yalnızca görsel dosyaları yüklenebilir"})
			return
		}

		filename := fmt.Sprintf("bg_%d_%d%s", page.ID, time.Now().Unix(), ext)
		uploadPath := "./uploads"
		_ = os.MkdirAll(uploadPath, 0755)

		out, err := os.Create(filepath.Join(uploadPath, filename))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Dosya kaydedilemedi"})
			return
		}
		defer out.Close()

		_, _ = io.Copy(out, file)
		backgroundImageURL = "/uploads/" + filename
	}

	err = db.UpdatePage(page.ID, page.Slug, page.Title, page.Bio, page.AvatarURL, page.Theme, backgroundImageURL)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Veritabanı güncellenemedi"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"url":     backgroundImageURL,
	})
}

// FormSubmissionDelete deletes a form submission from admin panel.
func FormSubmissionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		http.Redirect(w, r, "/admin?error=Sayfa+bulunamadı", http.StatusSeeOther)
		return
	}

	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	// Verify ownership
	var submissionPageID int64
	err = db.DB.QueryRow("SELECT page_id FROM form_submissions WHERE id = ?", id).Scan(&submissionPageID)
	if err != nil || submissionPageID != page.ID {
		http.Redirect(w, r, "/admin?error=Yetkisiz+işlem", http.StatusSeeOther)
		return
	}

	err = db.DeleteFormSubmission(id)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Mesaj+silinemedi", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Mesaj+başarıyla+silindi", http.StatusSeeOther)
}

