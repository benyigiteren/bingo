package handlers

import (
	"gotree/db"
	"gotree/middleware"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"
)

var startTime = time.Now()

// APIDocsGet API yardım ve dökümantasyon sayfasını gösterir.
func APIDocsGet(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	var apiKey string
	if user != nil && user.APIKey.Valid {
		apiKey = user.APIKey.String
	}

	data := map[string]interface{}{
		"APIKey": apiKey,
		"Host":   r.Host,
	}

	RenderTemplate(w, "api_docs.html", data)
}

// APIHelp API hakkında detaylı yardım bilgilerini JSON döner.
func APIHelp(w http.ResponseWriter, r *http.Request) {
	helpData := map[string]interface{}{
		"name":        "Gotree API",
		"version":     "v1.0.0",
		"description": "Gotree platformu icin RESTful yonetim ve veri erisim servisi. Sayfa, blok, istatistik ve form yonetimi saglar.",
		"base_url":    fmt.Sprintf("https://%s", r.Host),
		"auth": map[string]interface{}{
			"method":      "API Key veya Oturum Cerezi",
			"header":      "X-API-Key: <api_key>",
			"query":       "?api_key=<api_key>",
			"description": "API istekleri icin X-API-Key header'i veya ?api_key query parametresi gereklidir. API anahtarinizi yonetim panelinden (Admin > API Ayarlari) gorebilir ve yeniden olusturabilirsiniz. Oturum cerezi (session cookie) ile de dogrulama yapilabilir.",
		},
		"rate_limiting": map[string]interface{}{
			"enabled":     true,
			"description": "Asiri kullanimi onlemek icin hiz sinirlamasi uygulanir. Siniri asildiginda HTTP 429 doner.",
		},
		"endpoints": []map[string]interface{}{
			{
				"method":      "GET",
				"path":        "/api/v1/page",
				"description": "Kullanicinin sayfa bilgilerini (baslik, bio, tema, avatar, slug) ve tum bloklarini getirir.",
				"auth":        true,
				"response": map[string]interface{}{
					"page":   "Sayfa nesnesi (id, slug, title, bio, theme, avatar_url, background_image_url)",
					"blocks": "Blok dizisi",
				},
			},
			{
				"method":      "POST",
				"path":        "/api/v1/page",
				"description": "Sayfa ayarlarini gunceller. Sadece gonderilen alanlar guncellenir.",
				"auth":        true,
				"params": map[string]string{
					"title": "Sayfa basligi (string, opsiyonel)",
					"bio":   "Biyografi metni (string, opsiyonel)",
					"theme": "Tema adi (string, opsiyonel). Gecerli temalar: minimal_cream, brutalist_mint, sunset_gradient, olive_garden, skater_peach, forest_outline, sunset_glow, cosmic_space, sticky_wicks, galaxy_night, metallic_luxury, business_dark, agency_minimal, community_neon, corp_professional",
					"slug":  "URL slug (string, opsiyonel). Benzersiz olmalidir.",
				},
				"response": map[string]interface{}{
					"success": true,
					"page":    "Guncellenmis sayfa nesnesi",
				},
			},
			{
				"method":      "PUT",
				"path":        "/api/v1/page",
				"description": "POST /api/v1/page ile ayni islevi gorur (sayfa guncelleme).",
				"auth":        true,
			},
			{
				"method":      "GET",
				"path":        "/api/v1/blocks",
				"description": "Kullanicinin sayfasina ait tum bloklari listeler (link, text, social_links, form).",
				"auth":        true,
				"response":    "Blok dizisi (id, type, title, content, style, icon, icon_size, clicks, position)",
			},
			{
				"method":      "POST",
				"path":        "/api/v1/blocks",
				"description": "Sayfaya yeni bir blok ekler. Social_links blogu en fazla 1 adet olabilir.",
				"auth":        true,
				"params": map[string]string{
					"type":    "Blok tipi (zorunlu). Gecerli degerler: link, text, social_links, form",
					"title":   "Blok basligi (string, opsiyonel)",
					"content": "Blok icerigi (string, opsiyonel). Link tipi icin URL, text tipi icin metin, social_links icin JSON, form icin JSON ayarlari.",
				},
				"response": map[string]interface{}{
					"success":  true,
					"block_id": "Olusturulan blogun ID degeri (int64)",
				},
			},
			{
				"method":      "POST",
				"path":        "/api/v1/blocks/update",
				"description": "Mevcut bir blogu gunceller. Blogun sahibi olmali.",
				"auth":        true,
				"params": map[string]string{
					"id":        "Guncellenecek blogun ID degeri (int64, zorunlu)",
					"title":     "Yeni baslik (string, opsiyonel)",
					"content":   "Yeni icerik (string, opsiyonel)",
					"style":     "Yeni stil (string, opsiyonel). Gecerli stiller: standard, brutal, metallic, rounded, outline, glass, neon, shadow, cyber, border_glow, soft_clay, retro_floppy, gradient_glow, aurora_glass, minimal_flat, glass_border",
					"icon":      "Ikon adi veya resim yolu (string, opsiyonel). Lucide ikon adi veya /uploads/ ile baslayan resim yolu.",
					"icon_size": "Ikon boyutu (string, opsiyonel)",
				},
				"response": map[string]interface{}{
					"success": true,
				},
			},
			{
				"method":      "PUT",
				"path":        "/api/v1/blocks/update",
				"description": "POST /api/v1/blocks/update ile ayni islevi gorur (blok guncelleme).",
				"auth":        true,
			},
			{
				"method":      "POST",
				"path":        "/api/v1/blocks/reorder",
				"description": "Bloklarin siralama sirasini gunceller. Gonderilen dizi sirasina gore position degerleri atanir.",
				"auth":        true,
				"params": map[string]string{
					"order": "Blok ID dizisi (int64[], zorunlu). Ornek: [3, 1, 5, 2]",
				},
				"response": map[string]interface{}{
					"success": true,
					"message": "Blok siralamasi guncellendi",
				},
			},
			{
				"method":      "DELETE",
				"path":        "/api/v1/blocks",
				"description": "Bir blogu siler. Blogun sahibi olmali. ?id= query parametresi veya JSON body ile id gonderilebilir.",
				"auth":        true,
				"params": map[string]string{
					"id": "Silinecek blogun ID degeri (int64, zorunlu). Query parametresi veya JSON body.",
				},
				"response": map[string]interface{}{
					"success": true,
				},
			},
			{
				"method":      "GET",
				"path":        "/api/v1/stats",
				"description": "Sayfa ve blok istatistiklerini getirir. Link tiklanma, sayfa goruntuleme, gunluk grafik, cihaz ve ulke kirilimi, sosyal medya tiklamalari icerir.",
				"auth":        true,
				"params": map[string]string{
					"range": "Zaman araligi (string, opsiyonel). Gecerli degerler: 7d (varsayilan), 30d, all",
				},
				"response": map[string]interface{}{
					"range":         "Secilen zaman araligi",
					"link_clicks":   "Link bloklarinin tiklanma sayilari",
					"total_views":   "Toplam sayfa goruntuleme",
					"total_clicks":  "Toplam link ve sosyal medya tiklamasi",
					"daily_views":   "Gunluk goruntuleme grafigi",
					"devices":       "Cihaz kirilimi (desktop, mobile, tablet)",
					"countries":     "Ulke kirilimi (ilk 10)",
					"social_clicks": "Sosyal medya platform tiklamalari",
				},
			},
			{
				"method":      "GET",
				"path":        "/api/v1/form-submissions",
				"description": "Iletisim formu basvurularini listeler.",
				"auth":        true,
				"response": map[string]interface{}{
					"total":       "Toplam basvuru sayisi",
					"submissions": "Basvuru dizisi (id, block_id, name, email, message, created_at)",
				},
			},
			{
				"method":      "DELETE",
				"path":        "/api/v1/form-submissions",
				"description": "Bir form basvurusunu siler.",
				"auth":        true,
				"params": map[string]string{
					"id": "Silinecek basvurunun ID degeri (int64, zorunlu). Query parametresi.",
				},
				"response": map[string]interface{}{
					"success": true,
					"message": "Mesaj basariyla silindi",
				},
			},
			{
				"method":      "GET",
				"path":        "/api/v1/system/stats",
				"description": "Sistem sagligi ve kaynak kullanimi bilgilerini dondurur (uptime, RAM, CPU, DB boyutu, toplam kayit sayilari).",
				"auth":        true,
				"response": map[string]interface{}{
					"system":   "Sistem bilgileri (uptime, go_version, num_cpu, goroutines, ram_used_mb, db_size_kb)",
					"database": "Veritabani istatistikleri (total_pages, total_blocks, total_clicks)",
					"status":   "Durum (healthy)",
				},
			},
			{
				"method":      "GET",
				"path":        "/api/v1/profile/{slug}",
				"description": "Herkese acik profil bilgilerini dondurur. Kimlik dogrulama GEREKMEZ. CORS desteklidir.",
				"auth":        false,
				"params": map[string]string{
					"slug": "Profil slug'i (path parametresi, zorunlu)",
				},
				"response": map[string]interface{}{
					"profile": "Profil nesnesi (slug, title, bio, theme, avatar_url, background_image_url)",
					"blocks":  "Blok dizisi",
				},
			},
			{
				"method":      "POST",
				"path":        "/form/submit/{id}",
				"description": "Ziyaretcilerin iletisim formu gondermesi icin kullanilir. Kimlik dogrulama GEREKMEZ. Honeypot spam korumasi aktiftir.",
				"auth":        false,
				"params": map[string]string{
					"name":    "Gonderen adi (form field, zorunlu)",
					"email":   "Gonderen e-posta (form field, zorunlu)",
					"message": "Mesaj icerigi (form field, zorunlu)",
				},
				"response": map[string]interface{}{
					"success": true,
					"message": "Mesajiniz basariyla iletildi!",
				},
			},
			{
				"method":      "GET",
				"path":        "/api/v1/help",
				"description": "Bu yardim dokumanini goruntuler.",
				"auth":        false,
			},
			{
				"method":      "GET",
				"path":        "/api/docs",
				"description": "Etkilesimli API dokumantasyon sayfasini gosterir (HTML).",
				"auth":        false,
			},
		},
		"block_types": []map[string]string{
			{"type": "link", "description": "Harici URL'ye yonlendirme linki. Ikon, stil ve tiklanma sayaci destekler."},
			{"type": "text", "description": "Baslik ve icerik metni blogu."},
			{"type": "social_links", "description": "Sosyal medya ikonlari blogu. En fazla 1 adet eklenebilir. Desteklenen platformlar: instagram, twitter, github, youtube, linkedin, email, discord, mastodon, tiktok, telegram, whatsapp, pinterest, reddit, twitch, snapchat"},
			{"type": "form", "description": "Iletisim formu blogu. Display modlari: direct (satir ici form), button (akordiyon), floating (kayan buton + modal). Discord, Telegram ve SMTP bildirim destekler."},
		},
		"themes": []string{
			"minimal_cream", "brutalist_mint", "sunset_gradient", "olive_garden",
			"skater_peach", "forest_outline", "sunset_glow", "cosmic_space",
			"sticky_wicks", "galaxy_night", "metallic_luxury", "business_dark",
			"agency_minimal", "community_neon", "corp_professional",
		},
		"block_styles": []string{
			"standard", "brutalist", "metallic", "rounded", "outline", "glass",
			"neon", "shadow", "cyber", "border_glow", "soft_clay", "retro_floppy",
			"gradient_glow", "aurora_glass", "minimal_flat", "glass_border",
		},
		"error_codes": map[string]string{
			"400": "Gecersiz istek (eksik veya hatali parametre)",
			"401": "Yetkisiz erisim (API key veya oturum bulunamadi)",
			"403": "Yasak (baska kullanicinin kaynagina erisim girisimi)",
			"404": "Bulunamadi (sayfa, blok veya profil mevcut degil)",
			"409": "Cakisma (benzersiz alan ihlali veya limit asimi)",
			"429": "Cok fazla istek (hiz siniri asildi)",
			"500": "Sunucu hatasi (veritabani veya dahili hata)",
		},
		"status":      "active",
		"server_time": time.Now().Format(time.RFC3339),
		"uptime":      time.Since(startTime).String(),
		"runtime":     runtime.Version(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(helpData)
}

// APIGetPage kullanıcının sayfa ve blok bilgilerini JSON döner.
func APIGetPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Yetkisiz işlem"})
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Veritabanı hatası"})
		return
	}

	if page == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sayfa bulunamadı"})
		return
	}

	blocks, err := db.GetBlocks(page.ID)
	if err != nil {
		blocks = []db.Block{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"page":   page,
		"blocks": blocks,
	})
}

// APIGetPublicProfile herkese açık bir sayfanın profil bilgilerini döner (kimlik doğrulama gerekmez).
func APIGetPublicProfile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		slug = r.URL.Query().Get("slug")
	}
	if slug == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "slug parametresi gerekli"})
		return
	}

	page, err := db.GetPageBySlug(slug)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Profil bulunamadı"})
		return
	}

	blocks, err := db.GetBlocks(page.ID)
	if err != nil {
		blocks = []db.Block{}
	}

	type PublicPage struct {
		Slug               string `json:"slug"`
		Title              string `json:"title"`
		Bio                string `json:"bio"`
		Theme              string `json:"theme"`
		AvatarURL          string `json:"avatar_url"`
		BackgroundImageURL string `json:"background_image_url"`
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"profile": PublicPage{
			Slug:               page.Slug,
			Title:              page.Title,
			Bio:                page.Bio,
			Theme:              page.Theme,
			AvatarURL:          page.AvatarURL,
			BackgroundImageURL: page.BackgroundImageURL,
		},
		"blocks": blocks,
	})
}

// APIUpdatePage sayfa bilgilerini JSON isteğiyle günceller.
func APIUpdatePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
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
		return
	}

	if req.Slug != "" && req.Slug != page.Slug {
		existing, _ := db.GetPageBySlug(req.Slug)
		if existing != nil {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bu slug zaten kullanılıyor"})
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
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"page":    page,
	})
}

// APIGetBlocks blok listesini döner.
func APIGetBlocks(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	blocks, err := db.GetBlocks(page.ID)
	if err != nil {
		blocks = []db.Block{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(blocks)
}

// APICreateBlock yeni içerik bloğu ekler.
func APICreateBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var req struct {
		Type    string `json:"type"` // 'link', 'text', 'social_links', 'form'
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Type != "link" && req.Type != "text" && req.Type != "social_links" && req.Type != "form" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Geçersiz blok tipi. Geçerli tipler: link, text, social_links, form"})
		return
	}

	// Sosyal ikon blok sınırı kontrolü
	if req.Type == "social_links" {
		var count int
		_ = db.DB.QueryRow("SELECT COUNT(*) FROM blocks WHERE page_id = ? AND type = 'social_links'", page.ID).Scan(&count)
		if count > 0 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "En fazla 1 adet sosyal medya bloğu ekleyebilirsiniz"})
			return
		}
	}

	id, err := db.CreateBlock(page.ID, req.Type, req.Title, req.Content)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"block_id": id,
	})
}

// APIUpdateBlock mevcut bloğu günceller.
func APIUpdateBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// URL parametresinden veya istek gövdesinden id oku
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
		return
	}

	err = db.UpdateBlock(req.ID, req.Title, req.Content, req.Style, req.Icon, req.IconSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// APIDeleteBlock bloğu siler.
func APIDeleteBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// URL sorgu parametresinden id al
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		// İstek gövdesinden de okunabilir
		var req struct {
			ID int64 `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		idStr = strconv.FormatInt(req.ID, 10)
	}

	id, _ := strconv.ParseInt(idStr, 10, 64)

	var pageUserID int64
	err := db.DB.QueryRow(
		"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
		id,
	).Scan(&pageUserID)

	if err != nil || pageUserID != user.ID {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = db.DeleteBlock(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// APIReorderBlocks blokların sırasını günceller.
func APIReorderBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Order []int64 `json:"order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Geçersiz istek. 'order' alanı int64 dizisi olmalı"})
		return
	}

	if len(req.Order) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'order' dizisi boş olamaz"})
		return
	}

	for i, blockID := range req.Order {
		var pageUserID int64
		err := db.DB.QueryRow(
			"SELECT p.user_id FROM blocks b JOIN pages p ON b.page_id = p.id WHERE b.id = ?",
			blockID,
		).Scan(&pageUserID)
		if err != nil || pageUserID != user.ID {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Yetkisiz işlem"})
			return
		}
		_, _ = db.DB.Exec("UPDATE blocks SET position = ? WHERE id = ?", i, blockID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Blok sıralaması güncellendi",
	})
}

// APIGetFormSubmissions form mesajlarını listeler.
func APIGetFormSubmissions(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	subs, err := db.GetFormSubmissions(page.ID)
	if err != nil {
		subs = []db.FormSubmission{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":       len(subs),
		"submissions": subs,
	})
}

// APIGetStats tıklama ve ziyaret istatistiklerini getirir.
// Query param: ?range=7d|30d|all (varsayılan: 7d)
func APIGetStats(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	page, err := db.GetPageByUserID(user.ID)
	if err != nil || page == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	rangeParam := r.URL.Query().Get("range")
	var daysFilter string
	var rangeLabel string
	switch rangeParam {
	case "30d":
		daysFilter = "-30 days"
		rangeLabel = "30d"
	case "all":
		daysFilter = ""
		rangeLabel = "all"
	default:
		daysFilter = "-7 days"
		rangeLabel = "7d"
	}

	// 1. Link bloklarının tıklanma sayılarını al
	rows, err := db.DB.Query(
		"SELECT id, title, content, clicks FROM blocks WHERE page_id = ? AND type = 'link' ORDER BY clicks DESC",
		page.ID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LinkStat struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		URL    string `json:"url"`
		Clicks int    `json:"clicks"`
	}

	var stats []LinkStat
	for rows.Next() {
		var s LinkStat
		if err := rows.Scan(&s.ID, &s.Title, &s.URL, &s.Clicks); err == nil {
			stats = append(stats, s)
		}
	}
	if stats == nil {
		stats = []LinkStat{}
	}

	// 2. Toplam sayfa görüntüleme sayısı
	var totalViews int
	if daysFilter != "" {
		_ = db.DB.QueryRow(
			"SELECT COUNT(*) FROM analytics_events WHERE page_id = ? AND event_type = 'page_view' AND created_at >= date('now', ?)",
			page.ID, daysFilter,
		).Scan(&totalViews)
	} else {
		_ = db.DB.QueryRow("SELECT COUNT(*) FROM analytics_events WHERE page_id = ? AND event_type = 'page_view'", page.ID).Scan(&totalViews)
	}

	// 3. Toplam link ve sosyal medya tıklama sayısı
	var totalClicks int
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM analytics_events WHERE page_id = ? AND event_type IN ('link_click', 'social_click')", page.ID).Scan(&totalClicks)

	// 4. Son periyot grafik verisi
	type DailyView struct {
		Date  string `json:"date"`
		Views int    `json:"views"`
	}
	var dailyViews []DailyView
	var dailyRows_err error
	var dailyQuery string
	if daysFilter != "" {
		dailyQuery = `SELECT date(created_at) as view_date, COUNT(*) as view_count 
		 FROM analytics_events 
		 WHERE page_id = ? AND event_type = 'page_view' AND created_at >= date('now', ?)
		 GROUP BY view_date ORDER BY view_date ASC`
		dailyRows, dErr := db.DB.Query(dailyQuery, page.ID, daysFilter)
		dailyRows_err = dErr
		if dErr == nil {
			defer dailyRows.Close()
			for dailyRows.Next() {
				var dv DailyView
				if err := dailyRows.Scan(&dv.Date, &dv.Views); err == nil {
					dailyViews = append(dailyViews, dv)
				}
			}
		}
	} else {
		dailyQuery = `SELECT date(created_at) as view_date, COUNT(*) as view_count 
		 FROM analytics_events 
		 WHERE page_id = ? AND event_type = 'page_view'
		 GROUP BY view_date ORDER BY view_date ASC`
		dailyRows, dErr := db.DB.Query(dailyQuery, page.ID)
		dailyRows_err = dErr
		if dErr == nil {
			defer dailyRows.Close()
			for dailyRows.Next() {
				var dv DailyView
				if err := dailyRows.Scan(&dv.Date, &dv.Views); err == nil {
					dailyViews = append(dailyViews, dv)
				}
			}
		}
	}
	_ = dailyRows_err
	if dailyViews == nil {
		dailyViews = []DailyView{}
	}

	// 5. Cihaz kırılımı
	type DeviceStat struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var devices []DeviceStat
	deviceRows, err := db.DB.Query(
		`SELECT device, COUNT(*) as dev_count 
		 FROM analytics_events 
		 WHERE page_id = ? AND event_type = 'page_view'
		 GROUP BY device
		 ORDER BY dev_count DESC`,
		page.ID,
	)
	if err == nil {
		defer deviceRows.Close()
		for deviceRows.Next() {
			var ds DeviceStat
			if err := deviceRows.Scan(&ds.Name, &ds.Count); err == nil {
				devices = append(devices, ds)
			}
		}
	}
	if devices == nil {
		devices = []DeviceStat{}
	}

	// 6. Ülke kırılımı
	type CountryStat struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var countries []CountryStat
	countryRows, err := db.DB.Query(
		`SELECT country, COUNT(*) as c_count 
		 FROM analytics_events 
		 WHERE page_id = ? AND event_type = 'page_view'
		 GROUP BY country
		 ORDER BY c_count DESC
		 LIMIT 10`,
		page.ID,
	)
	if err == nil {
		defer countryRows.Close()
		for countryRows.Next() {
			var cs CountryStat
			if err := countryRows.Scan(&cs.Name, &cs.Count); err == nil {
				countries = append(countries, cs)
			}
		}
	}
	if countries == nil {
		countries = []CountryStat{}
	}

	// 7. Sosyal Medya İkon Tıklamaları
	type SocialClickStat struct {
		Platform string `json:"platform"`
		Clicks   int    `json:"clicks"`
	}
	var socialClicks []SocialClickStat
	socialRows, err := db.DB.Query(
		`SELECT platform, COUNT(*) as p_count 
		 FROM analytics_events 
		 WHERE page_id = ? AND event_type = 'social_click' AND platform != ''
		 GROUP BY platform
		 ORDER BY p_count DESC`,
		page.ID,
	)
	if err == nil {
		defer socialRows.Close()
		for socialRows.Next() {
			var sc SocialClickStat
			if err := socialRows.Scan(&sc.Platform, &sc.Clicks); err == nil {
				socialClicks = append(socialClicks, sc)
			}
		}
	}
	if socialClicks == nil {
		socialClicks = []SocialClickStat{}
	}

	// Hepsini birleştirip JSON dön
	response := map[string]interface{}{
		"range":         rangeLabel,
		"link_clicks":   stats,
		"total_views":   totalViews,
		"total_clicks":  totalClicks,
		"daily_views":   dailyViews,
		"devices":       devices,
		"countries":     countries,
		"social_clicks": socialClicks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// APIGetAnalytics genel analitik özeti döner.
func APIGetAnalytics(w http.ResponseWriter, r *http.Request) {
	APIGetStats(w, r)
}

// APIGetSystemStats returns system health and metrics (uptime, memory, database size, total counts)
func APIGetSystemStats(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Yetkisiz işlem"})
		return
	}

	// Calculate uptime
	uptime := time.Since(startTime).Truncate(time.Second).String()

	// Memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	ramUsed := float64(m.Alloc) / 1024.0 / 1024.0 // MB

	// DB file size
	var dbSize int64
	dbInfo, err := os.Stat("./gotree.db")
	if err == nil {
		dbSize = dbInfo.Size()
	}

	// Database counts
	var totalPages int
	var totalBlocks int
	var totalClicks int
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM pages").Scan(&totalPages)
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&totalBlocks)
	_ = db.DB.QueryRow("SELECT COALESCE(SUM(clicks), 0) FROM blocks").Scan(&totalClicks)

	response := map[string]interface{}{
		"system": map[string]interface{}{
			"uptime":      uptime,
			"go_version":  runtime.Version(),
			"num_cpu":     runtime.NumCPU(),
			"goroutines":  runtime.NumGoroutine(),
			"ram_used_mb": fmt.Sprintf("%.2f MB", ramUsed),
			"db_size_kb":  dbSize / 1024,
		},
		"database": map[string]interface{}{
			"total_pages":  totalPages,
			"total_blocks": totalBlocks,
			"total_clicks": totalClicks,
		},
		"status": "healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// APIDeleteFormSubmission deletes a form submission by ID
func APIDeleteFormSubmission(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserContext(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Yetkisiz işlem"})
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "id parametresi gerekli"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Geçersiz id"})
		return
	}

	// Delete form submission helper
	err = db.DeleteFormSubmission(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Silme hatası"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Mesaj başarıyla silindi"})
}
