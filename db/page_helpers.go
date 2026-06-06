package db

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Page biyografi sayfasını temsil eder.
type Page struct {
	ID                 int64
	UserID             int64
	Slug               string
	Title              string
	Bio                string
	AvatarURL          string
	Theme              string
	BackgroundImageURL string
	CreatedAt          time.Time
}

// Block içerik bloklarını (Link, Metin vb.) temsil eder.
type Block struct {
	ID        int64
	PageID    int64
	Type      string // 'link', 'text', 'social_links'
	Title     string
	Content   string
	Style     string // 'standard', 'metallic', 'brutalist', 'rounded', 'outline'
	Icon      string // icon adı
	IconSize  string // 'small', 'large', 'banner'
	Clicks    int
	SortOrder int
	CreatedAt time.Time
}

// NormalizeTheme geçersiz tema isimlerini minimal_cream temasına eşler.
func NormalizeTheme(theme string) string {
	switch theme {
	case "minimal_cream", "brutalist_mint", "sunset_gradient", "olive_garden",
		"skater_peach", "forest_outline", "sunset_glow", "cosmic_space",
		"sticky_wicks", "galaxy_night", "metallic_luxury", "business_dark",
		"agency_minimal", "community_neon", "corp_professional":
		return theme
	default:
		return "minimal_cream"
	}
}

// GetPageByUserID kullanıcının sayfasını getirir.
func GetPageByUserID(userID int64) (*Page, error) {
	var p Page
	err := DB.QueryRow(
		"SELECT id, user_id, slug, title, bio, avatar_url, theme, background_image_url, created_at FROM pages WHERE user_id = ?",
		userID,
	).Scan(&p.ID, &p.UserID, &p.Slug, &p.Title, &p.Bio, &p.AvatarURL, &p.Theme, &p.BackgroundImageURL, &p.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Theme = NormalizeTheme(p.Theme)
	return &p, nil
}

// GetPageBySlug slug değerine göre sayfayı getirir.
func GetPageBySlug(slug string) (*Page, error) {
	var p Page
	err := DB.QueryRow(
		"SELECT id, user_id, slug, title, bio, avatar_url, theme, background_image_url, created_at FROM pages WHERE slug = ?",
		slug,
	).Scan(&p.ID, &p.UserID, &p.Slug, &p.Title, &p.Bio, &p.AvatarURL, &p.Theme, &p.BackgroundImageURL, &p.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Theme = NormalizeTheme(p.Theme)
	return &p, nil
}

// CreatePage yeni bir biyografi sayfası oluşturur.
func CreatePage(userID int64, slug, title, bio, avatarURL, theme string) (int64, error) {
	theme = NormalizeTheme(theme)
	result, err := DB.Exec(
		"INSERT INTO pages (user_id, slug, title, bio, avatar_url, theme, background_image_url) VALUES (?, ?, ?, ?, ?, ?, '')",
		userID, slug, title, bio, avatarURL, theme,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdatePage sayfa bilgilerini günceller.
func UpdatePage(id int64, slug, title, bio, avatarURL, theme, backgroundImageURL string) error {
	theme = NormalizeTheme(theme)
	_, err := DB.Exec(
		"UPDATE pages SET slug = ?, title = ?, bio = ?, avatar_url = ?, theme = ?, background_image_url = ? WHERE id = ?",
		slug, title, bio, avatarURL, theme, backgroundImageURL, id,
	)
	return err
}

// GetBlocks sayfaya ait tüm blokları sıralı olarak getirir.
func GetBlocks(pageID int64) ([]Block, error) {
	rows, err := DB.Query(
		"SELECT id, page_id, type, title, content, style, icon, icon_size, clicks, sort_order, created_at FROM blocks WHERE page_id = ? ORDER BY sort_order ASC, id ASC",
		pageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []Block
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.PageID, &b.Type, &b.Title, &b.Content, &b.Style, &b.Icon, &b.IconSize, &b.Clicks, &b.SortOrder, &b.CreatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// CreateBlock sayfaya yeni bir içerik bloğu ekler.
func CreateBlock(pageID int64, blockType, title, content string) (int64, error) {
	var maxOrder int
	err := DB.QueryRow("SELECT COALESCE(MAX(sort_order), 0) FROM blocks WHERE page_id = ?", pageID).Scan(&maxOrder)
	if err != nil {
		maxOrder = 0
	}

	result, err := DB.Exec(
		"INSERT INTO blocks (page_id, type, title, content, style, icon, icon_size, clicks, sort_order) VALUES (?, ?, ?, ?, 'standard', '', 'small', 0, ?)",
		pageID, blockType, title, content, maxOrder+1,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateBlock bir bloğu günceller.
func UpdateBlock(id int64, title, content, style, icon, iconSize string) error {
	if iconSize == "" {
		iconSize = "small"
	}
	_, err := DB.Exec(
		"UPDATE blocks SET title = ?, content = ?, style = ?, icon = ?, icon_size = ? WHERE id = ?",
		title, content, style, icon, iconSize, id,
	)
	return err
}

// IncrementBlockClicks tıklama sayısını 1 arttırır.
func IncrementBlockClicks(id int64) error {
	_, err := DB.Exec("UPDATE blocks SET clicks = clicks + 1 WHERE id = ?", id)
	return err
}

// DeleteBlock bir bloğu siler.
func DeleteBlock(id int64) error {
	_, err := DB.Exec("DELETE FROM blocks WHERE id = ?", id)
	return err
}

// UpdateBlockOrders blok sıralarını günceller.
func UpdateBlockOrders(orders map[int64]int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE blocks SET sort_order = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id, order := range orders {
		_, err = stmt.Exec(order, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetUsers tüm kullanıcıları listeler.
func GetUsers() ([]User, error) {
	rows, err := DB.Query("SELECT id, username, role, api_key, created_at FROM users ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// DeleteUser bir kullanıcıyı siler.
func DeleteUser(id int64) error {
	_, err := DB.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// LogAnalyticsEvent ziyaretçi olaylarını (sayfa görüntüleme veya link tıklama) kaydeder.
func LogAnalyticsEvent(pageID int64, eventType string, blockID int64, platform string, r *http.Request) {
	// 1. Cihaz tespiti (User-Agent parsing)
	ua := r.Header.Get("User-Agent")
	device := "Masaüstü"
	uaLower := strings.ToLower(ua)
	if strings.Contains(uaLower, "mobi") || strings.Contains(uaLower, "android") || strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipod") {
		if strings.Contains(uaLower, "ipad") || strings.Contains(uaLower, "tablet") {
			device = "Tablet"
		} else {
			device = "Mobil"
		}
	}

	// 2. Ülke tespiti
	country := r.Header.Get("CF-IPCountry")
	if country == "" {
		country = r.Header.Get("X-Country")
	}
	if country == "" {
		// Fallback: Accept-Language dil kodundan ülke çıkarımı (Lokal testler için çok yararlıdır)
		lang := r.Header.Get("Accept-Language")
		country = getCountryFromAcceptLanguage(lang)
	}

	// 3. Veritabanına kaydet
	var blockVal interface{} = nil
	if blockID > 0 {
		blockVal = blockID
	}

	_, _ = DB.Exec(
		"INSERT INTO analytics_events (page_id, event_type, block_id, country, device, platform) VALUES (?, ?, ?, ?, ?, ?)",
		pageID, eventType, blockVal, country, device, platform,
	)
}

func getCountryFromAcceptLanguage(lang string) string {
	if lang == "" {
		return "Bilinmeyen"
	}
	parts := strings.Split(lang, ",")
	first := strings.TrimSpace(parts[0])
	langCode := strings.Split(first, ";")[0]
	langCode = strings.ToLower(langCode)

	switch {
	case strings.HasPrefix(langCode, "tr"):
		return "Türkiye"
	case strings.HasPrefix(langCode, "en-us"):
		return "ABD"
	case strings.HasPrefix(langCode, "en-gb"):
		return "İngiltere"
	case strings.HasPrefix(langCode, "en"):
		return "ABD"
	case strings.HasPrefix(langCode, "de"):
		return "Almanya"
	case strings.HasPrefix(langCode, "fr"):
		return "Fransa"
	case strings.HasPrefix(langCode, "it"):
		return "İtalya"
	case strings.HasPrefix(langCode, "es"):
		return "İspanya"
	case strings.HasPrefix(langCode, "ru"):
		return "Rusya"
	case strings.HasPrefix(langCode, "nl"):
		return "Hollanda"
	case strings.HasPrefix(langCode, "az"):
		return "Azerbaycan"
	default:
		return "Diğer"
	}
}

// FormSubmission represents contact form messages from visitors.
type FormSubmission struct {
	ID          int64     `json:"id"`
	PageID      int64     `json:"page_id"`
	BlockID     int64     `json:"block_id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Message     string    `json:"message"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// CreateFormSubmission inserts a form entry.
func CreateFormSubmission(pageID, blockID int64, name, email, message string) (int64, error) {
	res, err := DB.Exec(
		"INSERT INTO form_submissions (page_id, block_id, name, email, message) VALUES (?, ?, ?, ?, ?)",
		pageID, blockID, name, email, message,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetFormSubmissions retrieves form submissions for a given page.
func GetFormSubmissions(pageID int64) ([]FormSubmission, error) {
	rows, err := DB.Query(
		"SELECT id, page_id, block_id, name, email, message, submitted_at FROM form_submissions WHERE page_id = ? ORDER BY submitted_at DESC",
		pageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []FormSubmission
	for rows.Next() {
		var s FormSubmission
		if err := rows.Scan(&s.ID, &s.PageID, &s.BlockID, &s.Name, &s.Email, &s.Message, &s.SubmittedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []FormSubmission{}
	}
	return subs, nil
}

// DeleteFormSubmission deletes a specific form message.
func DeleteFormSubmission(id int64) error {
	_, err := DB.Exec("DELETE FROM form_submissions WHERE id = ?", id)
	return err
}
