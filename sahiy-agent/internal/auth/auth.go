package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"sahiy-agent/internal/cache"
	"sahiy-agent/internal/config"
)

// FallbackTTL — token ichidan muddatni aniqlab bo'lmasa ishlatiladi.
const FallbackTTL = 30 * time.Minute

// Login serverga POST yuborib token oladi.
// Login maydonining nomi cfg.LoginField orqali sozlanadi
// (masalan "login", "username" yoki "phone").
func Login(cfg *config.Config) (string, error) {
	field := cfg.LoginField
	if field == "" {
		field = "login"
	}
	body, err := json.Marshal(map[string]string{
		field:      cfg.Login,
		"password": cfg.Password,
	})
	if err != nil {
		return "", fmt.Errorf("body marshal: %w", err)
	}

	url := cfg.BaseURL + "/api/v1/admins/login"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("so'rov yaratish: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://audit.sahiy.uz")
	req.Header.Set("Referer", "https://audit.sahiy.uz/")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("so'rov yuborish: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("login muvaffaqiyatsiz (status %d): %s", resp.StatusCode, string(respBody))
	}

	token, err := extractToken(respBody)
	if err != nil {
		return "", fmt.Errorf("%w\nXom javob: %s", err, string(respBody))
	}
	return token, nil
}

// extractToken javobdan tokenni moslashuvchan izlaydi.
// Real API: {"data": {"token": "..."}}. Boshqa variantlar ham qo'llab-quvvatlanadi.
func extractToken(body []byte) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("javobni JSON sifatida o'qib bo'lmadi: %w", err)
	}

	keys := []string{"token", "access_token", "accessToken", "jwt"}

	// data.* ichida qidirish (real API shu yerda qaytaradi)
	if data, ok := m["data"].(map[string]interface{}); ok {
		for _, k := range keys {
			if v, ok := data[k].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	// top-level qidirish
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v, nil
		}
	}

	return "", fmt.Errorf("javobdan token topilmadi (data.token/token/access_token tekshirildi)")
}

// tokenTTL JWT ichidagi "exp" claim'iga qarab qancha vaqt amal qilishini hisoblaydi.
// exp topilmasa FallbackTTL qaytaradi.
func tokenTTL(token string) time.Duration {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return FallbackTTL
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return FallbackTTL
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return FallbackTTL
	}
	// 60s zaxira — muddat tugashiga yaqin qayta login qilinsin.
	d := time.Until(time.Unix(claims.Exp, 0)) - 60*time.Second
	if d <= 0 {
		return FallbackTTL
	}
	return d
}

// GetToken avval cache'ni tekshiradi, bo'lmasa login qilib cache'ga saqlaydi.
func GetToken(cfg *config.Config, cachePath string) (string, error) {
	if tc := cache.Load(cachePath); tc != nil {
		return tc.Token, nil
	}

	token, err := Login(cfg)
	if err != nil {
		return "", err
	}

	if err := cache.Save(cachePath, token, tokenTTL(token)); err != nil {
		fmt.Printf("ogohlantirish: token cache'ga saqlanmadi: %v\n", err)
	}
	return token, nil
}

// Refresh cache'ni bekor qilib, yangi token oladi va saqlaydi.
// Token 401 bilan rad etilganda chaqiriladi.
func Refresh(cfg *config.Config, cachePath string) (string, error) {
	_ = os.Remove(cachePath)

	token, err := Login(cfg)
	if err != nil {
		return "", err
	}
	if err := cache.Save(cachePath, token, tokenTTL(token)); err != nil {
		fmt.Printf("ogohlantirish: token cache'ga saqlanmadi: %v\n", err)
	}
	return token, nil
}

// Subject JWT ichidagi "sub" claim'ini (odatda admin/agent ID) qaytaradi.
// Topilmasa 0 va xato qaytaradi.
func Subject(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("token JWT formatida emas")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("payload dekod: %w", err)
	}
	var claims struct {
		Sub int64 `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, fmt.Errorf("claims o'qish: %w", err)
	}
	if claims.Sub == 0 {
		return 0, fmt.Errorf("token'da sub topilmadi")
	}
	return claims.Sub, nil
}
