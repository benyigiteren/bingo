package db

import (
	"database/sql"
	"errors"
)

// GetSetting anahtara karşılık gelen ayar değerini döner.
func GetSetting(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil // Bulunamadı
		}
		return "", err
	}
	return value, nil
}

// SetSetting ayar değerini kaydeder veya günceller.
func SetSetting(key, value string) error {
	_, err := DB.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}
