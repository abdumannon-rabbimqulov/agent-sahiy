// Xodimlar guruhiga (Telegram bot) xabar yuborish: muammoli
// buyurtmalar haqidagi xabarlar shu yerdan ketadi.
package support

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// DefaultTelegramAPI - Bot API bazasi.
const DefaultTelegramAPI = "https://api.telegram.org"

// TelegramAPI - .env dagi TELEGRAM_API_URL (sinov uchun almashtiriladi).
func TelegramAPI() string { return envStr("TELEGRAM_API_URL", DefaultTelegramAPI) }

// SendTelegramMessage guruhga xabar yuboradi va Telegram bergan message_id ni
// qaytaradi. Muammoli buyurtma xabarlarida shu id saqlanadi: xodim o'sha
// xabarga reply qilsa, javob yechim sifatida yoziladi.
//
// replyTo > 0 bo'lsa xabar o'sha xabarga javob bo'lib chiqadi.
//
// Xabar RAQAMLANMAYDI: kunlik "#N" faqat muammo xabarlariga qo'yiladi —
// ular SendTelegramIssue orqali ketadi (support/telegram_number.go).
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
	// Yuborish navbat orqali ketadi: tezlik cheklanadi va 429 da
	// qayta uriniladi (support/telegram_rate.go).
	raw, err := telegramPost("sendMessage", payload)
	if err != nil {
		return 0, err
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
func SendTelegramPhoto(photoURL, caption string) (int64, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_GROUP_ID")
	if token == "" || chatID == "" {
		return 0, fmt.Errorf("TELEGRAM_BOT_TOKEN yoki TELEGRAM_GROUP_ID berilmagan")
	}
	if strings.TrimSpace(photoURL) == "" {
		return 0, fmt.Errorf("bo'sh rasm havolasi")
	}

	payload := map[string]any{
		"chat_id": chatID,
		"photo":   photoURL,
	}
	payload["caption"] = caption
	raw, err := telegramPost("sendPhoto", payload)
	if err != nil {
		return 0, err
	}

	var out struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, nil // rasm ketdi, faqat id o'qilmadi
	}
	return out.Result.MessageID, nil
}
