package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// SendPath - suhbatga xabar yuborish (login v1'da bo'lsa ham bu v2).
const SendPath = "/api/v2/chat/send"

// ResolutionPath - suhbatni "hal bo'ldi" deb belgilash.
const ResolutionPath = "/api/v1/support.chat.conversation/resolution/"

// SendRequest - /api/v2/chat/send body.
type SendRequest struct {
	SenderID       int64  `json:"sender_id"`
	Role           string `json:"role"` // "agent" | "client" | "seller"
	ConversationID int64  `json:"conversation_id"`
	Text           string `json:"text"`
	Content        string `json:"content"`       // "text"
	SupportField   int    `json:"support_field"` // 0
}

// AgentSenderID - agent qaysi id bilan yozadi (.env: AGENT_SENDER_ID).
func AgentSenderID() int64 {
	n, _ := strconv.ParseInt(os.Getenv("AGENT_SENDER_ID"), 10, 64)
	return n
}

// SendMessage suhbatga agent nomidan xabar yuboradi.
func SendMessage(baseURL, token string, senderID, conversationID int64, text string) error {
	if senderID <= 0 {
		return fmt.Errorf("AGENT_SENDER_ID berilmagan — .env ga agent yozadigan " +
			"support akkaunt id'sini qo'ying (login javobidagi admin.id) va API'ni qayta ishga tushiring")
	}
	if conversationID <= 0 {
		return fmt.Errorf("conversation_id berilmagan")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("bo'sh xabar")
	}

	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	body, err := json.Marshal(SendRequest{
		SenderID:       senderID,
		Role:           "agent",
		ConversationID: conversationID,
		Text:           text,
		Content:        "text",
		SupportField:   0,
	})
	if err != nil {
		return fmt.Errorf("so'rov yasash: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+SendPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("so'rov yaratish: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("xabar yuborish: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("xabar yuborilmadi (status %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil
}

// ResolveConversation suhbatni yopadi (state: 1 — hal bo'ldi).
func ResolveConversation(baseURL, token string, conversationID int64, state int, comment string) error {
	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	body, err := json.Marshal(map[string]any{"state": state, "comment": comment})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s%s%d", strings.TrimRight(base, "/"), ResolutionPath, conversationID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resolution (status %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil
}

// SendToClient token keshidan foydalanib xabar yuboradi; token eskirgan
// bo'lsa bir marta yangilab qayta uriniladi.
func SendToClient(conversationID int64, text string) error {
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return fmt.Errorf("support login: %w", err)
	}
	err = SendMessage(creds.BaseURL, token, AgentSenderID(), conversationID, text)
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			err = SendMessage(creds.BaseURL, token, AgentSenderID(), conversationID, text)
		}
	}
	return err
}
