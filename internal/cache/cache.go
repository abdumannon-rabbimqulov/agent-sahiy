package cache

import (
	"encoding/json"
	"os"
	"time"
)

// TokenCache diskda saqlanadigan token va uning muddati.
type TokenCache struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Load token.json ni o'qiydi. Fayl yo'q, buzuq yoki muddati o'tgan bo'lsa
// nil qaytaradi (xato emas — shunchaki cache yo'q deb hisoblanadi).
func Load(path string) *TokenCache {
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

// Save tokenni ttl muddati bilan faylga yozadi (0600 ruxsat).
func Save(path, token string, ttl time.Duration) error {
	tc := TokenCache{
		Token:     token,
		ExpiresAt: time.Now().Add(ttl),
	}
	data, err := json.MarshalIndent(tc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
