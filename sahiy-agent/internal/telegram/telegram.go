package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"sahiy-agent/internal/tgtext"
)

// Bot — Telegram Bot API client (tashqi kutubxonasiz).
// Eslatma: guruh javoblarini o'qish uchun bot guruhda bo'lishi va
// privacy mode o'chirilgan bo'lishi kerak. Muqobil: MTProto userbot
// (gotd/td) — TelegramSender interfeysiga ulanadi.
type Bot struct {
	token string
	http  *http.Client
}

// New yangi Bot.
func New(token string) *Bot {
	return &Bot{token: token, http: &http.Client{Timeout: 65 * time.Second}}
}

func (b *Bot) api(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method)
}

// SendMessage guruhga/chatga xabar yuboradi va message_id qaytaradi.
// code — monospace bo'laklar (Telegram'da bosilganda nusxalanadi).
func (b *Bot) SendMessage(chatID, text string, code []tgtext.Span) (int64, error) {
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	if ents := codeEntities(code); ents != "" {
		form.Set("entities", ents)
	}

	resp, err := b.http.PostForm(b.api("sendMessage"), form)
	if err != nil {
		return 0, fmt.Errorf("telegram sendMessage: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, fmt.Errorf("javobni o'qish: %w\nXom: %s", err, string(body))
	}
	if !r.OK {
		return 0, fmt.Errorf("telegram xatosi: %s", r.Description)
	}
	return r.Result.MessageID, nil
}

// Update — getUpdates natijasidagi bitta yangilanish (kerakli qismi).
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		From      struct {
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		ReplyTo *struct {
			MessageID int64 `json:"message_id"`
		} `json:"reply_to_message"`
	} `json:"message"`
}

// GetUpdates offset'dan boshlab yangilanishlarni oladi (long-polling).
func (b *Bot) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	form := url.Values{}
	form.Set("offset", strconv.FormatInt(offset, 10))
	form.Set("timeout", strconv.Itoa(timeoutSec))

	resp, err := b.http.PostForm(b.api("getUpdates"), form)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r struct {
		OK          bool     `json:"ok"`
		Result      []Update `json:"result"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w\nXom: %s", err, string(body))
	}
	if !r.OK {
		return nil, fmt.Errorf("telegram xatosi: %s", r.Description)
	}
	return r.Result, nil
}

// codeEntities Span'larni Bot API'ning entities JSON'iga aylantiradi.
// Bo'sh bo'lsa "" qaytaradi.
func codeEntities(code []tgtext.Span) string {
	if len(code) == 0 {
		return ""
	}
	type entity struct {
		Type   string `json:"type"`
		Offset int    `json:"offset"`
		Length int    `json:"length"`
	}
	list := make([]entity, 0, len(code))
	for _, s := range code {
		list = append(list, entity{Type: "code", Offset: s.Offset, Length: s.Length})
	}
	data, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(data)
}
