// Token keshi: olingan token muddati bilan birga diskda saqlanadi —
// har so'rovda qaytadan login qilinmasin.
package support

import (
	"encoding/json"
	"os"
	"time"
)

// TokenFile — token diskda shu faylda saqlanadi.
const TokenFile = "token.json"

// TokenCache diskda saqlanadigan token va uning muddati.
type TokenCache struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LoadToken faylni o'qiydi. Fayl yo'q, buzuq yoki muddati o'tgan bo'lsa nil
// qaytaradi — bu xato emas, shunchaki "cache yo'q" degani.
func LoadToken(path string) *TokenCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var tc TokenCache
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil
	}
	if tc.Token == "" || time.Now().After(tc.ExpiresAt) {
		return nil
	}
	return &tc
}

// SaveToken tokenni ttl muddati bilan faylga yozadi (0600 ruxsat — token maxfiy).
func SaveToken(path, token string, ttl time.Duration) error {
	data, err := json.MarshalIndent(TokenCache{
		Token:     token,
		ExpiresAt: time.Now().Add(ttl),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DropToken cache faylini o'chiradi (401 kelganda chaqiriladi).
func DropToken(path string) {
	_ = os.Remove(path)
}
