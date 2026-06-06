package handlers

import (
	"bytes"
	"gotree/db"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SocialLinks sosyal medya linklerini yapısallaştırmak için kullanılır.
type SocialLinks struct {
	Instagram string `json:"instagram"`
	Twitter   string `json:"twitter"`
	GitHub    string `json:"github"`
	YouTube   string `json:"youtube"`
	LinkedIn  string `json:"linkedin"`
	Email     string `json:"email"`
	Discord   string `json:"discord"`
	Mastodon  string `json:"mastodon"`
	TikTok    string `json:"tiktok"`
	Telegram  string `json:"telegram"`
	WhatsApp  string `json:"whatsapp"`
	Pinterest string `json:"pinterest"`
	Reddit    string `json:"reddit"`
	Twitch    string `json:"twitch"`
	Snapchat  string `json:"snapchat"`
}

// BioBlock biyografi sayfasında render edilecek bloğu ve çözümlenmiş ek verileri tutar.
type BioBlock struct {
	db.Block
	Socials *SocialLinks
}

// BioViewModel biyografi şablonu veri modelidir.
type BioViewModel struct {
	Page   *db.Page
	Blocks []BioBlock
}

// BioGet /{slug} isteklerini işler ve biyografi sayfasını gösterir.
func BioGet(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	renderBio(w, r, slug)
}

// BioGetBySlug doğrudan belirtilen slug'a ait biyografi sayfasını render eder (ana sayfa yönlendirmesi için).
func BioGetBySlug(w http.ResponseWriter, r *http.Request, slug string) {
	renderBio(w, r, slug)
}

// renderBio veritabanından biyografi sayfasını ve bloklarını çeker ve templates/bio.html ile render eder.
func renderBio(w http.ResponseWriter, r *http.Request, slug string) {
	page, err := db.GetPageBySlug(slug)
	if err != nil {
		http.Error(w, "Veritabanı hatası", http.StatusInternalServerError)
		return
	}

	if page == nil {
		http.NotFound(w, r)
		return
	}

	blocks, err := db.GetBlocks(page.ID)
	if err != nil {
		http.Error(w, "Bloklar yüklenirken hata oluştu", http.StatusInternalServerError)
		return
	}

	// Sosyal medya bloklarının JSON verilerini çöz
	var bioBlocks []BioBlock
	for _, b := range blocks {
		bb := BioBlock{Block: b}
		if b.Type == "social_links" {
			var socs SocialLinks
			if err := json.Unmarshal([]byte(b.Content), &socs); err == nil {
				bb.Socials = &socs
			}
		}
		bioBlocks = append(bioBlocks, bb)
	}

	vm := BioViewModel{
		Page:   page,
		Blocks: bioBlocks,
	}

	// Sayfa ziyareti analitiğini asenkron olarak kaydet
	go db.LogAnalyticsEvent(page.ID, "page_view", 0, "", r)

	RenderTemplate(w, "bio.html", vm)
}

// RedirectHandler /r/{id} ve /r/{id}/{platform} yönlendirme isteklerini yakalar, tıklamayı arttırır ve yönlendirir.
func RedirectHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var blockType string
	var content string

	err = db.DB.QueryRow("SELECT type, content FROM blocks WHERE id = ?", id).Scan(&blockType, &content)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	platform := r.PathValue("platform")

	if platform == "" {
		// Standart Link yönlendirmesi
		if blockType == "link" && content != "" {
			_ = db.IncrementBlockClicks(id)
			
			var pageID int64
			err = db.DB.QueryRow("SELECT page_id FROM blocks WHERE id = ?", id).Scan(&pageID)
			if err == nil {
				go db.LogAnalyticsEvent(pageID, "link_click", id, "", r)
			}
			
			http.Redirect(w, r, content, http.StatusFound)
			return
		}
	} else {
		// Sosyal medya ikon tıklama yönlendirmesi
		if blockType == "social_links" && content != "" {
			var socials map[string]string
			if err := json.Unmarshal([]byte(content), &socials); err == nil {
				targetURL, ok := socials[platform]
				if ok && targetURL != "" {
					// Eğer email ise ve mailto: içermiyorsa ekle
					if platform == "email" && !strings.HasPrefix(strings.ToLower(targetURL), "mailto:") {
						targetURL = "mailto:" + targetURL
					} else if platform != "email" && !strings.HasPrefix(strings.ToLower(targetURL), "http://") && !strings.HasPrefix(strings.ToLower(targetURL), "https://") {
						// Kullanıcı sadece kullanıcı adı veya eksik URL girmişse ilgili platforma göre tamamla
						switch platform {
						case "instagram":
							targetURL = "https://instagram.com/" + targetURL
						case "twitter":
							targetURL = "https://twitter.com/" + targetURL
						case "github":
							targetURL = "https://github.com/" + targetURL
						case "youtube":
							targetURL = "https://youtube.com/" + targetURL
						case "linkedin":
							targetURL = "https://linkedin.com/in/" + targetURL
						case "discord":
							if !strings.Contains(targetURL, "/") {
								targetURL = "https://discord.gg/" + targetURL
							}
						case "mastodon":
							if strings.HasPrefix(targetURL, "@") {
								parts := strings.Split(strings.TrimPrefix(targetURL, "@"), "@")
								if len(parts) == 2 {
									targetURL = "https://" + parts[1] + "/@" + parts[0]
								}
							}
						}
					}

					// Sosyal bloğun genel tıklama sayacını arttır
					_ = db.IncrementBlockClicks(id)

					var pageID int64
					err = db.DB.QueryRow("SELECT page_id FROM blocks WHERE id = ?", id).Scan(&pageID)
					if err == nil {
						go db.LogAnalyticsEvent(pageID, "social_click", id, platform, r)
					}

					http.Redirect(w, r, targetURL, http.StatusFound)
					return
				}
			}
		}
	}

	http.NotFound(w, r)
}

// FormSubmitHandler handles AJAX visitor submissions for contact forms.
func FormSubmitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Geçersiz form ID"})
		return
	}

	// Fetch block
	var blockType string
	var pageID int64
	var settingsJSON string
	err = db.DB.QueryRow("SELECT type, page_id, content FROM blocks WHERE id = ?", id).Scan(&blockType, &pageID, &settingsJSON)
	if err != nil || blockType != "form" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Form bulunamadı"})
		return
	}

	// Spam protection: Honeypot check
	if r.FormValue("website") != "" {
		// Silently ignore spam bots by returning success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Mesajınız başarıyla iletildi!"})
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	message := r.FormValue("message")

	// Basic validation
	if name == "" || email == "" || message == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Lütfen tüm alanları doldurun"})
		return
	}

	// Save submission
	_, err = db.CreateFormSubmission(pageID, id, name, email, message)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Mesajınız kaydedilirken hata oluştu"})
		return
	}

	// Trigger notifications asynchronously
	if settingsJSON != "" {
		go sendNotifications(settingsJSON, name, email, message)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Mesajınız başarıyla iletildi!",
	})
}

// sendNotifications dispatches email/webhook alerts in the background.
func sendNotifications(settingsJson string, name, email, message string) {
	var settings struct {
		DiscordWebhookURL string `json:"discord_webhook_url"`
		TelegramBotToken  string `json:"telegram_bot_token"`
		TelegramChatID    string `json:"telegram_chat_id"`
		SMTPHost          string `json:"smtp_host"`
		SMTPPort          string `json:"smtp_port"`
		SMTPUser          string `json:"smtp_user"`
		SMTPPass          string `json:"smtp_pass"`
		SMTPTo            string `json:"smtp_to"`
	}

	if err := json.Unmarshal([]byte(settingsJson), &settings); err != nil {
		return
	}

	// 1. Discord Webhook
	if settings.DiscordWebhookURL != "" {
		go func() {
			payload := map[string]interface{}{
				"embeds": []map[string]interface{}{
					{
						"title": "📩 Yeni Gotree Form Mesajı",
						"color": 0x50311f, // Gotree brand color
						"fields": []map[string]interface{}{
							{"name": "👤 İsim", "value": name, "inline": true},
							{"name": "✉️ E-posta", "value": email, "inline": true},
							{"name": "💬 Mesaj", "value": message},
						},
						"timestamp": time.Now().Format(time.RFC3339),
					},
				},
			}
			data, err := json.Marshal(payload)
			if err == nil {
				resp, err := http.Post(settings.DiscordWebhookURL, "application/json", bytes.NewBuffer(data))
				if err == nil {
					resp.Body.Close()
				}
			}
		}()
	}

	// 2. Telegram Bot
	if settings.TelegramBotToken != "" && settings.TelegramChatID != "" {
		go func() {
			text := fmt.Sprintf("📩 *Yeni Gotree Form Mesajı!*\n\n👤 *İsim:* %s\n✉️ *E-posta:* %s\n💬 *Mesaj:* %s", name, email, message)
			apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", settings.TelegramBotToken)
			payload := map[string]string{
				"chat_id":    settings.TelegramChatID,
				"text":       text,
				"parse_mode": "Markdown",
			}
			data, err := json.Marshal(payload)
			if err == nil {
				resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(data))
				if err == nil {
					resp.Body.Close()
				}
			}
		}()
	}

	// 3. SMTP Email
	if settings.SMTPHost != "" && settings.SMTPUser != "" && settings.SMTPPass != "" && settings.SMTPTo != "" {
		go func() {
			port := settings.SMTPPort
			if port == "" {
				port = "587"
			}
			addr := settings.SMTPHost + ":" + port
			auth := smtp.PlainAuth("", settings.SMTPUser, settings.SMTPPass, settings.SMTPHost)

			msg := []byte("To: " + settings.SMTPTo + "\r\n" +
				"Subject: Yeni Gotree İletişim Mesajı!\r\n" +
				"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
				"Gönderen: " + name + "\r\n" +
				"E-posta: " + email + "\r\n\r\n" +
				"Mesaj:\r\n" + message + "\r\n")

			_ = smtp.SendMail(addr, auth, settings.SMTPUser, []string{settings.SMTPTo}, msg)
		}()
	}
}
