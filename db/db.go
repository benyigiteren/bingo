package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// InitDB veritabanı dosyasını açar ve tabloları oluşturur.
func InitDB(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("veritabanı açılırken hata oluştu: %w", err)
	}

	DB.SetMaxOpenConns(1)

	// SQLite için foreign key kısıtlamalarını etkinleştir
	_, err = DB.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return fmt.Errorf("pragma foreign_keys etkinleştirilemedi: %w", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("tablolar oluşturulurken hata: %w", err)
	}

	// Tablolardaki yeni kolonların otomatik eklenmesi (Migration)
	runMigrations()

	log.Printf("SQLite veritabanı başarıyla ilklendirildi: %s", dbPath)
	return nil
}

// createTables gerekli tabloları oluşturur.
func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user', -- 'superadmin', 'admin', 'user'
			api_key TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS pages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			bio TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			theme TEXT NOT NULL DEFAULT 'default',
			background_image_url TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS blocks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			page_id INTEGER NOT NULL,
			type TEXT NOT NULL, -- 'link', 'text', 'social_links'
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			style TEXT NOT NULL DEFAULT 'standard', -- 'standard', 'metallic', 'brutalist', 'rounded', 'outline'
			icon TEXT NOT NULL DEFAULT '', -- 'website', 'portfolio', 'instagram', 'github', etc.
			clicks INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
	}

	for _, query := range queries {
		if _, err := DB.Exec(query); err != nil {
			return err
		}
	}

	// Benzersiz API Anahtarı İndeksini oluştur
	_, _ = DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key);")

	return nil
}

// runMigrations var olan veritabanlarına yeni eklenen alanları ekler.
func runMigrations() {
	// SQLite'ta UNIQUE kolonu doğrudan ALTER TABLE ADD COLUMN ile eklenemez (hata verir).
	// Bu nedenle önce kolonu ekliyoruz, ardından UNIQUE index oluşturuyoruz.
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN api_key TEXT;")
	_, _ = DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key);")

	_, _ = DB.Exec("ALTER TABLE blocks ADD COLUMN clicks INTEGER NOT NULL DEFAULT 0;")
	_, _ = DB.Exec("ALTER TABLE blocks ADD COLUMN style TEXT NOT NULL DEFAULT 'standard';")
	_, _ = DB.Exec("ALTER TABLE blocks ADD COLUMN icon TEXT NOT NULL DEFAULT '';")

	// İterasyon 3 Migrasyonları
	_, _ = DB.Exec("ALTER TABLE pages ADD COLUMN background_image_url TEXT NOT NULL DEFAULT '';")
	_, _ = DB.Exec(`CREATE TABLE IF NOT EXISTS analytics_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		page_id INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		block_id INTEGER,
		country TEXT NOT NULL DEFAULT 'Bilinmeyen',
		device TEXT NOT NULL DEFAULT 'Masaüstü',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE,
		FOREIGN KEY (block_id) REFERENCES blocks(id) ON DELETE CASCADE
	);`)
	_, _ = DB.Exec("ALTER TABLE analytics_events ADD COLUMN platform TEXT NOT NULL DEFAULT '';")

	// İterasyon 4 Migrasyonları (Kırpma, Otomatik Kayıt, Form Bloğu, Webhook)
	_, _ = DB.Exec("ALTER TABLE blocks ADD COLUMN icon_size TEXT NOT NULL DEFAULT 'small';")
	_, _ = DB.Exec(`CREATE TABLE IF NOT EXISTS form_submissions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		page_id INTEGER NOT NULL,
		block_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		message TEXT NOT NULL,
		submitted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE,
		FOREIGN KEY (block_id) REFERENCES blocks(id) ON DELETE CASCADE
	);`)
}

