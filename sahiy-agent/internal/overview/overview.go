package overview

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Bu endpoint BOSHQA hostda: api.sahiy.uz (market emas).
// Headers: Accept, Content-Type, Language, x-uuid. Hozircha Network Error
// berayotgan bo'lishi mumkin (CORS/backend) — kod tayyor turadi.
const defaultBaseURL = "https://api.sahiy.uz"

// Request — POST /api/client/user/overview body.
type Request struct {
	UserID           int64  `json:"user_id"`
	OrderPage        int    `json:"order_page"`
	OrderSize        int    `json:"order_size"`
	OrderStatus      *int   `json:"order_status"`     // null bo'lishi mumkin
	OrderSubStatus   *int   `json:"order_sub_status"` // null bo'lishi mumkin
	OrderKeyword     string `json:"order_keyword"`
	OrderCreatedFrom string `json:"order_created_from"` // "2024-01-01"
	OrderCreatedTo   string `json:"order_created_to"`   // "2026-08-11"
}

// Client user overview endpoint bilan ishlaydi.
type Client struct {
	BaseURL string
	Token   string // ixtiyoriy — kerak bo'lsa Bearer qo'yiladi
	http    *http.Client
}

// New yangi client. baseURL bo'sh bo'lsa api.sahiy.uz ishlatiladi.
func New(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// DefaultRequest oqilona standart qiymatlar bilan so'rov tayyorlaydi
// (oxirgi order_created_to = bugun).
func DefaultRequest(userID int64) Request {
	return Request{
		UserID:           userID,
		OrderPage:        1,
		OrderSize:        10,
		OrderKeyword:     "",
		OrderCreatedFrom: "2024-01-01",
		OrderCreatedTo:   time.Now().Format("2006-01-02"),
	}
}

// Fetch foydalanuvchi overview'sini oladi. Serverning xom javobini qaytaradi.
func (c *Client) Fetch(req Request) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/client/user/overview", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Language", "uz_UZ")
	httpReq.Header.Set("x-uuid", randomUUID())
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("overview so'rov: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, fmt.Errorf("overview muvaffaqiyatsiz (status %d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// randomUUID oddiy v4-ga o'xshash UUID yasaydi (x-uuid uchun).
func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
