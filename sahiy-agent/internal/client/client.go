package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client avtorizatsiyalangan (token bilan) qayta ishlatiladigan HTTP client.
type Client struct {
	BaseURL string
	Token   string
	http    *http.Client
}

// New yangi Client yaratadi.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Do umumiy so'rov metodi — barcha endpointlar shu orqali chaqiriladi.
// body nil bo'lishi mumkin. Javob body baytlari va status qaytaradi.
func (c *Client) Do(method, path string, body interface{}) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("body marshal: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("so'rov yaratish: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://audit.sahiy.uz")
	req.Header.Set("Referer", "https://audit.sahiy.uz/")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
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

// Get yordamchi.
func (c *Client) Get(path string) ([]byte, int, error) {
	return c.Do(http.MethodGet, path, nil)
}

// Post yordamchi.
func (c *Client) Post(path string, body interface{}) ([]byte, int, error) {
	return c.Do(http.MethodPost, path, body)
}
