package support

import (
	"fmt"
	"net/http"

	"sahiy-agent/internal/client"
)

// sendPath — /api/v2 ostida (login v1'da bo'lsa ham, bu endpoint v2).
const sendPath = "/api/v2/chat/send"

// SendRequest — /api/v2/chat/send POST body.
type SendRequest struct {
	SenderID       int64  `json:"sender_id"`
	Role           string `json:"role"` // "agent" | "client" | "seller"
	ConversationID int64  `json:"conversation_id"`
	Text           string `json:"text"`
	Content        string `json:"content"`       // odatda "text"
	SupportField   int    `json:"support_field"` // odatda 0
}

// SendMessage suhbatga xabar yuboradi.
func SendMessage(c *client.Client, senderID, conversationID int64, role, text string) ([]byte, error) {
	req := SendRequest{
		SenderID:       senderID,
		Role:           role,
		ConversationID: conversationID,
		Text:           text,
		Content:        "text",
		SupportField:   0,
	}
	body, status, err := c.Do(http.MethodPost, sendPath, req)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("xabar yuborish muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return body, nil
}
