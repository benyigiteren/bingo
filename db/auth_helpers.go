package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// User veritabanındaki kullanıcı kaydıdır.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	APIKey       sql.NullString
	CreatedAt    time.Time
}

// HasUsers sistemde en az bir kullanıcının olup olmadığını kontrol eder.
func HasUsers() (bool, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateUser yeni bir kullanıcı oluşturur.
func CreateUser(username, passwordHash, role string) (int64, error) {
	// Benzersiz bir API key oluştur
	b := make([]byte, 24)
	var apiKey string
	if _, err := rand.Read(b); err == nil {
		apiKey = "bl_" + hex.EncodeToString(b)
	}

	result, err := DB.Exec(
		"INSERT INTO users (username, password_hash, role, api_key) VALUES (?, ?, ?, ?)",
		username, passwordHash, role, apiKey,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetUserByUsername kullanıcı adına göre kullanıcıyı getirir.
func GetUserByUsername(username string) (*User, error) {
	var u User
	err := DB.QueryRow(
		"SELECT id, username, password_hash, role, api_key, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.APIKey, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GenerateAPIKey kullanıcı için yeni bir API key oluşturur.
func GenerateAPIKey(userID int64) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := "bl_" + hex.EncodeToString(b)
	_, err := DB.Exec("UPDATE users SET api_key = ? WHERE id = ?", key, userID)
	if err != nil {
		return "", err
	}
	return key, nil
}

// GetUserByAPIKey API anahtarı ile kullanıcıyı getirir (API Auth için).
func GetUserByAPIKey(apiKey string) (*User, error) {
	var u User
	err := DB.QueryRow(
		"SELECT id, username, role, api_key, created_at FROM users WHERE api_key = ?",
		apiKey,
	).Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// CreateSession yeni bir oturum token'ı oluşturur.
func CreateSession(userID int64, duration time.Duration) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(duration)

	_, err := DB.Exec(
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expiresAt,
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// GetUserBySessionToken geçerli oturum token'ına sahip kullanıcıyı getirir.
func GetUserBySessionToken(token string) (*User, error) {
	var u User
	var expiresAt time.Time

	err := DB.QueryRow(
		`SELECT u.id, u.username, u.role, u.api_key, s.expires_at 
		 FROM sessions s 
		 JOIN users u ON s.user_id = u.id 
		 WHERE s.token = ?`,
		token,
	).Scan(&u.ID, &u.Username, &u.Role, &u.APIKey, &expiresAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if time.Now().After(expiresAt) {
		_ = DeleteSession(token)
		return nil, nil
	}

	return &u, nil
}

// DeleteSession oturumu siler.
func DeleteSession(token string) error {
	_, err := DB.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// UpdateUsername kullanıcının kullanıcı adını günceller.
func UpdateUsername(userID int64, newUsername string) error {
	_, err := DB.Exec("UPDATE users SET username = ? WHERE id = ?", newUsername, userID)
	return err
}

// UpdatePassword kullanıcının şifresini günceller.
func UpdatePassword(userID int64, newPasswordHash string) error {
	_, err := DB.Exec("UPDATE users SET password_hash = ? WHERE id = ?", newPasswordHash, userID)
	return err
}
