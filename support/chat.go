package support

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatsPath — suhbatlar ro'yxati. Server buni GET bilan bermaydi (405),
// faqat POST qabul qiladi — filtr body'da ketadi, ma'lumot esa o'zgarmaydi.
const ChatsPath = "/api/v1/support.chat.conversation/filter"

// ErrUnauthorized token rad etilganda qaytadi — chaqiruvchi Refresh qiladi.
var ErrUnauthorized = errors.New("token rad etildi (401)")

// Chat — suhbatdan olinadigan maydonlar. Qolgani tashlanadi.
type Chat struct {
	ID          int64  `json:"id"`
	ClientID    int64  `json:"client_id"`
	CreatedAt   string `json:"created_at"`
	MsCreatedAt string `json:"ms_created_at"`
}

// ChatFilter — so'rov shartlari. ClientID 0 bo'lsa hamma suhbatlar keladi,
// noldan katta bo'lsa faqat o'sha mijoz bilan bog'liq suhbatlar.
type ChatFilter struct {
	ClientID int64 `json:"client_id"`
	Page     int   `json:"page"`
	Limit    int   `json:"limit"`
}

// FetchChats suhbatlarni oladi. Server GET qabul qilmaydi (405), shuning uchun
// so'rov POST bilan ketadi — filtr body'da, page/limit query'da.
func FetchChats(baseURL, token string, f ChatFilter) ([]Chat, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 10
	}
	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := fmt.Sprintf("%s%s?page=%d&limit=%d", strings.TrimRight(base, "/"), ChatsPath, f.Page, f.Limit)

	filter := map[string]any{
		"type":  "client",
		"state": []int{1, 2, 3},
	}
	if f.ClientID > 0 {
		filter["client_id"] = f.ClientID
	}
	body, err := json.Marshal(filter)
	if err != nil {
		return nil, fmt.Errorf("body marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("so'rov yaratish: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("so'rov yuborish: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("suhbatlar ro'yxati (status %d): %s", resp.StatusCode, string(raw))
	}

	var out struct {
		Data struct {
			Chats []Chat `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}
	return out.Data.Chats, nil
}

// ChatsJSON suhbatlarni tayyor JSON matn qilib qaytaradi:
// {"count": N, "chats": [{"id":..,"client_id":..,"created_at":..,"ms_created_at":..}]}
func ChatsJSON(baseURL, token string, f ChatFilter) ([]byte, error) {
	chats, err := FetchChats(baseURL, token, f)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Count int    `json:"count"`
		Chats []Chat `json:"chats"`
	}{len(chats), chats}, "", "  ")
}
