package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client avtorizatsiyalangan (token bilan) qayta ishlatiladigan HTTP client.
type Client struct {
	BaseURL string
	// Refresh — token 401 bilan rad etilganda yangi token oladi.
	// nil bo'lsa qayta urinilmaydi.
	Refresh func() (string, error)

	http  *http.Client
	mu    sync.Mutex
	token string
}

// New yangi Client yaratadi.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Token joriy tokenni qaytaradi.
func (c *Client) Token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// Do umumiy so'rov metodi — barcha endpointlar shu orqali chaqiriladi.
// body nil bo'lishi mumkin. Javob body baytlari va status qaytaradi.
func (c *Client) Do(method, path string, body interface{}) ([]byte, int, error) {
	var raw []byte
	contentType := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("body marshal: %w", err)
		}
		raw, contentType = data, "application/json"
	}
	return c.send(method, path, contentType, raw)
}

// Get yordamchi.
func (c *Client) Get(path string) ([]byte, int, error) {
	return c.Do(http.MethodGet, path, nil)
}

// Post yordamchi.
func (c *Client) Post(path string, body interface{}) ([]byte, int, error) {
	return c.Do(http.MethodPost, path, body)
}

// DoRaw tayyor body va Content-Type bilan so'rov yuboradi
// (masalan multipart/form-data fayl yuklash uchun).
func (c *Client) DoRaw(method, path, contentType string, body io.Reader) ([]byte, int, error) {
	var raw []byte
	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, 0, fmt.Errorf("body o'qish: %w", err)
		}
		raw = data
	}
	return c.send(method, path, contentType, raw)
}

// send so'rovni yuboradi; 401 kelsa tokenni yangilab bir marta qayta uradi.
func (c *Client) send(method, path, contentType string, body []byte) ([]byte, int, error) {
	respBody, status, err := c.once(method, path, contentType, body)
	if err != nil || status != http.StatusUnauthorized || c.Refresh == nil {
		return respBody, status, err
	}

	// Token eskirgan/bekor qilingan — yangisini olib bir marta qayta urinamiz.
	token, rerr := c.Refresh()
	if rerr != nil {
		return respBody, status, fmt.Errorf("token yangilash: %w", rerr)
	}
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()

	return c.once(method, path, contentType, body)
}

func (c *Client) once(method, path, contentType string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("so'rov yaratish: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://audit.sahiy.uz")
	req.Header.Set("Referer", "https://audit.sahiy.uz/")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if t := c.Token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("so'rov yuborish: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("javobni o'qish: %w", err)
	}
	return respBody, resp.StatusCode, nil
}
