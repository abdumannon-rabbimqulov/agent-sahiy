// Package service — ikkinchi sayt (api.sahiy.uz service API) bilan ishlaydi.
//
// Bu yerda xodim (service user) hisobi orqali kiriladi va buyurtma holati
// id yoki track raqami bo'yicha qidiriladi. Token diskda keshlanadi —
// har restartda qayta login qilinmaydi.
package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LoginRequest — POST /api/v2/service/user/login body.
// Device maydonlari majburiy: ularsiz server 500 qaytaradi.
type LoginRequest struct {
	Phone      string `json:"phone"`
	Password   string `json:"password"`
	APKType    int    `json:"apk_type"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceType string `json:"device_type"`
	FcmToken   string `json:"fcm_token"`
}

// loginResponse — serverdan keladigan javobning kerakli qismi.
type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // soniyalarda
	ServiceUser  struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"service_user"`
	BranchName string `json:"branch_name"`
	Error      string `json:"error"`
	Message    string `json:"message"`
}

// cachedToken — diskda saqlanadigan token.
type cachedToken struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

// Client — service API client.
type Client struct {
	BaseURL   string
	Login     LoginRequest
	CachePath string

	http  *http.Client
	mu    sync.Mutex
	token string
	exp   time.Time
}

// New yangi client. baseURL bo'sh bo'lsa https://api.sahiy.uz ishlatiladi.
func New(baseURL string, login LoginRequest, cachePath string) *Client {
	if baseURL == "" {
		baseURL = "https://api.sahiy.uz"
	}
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Login:     login,
		CachePath: cachePath,
		http:      &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled — login ma'lumotlari to'liqmi.
func (c *Client) Enabled() bool {
	return c != nil && c.Login.Phone != "" && c.Login.Password != ""
}

// Token amaldagi tokenni qaytaradi (kerak bo'lsa login qiladi).
func (c *Client) Token() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.exp) {
		return c.token, nil
	}
	if t, ok := c.readCache(); ok {
		c.token, c.exp = t.Token, t.Expires
		return c.token, nil
	}
	return c.doLogin()
}

// doLogin — mu ushlab turilgan holda chaqiriladi.
func (c *Client) doLogin() (string, error) {
	body, status, err := c.raw(http.MethodPost, "/api/v2/service/user/login", c.Login, "")
	if err != nil {
		return "", err
	}
	var lr loginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return "", fmt.Errorf("login javobini o'qish (status %d): %w", status, err)
	}
	if lr.AccessToken == "" {
		msg := lr.Error
		if msg == "" {
			msg = lr.Message
		}
		return "", fmt.Errorf("service login muvaffaqiyatsiz (status %d): %s", status, msg)
	}

	ttl := time.Duration(lr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	// Muddat tugashidan biroz oldin yangilaymiz.
	c.token = lr.AccessToken
	c.exp = time.Now().Add(ttl - 5*time.Minute)
	c.writeCache(cachedToken{Token: c.token, Expires: c.exp})

	fmt.Printf("✓ Service API: %s (%s)\n", lr.ServiceUser.Name, lr.BranchName)
	return c.token, nil
}

// Get avtorizatsiyalangan GET; 401/403 da bir marta qayta login qiladi.
func (c *Client) Get(path string) ([]byte, int, error) {
	return c.do(http.MethodGet, path, nil)
}

// Post avtorizatsiyalangan POST; 401/403 da bir marta qayta login qiladi.
func (c *Client) Post(path string, body interface{}) ([]byte, int, error) {
	return c.do(http.MethodPost, path, body)
}

func (c *Client) do(method, path string, body interface{}) ([]byte, int, error) {
	token, err := c.Token()
	if err != nil {
		return nil, 0, err
	}
	respBody, status, err := c.raw(method, path, body, token)
	if err != nil || (status != http.StatusUnauthorized && status != http.StatusForbidden) {
		return respBody, status, err
	}

	// Token eskirgan — keshni tashlab qayta login qilamiz.
	c.mu.Lock()
	c.token, c.exp = "", time.Time{}
	_ = os.Remove(c.CachePath)
	token, err = c.doLogin()
	c.mu.Unlock()
	if err != nil {
		return respBody, status, err
	}
	return c.raw(method, path, body, token)
}

// raw — bitta HTTP so'rov (qayta urinishsiz).
func (c *Client) raw(method, path string, body interface{}, token string) ([]byte, int, error) {
	var reader *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("body marshal: %w", err)
		}
		reader = strings.NewReader(string(data))
	}

	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequest(method, c.BaseURL+path, reader)
	} else {
		req, err = http.NewRequest(method, c.BaseURL+path, nil)
	}
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Language", "uz_UZ")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("service so'rov: %w", err)
	}
	defer resp.Body.Close()

	data, err := readAll(resp)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// --- token keshi ---

func (c *Client) readCache() (cachedToken, bool) {
	if c.CachePath == "" {
		return cachedToken{}, false
	}
	data, err := os.ReadFile(c.CachePath)
	if err != nil {
		return cachedToken{}, false
	}
	var t cachedToken
	if err := json.Unmarshal(data, &t); err != nil || t.Token == "" {
		return cachedToken{}, false
	}
	if time.Now().After(t.Expires) {
		return cachedToken{}, false
	}
	return t, true
}

func (c *Client) writeCache(t cachedToken) {
	if c.CachePath == "" {
		return
	}
	data, err := json.Marshal(t)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.CachePath), 0o755)
	_ = os.WriteFile(c.CachePath, data, 0o600) // token — maxfiy
}
