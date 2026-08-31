package support

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// MessagesPath — suhbat ichidagi xabarlar. Bu endpoint GET qabul qiladi.
const MessagesPath = "/api/v1/support.chat.message/conversation/"

// DefaultMessageLimit — modelga/javobga ketadigan oxirgi xabarlar soni.
const DefaultMessageLimit = 10

// Message — xabardan olinadigan maydonlar. Agent ham, client ham shu shaklda.
type Message struct {
	ID         int64  `json:"id"`
	SenderID   int64  `json:"sender_id"`
	Message    string `json:"message"`
	SenderType string `json:"sender_type"`
	CreatedAt  string `json:"created_at"`
}

// FromClient - xabarni mijoz yozganmi (agent/xodim emas).
func (m Message) FromClient() bool { return m.SenderType == "client" }

// FetchMessages suhbatning oxirgi `limit` ta xabarini eskisidan yangisiga
// saralab qaytaradi. Server yangisini birinchi qilib beradi, shuning uchun
// id bo'yicha qayta saralanadi.
func FetchMessages(baseURL, token string, conversationID int64, limit int) ([]Message, error) {
	if conversationID <= 0 {
		return nil, fmt.Errorf("conversation_id berilmagan")
	}
	if limit < 1 {
		limit = DefaultMessageLimit
	}
	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := fmt.Sprintf("%s%s%d?page=1&limit=%d", strings.TrimRight(base, "/"), MessagesPath, conversationID, limit)

	newReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	status, raw, err := doWithRetry(client, newReq, Retries())
	if err != nil {
		return nil, fmt.Errorf("so'rov yuborish: %w", err)
	}
	if status == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("xabarlar (status %d): %s", status, snippet(raw))
	}

	var out struct {
		Data []struct {
			ID         int64  `json:"id"`
			SenderID   int64  `json:"sender_id"`
			Message    string `json:"message"`
			SenderType string `json:"sender_type"`
			CreatedAt  string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}

	rows := out.Data
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	if len(rows) > limit {
		rows = rows[len(rows)-limit:] // eng oxirgilari
	}

	msgs := make([]Message, 0, len(rows))
	for _, r := range rows {
		msgs = append(msgs, Message{
			ID:         r.ID,
			SenderID:   r.SenderID,
			Message:    r.Message,
			SenderType: r.SenderType,
			CreatedAt:  r.CreatedAt,
		})
	}
	return msgs, nil
}

// MessagesJSON xabarlarni tayyor JSON matn qilib qaytaradi:
// {"conversation_id": N, "count": M, "messages": [{"message":..,"sender_type":..,"created_at":..}]}
func MessagesJSON(baseURL, token string, conversationID int64, limit int) ([]byte, error) {
	msgs, err := FetchMessages(baseURL, token, conversationID, limit)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		ConversationID int64     `json:"conversation_id"`
		Count          int       `json:"count"`
		Messages       []Message `json:"messages"`
	}{conversationID, len(msgs), msgs}, "", "  ")
}
