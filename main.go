package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"gotree/db"
	"gotree/handlers"
	"gotree/middleware"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	var defaultPort = "1907"
	var defaultDBPath = "./gotree.db"

	// Bellek Tüketimini Optimize Eden Arka Plan Rutini (GOGC ayarı ve FreeOSMemory)
	go func() {
		// Çöp toplama sıklığını arttırarak heap boyutunu küçük tut (RAM hedefi <25MB)
		debug.SetGCPercent(20)
		for {
			time.Sleep(15 * time.Second)
			runtime.GC()
			debug.FreeOSMemory()
		}
	}()

	// CLI Bayrakları
	var portFlag string
	var dbPathFlag string
	
	// go run veya go build sonrasında port ve db yollarını argümanlardan almak için flag tanımları
	// standart flag kütüphanesi yerine basit argüman analizi veya flag kütüphanesini kullanabiliriz
	// birden fazla çalıştırmada flag redefined hatası olmaması için flag kütüphanesini doğrudan ilklendiriyoruz
	for i, arg := range os.Args {
		if arg == "-port" && i+1 < len(os.Args) {
			portFlag = os.Args[i+1]
		}
		if arg == "-db" && i+1 < len(os.Args) {
			dbPathFlag = os.Args[i+1]
		}
	}

	port := portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = defaultPort
	}

	dbPath := dbPathFlag
	if dbPath == "" {
		dbPath = os.Getenv("DB_PATH")
	}
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	// 1. Veritabanını ilklendir
	if err := db.InitDB(dbPath); err != nil {
		log.Fatalf("Veritabanı başlatılamadı: %v", err)
	}
	defer db.DB.Close()

	// 2. Gömülü HTML şablonlarını yükle
	if err := handlers.LoadTemplates(templatesFS); err != nil {
		log.Fatalf("Şablonlar yüklenemedi: %v", err)
	}

	// 3. Gömülü statik dosyaları ayarla
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Statik dosya sistemi alt klasör oluşturma hatası: %v", err)
	}

	// 4. Uploads klasörünü oluştur ve ayarla (profil resimleri için)
	if err := os.MkdirAll("./uploads", 0755); err != nil {
		log.Fatalf("Uploads klasörü oluşturulamadı: %v", err)
	}

	// 5. HTTP Yönlendirici (Go 1.22+ ServeMux)
	mux := http.NewServeMux()

	// Statik Dosyalar (Embedded)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Uploads Klasörü (Profil Resimleri)
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))

	// Kurulum (Setup) Ekranları
	mux.HandleFunc("GET /setup", handlers.SetupGet)
	mux.HandleFunc("POST /setup", handlers.SetupPost)

	// Giriş & Çıkış İşlemleri
	mux.HandleFunc("GET /login", handlers.LoginGet)
	mux.HandleFunc("POST /login", handlers.LoginPost)
	mux.HandleFunc("GET /logout", handlers.Logout)

	// Yönlendirme ve İstatistik İzleme
	mux.HandleFunc("GET /r/{id}", handlers.RedirectHandler)
	mux.HandleFunc("GET /r/{id}/{platform}", handlers.RedirectHandler)

	// Yönetim Paneli Rotaları (Oturum kontrolü altındadır)
	mux.Handle("GET /admin", middleware.RequireAuth(http.HandlerFunc(handlers.AdminGet)))
	mux.Handle("POST /admin/page/create", middleware.RequireAuth(http.HandlerFunc(handlers.PageCreatePost)))
	mux.Handle("POST /admin/page/update", middleware.RequireAuth(http.HandlerFunc(handlers.PageUpdatePost)))
	mux.Handle("POST /admin/page/autosave", middleware.RequireAuth(http.HandlerFunc(handlers.PageAutosave)))
	mux.Handle("POST /admin/block/create", middleware.RequireAuth(http.HandlerFunc(handlers.BlockCreatePost)))
	mux.Handle("POST /admin/block/update", middleware.RequireAuth(http.HandlerFunc(handlers.BlockUpdatePost)))
	mux.Handle("POST /admin/block/autosave", middleware.RequireAuth(http.HandlerFunc(handlers.BlockAutosave)))
	mux.Handle("POST /admin/block/upload-image", middleware.RequireAuth(http.HandlerFunc(handlers.BlockUploadImage)))
	mux.Handle("POST /admin/block/delete", middleware.RequireAuth(http.HandlerFunc(handlers.BlockDeletePost)))
	mux.Handle("POST /admin/block/reorder", middleware.RequireAuth(http.HandlerFunc(handlers.BlockReorderPost)))
	mux.Handle("POST /admin/api-key/regenerate", middleware.RequireAuth(http.HandlerFunc(handlers.APIKeyRegeneratePost)))
	mux.Handle("POST /admin/users/create", middleware.RequireAuth(http.HandlerFunc(handlers.UserCreatePost)))
	mux.Handle("POST /admin/users/delete", middleware.RequireAuth(http.HandlerFunc(handlers.UserDeletePost)))
	mux.Handle("POST /admin/profile/update", middleware.RequireAuth(http.HandlerFunc(handlers.ProfileUpdatePost)))

	// Yeni Profil/Kırpma ve Form Bloğu Yönetim Yönlendirmeleri
	mux.Handle("POST /admin/page/upload-avatar", middleware.RequireAuth(http.HandlerFunc(handlers.PageUploadAvatar)))
	mux.Handle("POST /admin/page/upload-bg", middleware.RequireAuth(http.HandlerFunc(handlers.PageUploadBg)))
	mux.Handle("POST /admin/form-submissions/delete", middleware.RequireAuth(http.HandlerFunc(handlers.FormSubmissionDelete)))
	mux.HandleFunc("POST /form/submit/{id}", handlers.FormSubmitHandler)

	// API Geliştirici Dokümantasyonu (Etkileşimli ve Açık)
	mux.HandleFunc("GET /api/docs", handlers.APIDocsGet)
	mux.HandleFunc("GET /api/v1/help", handlers.APIHelp)

	// API REST Uç Noktaları (X-API-Key veya Çerez doğrulaması zorunludur)
	mux.Handle("GET /api/v1/page", middleware.RequireAuth(http.HandlerFunc(handlers.APIGetPage)))
	mux.Handle("POST /api/v1/page", middleware.RequireAuth(http.HandlerFunc(handlers.APIUpdatePage)))
	mux.Handle("PUT /api/v1/page", middleware.RequireAuth(http.HandlerFunc(handlers.APIUpdatePage)))
	mux.Handle("GET /api/v1/blocks", middleware.RequireAuth(http.HandlerFunc(handlers.APIGetBlocks)))
	mux.Handle("POST /api/v1/blocks", middleware.RequireAuth(http.HandlerFunc(handlers.APICreateBlock)))
	mux.Handle("POST /api/v1/blocks/update", middleware.RequireAuth(http.HandlerFunc(handlers.APIUpdateBlock)))
	mux.Handle("PUT /api/v1/blocks/update", middleware.RequireAuth(http.HandlerFunc(handlers.APIUpdateBlock)))
	mux.Handle("POST /api/v1/blocks/reorder", middleware.RequireAuth(http.HandlerFunc(handlers.APIReorderBlocks)))
	mux.Handle("DELETE /api/v1/blocks", middleware.RequireAuth(http.HandlerFunc(handlers.APIDeleteBlock)))
	mux.Handle("GET /api/v1/stats", middleware.RequireAuth(http.HandlerFunc(handlers.APIGetStats)))
	mux.Handle("GET /api/v1/form-submissions", middleware.RequireAuth(http.HandlerFunc(handlers.APIGetFormSubmissions)))
	mux.Handle("DELETE /api/v1/form-submissions", middleware.RequireAuth(http.HandlerFunc(handlers.APIDeleteFormSubmission)))
	mux.Handle("GET /api/v1/system/stats", middleware.RequireAuth(http.HandlerFunc(handlers.APIGetSystemStats)))
	// Herkese açık profil endpoint'i (Auth gerekmez)
	mux.HandleFunc("GET /api/v1/profile/{slug}", handlers.APIGetPublicProfile)

	// Ana Dizin Yönlendirmesi
	mux.HandleFunc("GET /", handlers.IndexHandler)

	// Biyografi Sayfaları (/{slug})
	mux.HandleFunc("GET /{slug}", handlers.BioGet)

	// Tüm istekleri kimlik doğrulama ve rate-limiting ara katmanından geçir
	globalHandler := middleware.RateLimit(middleware.Authenticate(mux))

	log.Printf("Gotree sunucusu :%s portunda dinleniyor...", port)
	if err := http.ListenAndServe(":"+port, globalHandler); err != nil {
		log.Fatalf("Sunucu hatası: %v", err)
	}
}
