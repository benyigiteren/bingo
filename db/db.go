package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	APIKey       string    `json:"api_key"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

type File struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username"` // Joined from users table
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	Views        int       `json:"views"`
	CreatedAt    time.Time `json:"created_at"`
}

type Stats struct {
	TotalFiles int64 `json:"total_files"`
	TotalSize  int64 `json:"total_size"` // bytes
	TotalViews int64 `json:"total_views"`
	TotalUsers int64 `json:"total_users"`
}

// InitDB initializes the SQLite database, sets WAL mode, and creates the schema
func InitDB(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Optimize SQLite performance for low RAM and high speed
	_, err = DB.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA temp_store=memory;
		PRAGMA foreign_keys=ON;
	`)
	if err != nil {
		return fmt.Errorf("failed to configure sqlite: %w", err)
	}

	// Create tables
	err = createTables()
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

func createTables() error {
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL, -- 'super_admin' or 'user'
		api_key TEXT UNIQUE NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL
	);`

	filesTable := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		original_name TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		mime_type TEXT NOT NULL,
		views INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(user_id, filename)
	);`

	_, err := DB.Exec(usersTable)
	if err != nil {
		return err
	}

	_, err = DB.Exec(filesTable)
	return err
}

// Helper to generate a random API Key
func generateRandomKey() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "bg_" + hex.EncodeToString(bytes), nil
}

// HasUsers checks if there are any users in the system
func HasUsers() (bool, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateUser registers a new user
func CreateUser(username, password, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	apiKey, err := generateRandomKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate api key: %w", err)
	}

	createdAt := time.Now()
	res, err := DB.Exec(
		"INSERT INTO users (username, password_hash, role, api_key, is_active, created_at) VALUES (?, ?, ?, ?, 1, ?)",
		username, string(hash), role, apiKey, createdAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:        id,
		Username:  username,
		Role:      role,
		APIKey:    apiKey,
		IsActive:  true,
		CreatedAt: createdAt,
	}, nil
}

// GetUserByUsername fetches a user by username
func GetUserByUsername(username string) (*User, error) {
	var u User
	var isActiveInt int
	err := DB.QueryRow(
		"SELECT id, username, password_hash, role, api_key, is_active, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.APIKey, &isActiveInt, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.IsActive = isActiveInt == 1
	return &u, nil
}

// GetUserByAPIKey fetches a user by API Key
func GetUserByAPIKey(apiKey string) (*User, error) {
	var u User
	var isActiveInt int
	err := DB.QueryRow(
		"SELECT id, username, role, api_key, is_active, created_at FROM users WHERE api_key = ?",
		apiKey,
	).Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &isActiveInt, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.IsActive = isActiveInt == 1
	return &u, nil
}

// GetUserByID fetches a user by ID
func GetUserByID(id int64) (*User, error) {
	var u User
	var isActiveInt int
	err := DB.QueryRow(
		"SELECT id, username, role, api_key, is_active, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &isActiveInt, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.IsActive = isActiveInt == 1
	return &u, nil
}

// GetUsers fetches all users
func GetUsers() ([]User, error) {
	rows, err := DB.Query("SELECT id, username, role, api_key, is_active, created_at FROM users ORDER BY username ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var isActiveInt int
		err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &isActiveInt, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		u.IsActive = isActiveInt == 1
		users = append(users, u)
	}
	return users, nil
}

// ToggleUserStatus enables or disables a user account
func ToggleUserStatus(id int64, active bool) error {
	isActiveInt := 0
	if active {
		isActiveInt = 1
	}
	_, err := DB.Exec("UPDATE users SET is_active = ? WHERE id = ?", isActiveInt, id)
	return err
}

// RegenerateAPIKey sets a new API key for the user
func RegenerateAPIKey(userID int64) (string, error) {
	apiKey, err := generateRandomKey()
	if err != nil {
		return "", err
	}

	_, err = DB.Exec("UPDATE users SET api_key = ? WHERE id = ?", apiKey, userID)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

// DeleteUser deletes a user and cascade deletes their files in SQLite
func DeleteUser(id int64) error {
	_, err := DB.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// CreateFile registers a new file record
func CreateFile(userID int64, filename, originalName string, fileSize int64, mimeType string) (*File, error) {
	createdAt := time.Now()
	_, err := DB.Exec(
		"INSERT INTO files (user_id, filename, original_name, file_size, mime_type, views, created_at) VALUES (?, ?, ?, ?, ?, 0, ?)",
		userID, filename, originalName, fileSize, mimeType, createdAt,
	)
	if err != nil {
		return nil, err
	}

	// Fetch username for returned struct
	var username string
	err = DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		return nil, err
	}

	var fileID int64
	err = DB.QueryRow("SELECT last_insert_rowid()").Scan(&fileID)
	if err != nil {
		return nil, err
	}

	return &File{
		ID:           fileID,
		UserID:       userID,
		Username:     username,
		Filename:     filename,
		OriginalName: originalName,
		FileSize:     fileSize,
		MimeType:     mimeType,
		Views:        0,
		CreatedAt:    createdAt,
	}, nil
}

// GetFile fetches metadata for a file by user and filename
func GetFile(username, filename string) (*File, error) {
	var f File
	err := DB.QueryRow(`
		SELECT f.id, f.user_id, u.username, f.filename, f.original_name, f.file_size, f.mime_type, f.views, f.created_at
		FROM files f
		JOIN users u ON f.user_id = u.id
		WHERE u.username = ? AND f.filename = ?`,
		username, filename,
	).Scan(&f.ID, &f.UserID, &f.Username, &f.Filename, &f.OriginalName, &f.FileSize, &f.MimeType, &f.Views, &f.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// GetFileByID fetches file metadata by file ID
func GetFileByID(fileID int64) (*File, error) {
	var f File
	err := DB.QueryRow(`
		SELECT f.id, f.user_id, u.username, f.filename, f.original_name, f.file_size, f.mime_type, f.views, f.created_at
		FROM files f
		JOIN users u ON f.user_id = u.id
		WHERE f.id = ?`,
		fileID,
	).Scan(&f.ID, &f.UserID, &f.Username, &f.Filename, &f.OriginalName, &f.FileSize, &f.MimeType, &f.Views, &f.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// IncrementFileViews increases the view count by 1
func IncrementFileViews(fileID int64) error {
	_, err := DB.Exec("UPDATE files SET views = views + 1 WHERE id = ?", fileID)
	return err
}

// DeleteFile deletes a file record
func DeleteFile(fileID int64) error {
	_, err := DB.Exec("DELETE FROM files WHERE id = ?", fileID)
	return err
}

// GetFiles fetches files for a specific user
func GetFiles(userID int64, limit, offset int) ([]File, error) {
	rows, err := DB.Query(`
		SELECT f.id, f.user_id, u.username, f.filename, f.original_name, f.file_size, f.mime_type, f.views, f.created_at
		FROM files f
		JOIN users u ON f.user_id = u.id
		WHERE f.user_id = ?
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		err := rows.Scan(&f.ID, &f.UserID, &f.Username, &f.Filename, &f.OriginalName, &f.FileSize, &f.MimeType, &f.Views, &f.CreatedAt)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// GetAllFiles fetches all files in the system (Super Admin view)
func GetAllFiles(limit, offset int) ([]File, error) {
	rows, err := DB.Query(`
		SELECT f.id, f.user_id, u.username, f.filename, f.original_name, f.file_size, f.mime_type, f.views, f.created_at
		FROM files f
		JOIN users u ON f.user_id = u.id
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		err := rows.Scan(&f.ID, &f.UserID, &f.Username, &f.Filename, &f.OriginalName, &f.FileSize, &f.MimeType, &f.Views, &f.CreatedAt)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// GetStats fetches high level file statistics
func GetStats() (Stats, error) {
	var s Stats
	// Count users
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&s.TotalUsers)
	if err != nil {
		return s, err
	}

	// Count files
	err = DB.QueryRow("SELECT COUNT(*), COALESCE(SUM(file_size), 0), COALESCE(SUM(views), 0) FROM files").Scan(&s.TotalFiles, &s.TotalSize, &s.TotalViews)
	if err != nil {
		return s, err
	}

	return s, nil
}
