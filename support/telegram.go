package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// TelegramAPI - Bot API bazasi.
const TelegramAPI = "https://api.telegram.org"

// TelegramReady - bot tokeni va guruh id'si berilganmi.
func TelegramReady() bool {
	return os.Getenv("TELEGRAM_BOT_TOKEN") != "" && os.Getenv("TELEGRAM_GROUP_ID") != ""
}

// SendTelegram xodimlar guruhiga xabar yuboradi (help kaliti shu yerga ketadi).
func SendTelegram(text string) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_GROUP_ID")
	if token == "" || chatID == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN yoki TELEGRAM_GROUP_ID berilmagan")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("bo'sh xabar")
	}

	body, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", TelegramAPI, token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).
		Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram (status %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil
}
