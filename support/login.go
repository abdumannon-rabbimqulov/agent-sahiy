// Support tizimiga kirish: login, tokenni keshdan olish va yangilash.
//
// withToken — support API'siga so'rov yuborishning yagona yo'li: token
// eskirsa bir marta o'zi yangilab qayta uriniladi.
package support

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
)

// FallbackTTL — token ichidan muddatni aniqlab bo'lmasa shu ishlatiladi.
const FallbackTTL = 30 * time.Minute

// DefaultBaseURL — support serveri (BASE_URL bo'sh bo'lsa).
const DefaultBaseURL = "https://api.market.sahiy.uz"

// LoginPath — support admin login endpointi.
const LoginPath = "/api/v1/admins/login"

// Credentials support saytiga kirish ma'lumotlari.
// LoginField — login maydonining nomi ("login", "username" yoki "phone").
type Credentials struct {
	BaseURL    string
	Login      string
	Password   string
	LoginField string
}

// CredentialsFromEnv .env dagi LOGIN/PASSWORD/LOGIN_FIELD/BASE_URL ni o'qiydi.
func CredentialsFromEnv() Credentials {
	c := Credentials{
		BaseURL:    os.Getenv("BASE_URL"),
		Login:      os.Getenv("LOGIN"),
		Password:   os.Getenv("PASSWORD"),
		LoginField: os.Getenv("LOGIN_FIELD"),
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.LoginField == "" {
		c.LoginField = "login"
	}
	return c
}

// Login serverga POST yuborib yangi token oladi (cache'ga tegmaydi).
func Login(c Credentials) (string, error) {
	if c.Login == "" || c.Password == "" {
		return "", fmt.Errorf("LOGIN yoki PASSWORD berilmagan")
	}
	field := c.LoginField
	if field == "" {
		field = "login"
	}
	body, err := json.Marshal(map[string]string{
		field:      c.Login,
		"password": c.Password,
	})
	if err != nil {
		return "", fmt.Errorf("body marshal: %w", err)
	}

	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+LoginPath, bytes.NewReader(body))
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

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("login muvaffaqiyatsiz (status %d): %s", resp.StatusCode, snippet(raw))
	}

	token, err := extractToken(raw)
	if err != nil {
		return "", fmt.Errorf("%w\nXom javob: %s", err, string(raw))
	}
	return token, nil
}

// Token avval cache'ni tekshiradi, bo'lmasa login qilib cache'ga saqlaydi.
func Token(c Credentials, cachePath string) (string, error) {
	if tc := LoadToken(cachePath); tc != nil {
		return tc.Token, nil
	}
	return refresh(c, cachePath)
}

// Refresh cache'ni o'chirib, yangi token oladi va saqlaydi.
// Token 401 bilan rad etilganda chaqiriladi.
func Refresh(c Credentials, cachePath string) (string, error) {
	DropToken(cachePath)
	return refresh(c, cachePath)
}

func refresh(c Credentials, cachePath string) (string, error) {
	token, err := Login(c)
	if err != nil {
		return "", err
	}
	if err := SaveToken(cachePath, token, tokenTTL(token)); err != nil {
		fmt.Printf("ogohlantirish: token cache'ga saqlanmadi: %v\n", err)
	}
	return token, nil
}

// extractToken javobdan tokenni moslashuvchan izlaydi.
// Real API: {"data": {"token": "..."}}.
func extractToken(body []byte) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("javobni JSON sifatida o'qib bo'lmadi: %w", err)
	}

	keys := []string{"token", "access_token", "accessToken", "jwt"}
	if data, ok := m["data"].(map[string]any); ok {
		for _, k := range keys {
			if v, ok := data[k].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("javobdan token topilmadi (data.token/token/access_token tekshirildi)")
}

// tokenTTL JWT ichidagi "exp" ga qarab muddatni hisoblaydi (60s zaxira bilan).
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
	d := time.Until(time.Unix(claims.Exp, 0)) - 60*time.Second
	if d <= 0 {
		return FallbackTTL
	}
	return d
}

// withToken - support tizimiga so'rov yuborishning yagona yo'li: token
// keshidan olinadi, ErrUnauthorized qaytsa bir marta yangilab qayta
// uriniladi. Shu bilan har chaqiruvda takrorlanadigan login/refresh
// bloki bitta joyda turadi.
func withToken[T any](fn func(baseURL, token string) (T, error)) (T, error) {
	var zero T
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return zero, fmt.Errorf("support login: %w", err)
	}
	out, err := fn(creds.BaseURL, token)
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			out, err = fn(creds.BaseURL, token)
		}
	}
	return out, err
}

// withTokenErr - withToken'ning natija qaytarmaydigan ko'rinishi.
func withTokenErr(fn func(baseURL, token string) error) error {
	_, err := withToken(func(baseURL, token string) (struct{}, error) {
		return struct{}{}, fn(baseURL, token)
	})
	return err
}
