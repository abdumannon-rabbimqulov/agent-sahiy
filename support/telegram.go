// Xodimlar guruhiga (Telegram bot) xabar yuborish: help kaliti va
// muammoli buyurtmalar haqidagi xabarlar shu yerdan ketadi.
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

// DefaultTelegramAPI - Bot API bazasi.
const DefaultTelegramAPI = "https://api.telegram.org"

// TelegramAPI - .env dagi TELEGRAM_API_URL (sinov uchun almashtiriladi).
func TelegramAPI() string { return envStr("TELEGRAM_API_URL", DefaultTelegramAPI) }

// SendTelegram xodimlar guruhiga xabar yuboradi (help kaliti shu yerga ketadi).
func SendTelegram(text string) error {
	_, err := SendTelegramMessage(text, 0)
	return err
}

// SendTelegramMessage guruhga xabar yuboradi va Telegram bergan message_id ni
// qaytaradi. Muammoli buyurtma xabarlarida shu id saqlanadi: xodim o'sha
// xabarga reply qilsa, javob yechim sifatida yoziladi.
//
// replyTo > 0 bo'lsa xabar o'sha xabarga javob bo'lib chiqadi.
func SendTelegramMessage(text string, replyTo int64) (int64, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_GROUP_ID")
	if token == "" || chatID == "" {
		return 0, fmt.Errorf("TELEGRAM_BOT_TOKEN yoki TELEGRAM_GROUP_ID berilmagan")
	}
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("bo'sh xabar")
	}

	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if replyTo > 0 {
		payload["reply_to_message_id"] = replyTo
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", TelegramAPI(), token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).
		Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("telegram (status %d): %s", resp.StatusCode, snippet(raw))
	}

	var out struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, nil // xabar ketdi, faqat id o'qilmadi
	}
	return out.Result.MessageID, nil
}

// SendTelegramPhoto guruhga rasmni havola orqali yuboradi (Telegram
// o'zi havoladan yuklab oladi — fayl bu yerga tushirilmaydi). caption
// bo'sh bo'lishi mumkin.
func SendTelegramPhoto(photoURL, caption string) error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_GROUP_ID")
	if token == "" || chatID == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN yoki TELEGRAM_GROUP_ID berilmagan")
	}
	if strings.TrimSpace(photoURL) == "" {
		return fmt.Errorf("bo'sh rasm havolasi")
	}

	payload := map[string]any{
		"chat_id": chatID,
		"photo":   photoURL,
	}
	if caption != "" {
		payload["caption"] = caption
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendPhoto", TelegramAPI(), token)
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
