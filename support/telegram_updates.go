// Telegram guruhidan javoblarni o'qish: xodim bot xabariga REPLY qilsa,
// o'sha matn muammoning yechimi sifatida saqlanadi.
package support

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// SettingTgOffset - getUpdates uchun oxirgi o'qilgan update_id.
// Restartdan keyin eski xabarlar qayta o'qilmasin.
const SettingTgOffset = "tg_update_offset"

// DefaultTgPollSec - guruhni tekshirish oralig'i.
const DefaultTgPollSec = 30

// tgUpdate - getUpdates javobidan kerakli maydonlar.
type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Date      int64  `json:"date"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"from"`
		ReplyTo *struct {
			MessageID int64 `json:"message_id"`
		} `json:"reply_to_message"`
	} `json:"message"`
}

// StartTelegramPoller guruhdagi javoblarni fon rejimida kuzatadi.
//
// Bu sikl `agent_enabled` o'chirilganda ham ishlaydi: xodim yozgan yechim
// yo'qolmasligi kerak.
func StartTelegramPoller(ctx context.Context) {
	if os.Getenv("TELEGRAM_BOT_TOKEN") == "" {
		log.Println("telegram: bot tokeni yo'q — guruh javoblari o'qilmaydi")
		return
	}
	interval := time.Duration(envInt("TG_POLL_SEC", DefaultTgPollSec)) * time.Second
	log.Printf("telegram: guruh javoblari har %s da tekshiriladi", interval)

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := PollTelegramReplies(); err != nil {
					log.Printf("telegram: %v", err)
				}
			}
		}
	}()
}

// PollTelegramReplies bir marta getUpdates qiladi va reply'larni qayta ishlaydi.
func PollTelegramReplies() error {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil
	}

	offset, _ := strconv.ParseInt(GetSetting(SettingTgOffset, "0"), 10, 64)
	url := fmt.Sprintf("%s/bot%s/getUpdates?timeout=0&allowed_updates=%%5B%%22message%%22%%5D",
		TelegramAPI(), token)
	if offset > 0 {
		url += "&offset=" + strconv.FormatInt(offset, 10)
	}

	resp, err := (&http.Client{Timeout: 25 * time.Second}).Get(url)
	if err != nil {
		return fmt.Errorf("getUpdates: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("getUpdates (status %d): %s", resp.StatusCode, snippet(raw))
	}

	var out struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("getUpdates javobi: %w", err)
	}

	var last int64
	for _, u := range out.Result {
		if u.UpdateID > last {
			last = u.UpdateID
		}
		handleTelegramReply(u)
	}

	// Keyingi safar shu update'lar qayta kelmasin.
	if last > 0 {
		SetSetting(DB, SettingTgOffset, strconv.FormatInt(last+1, 10))
	}
	return nil
}

// handleTelegramReply - bitta update. Bot xabariga reply bo'lsa va o'sha
// xabar ochiq muammoga tegishli bo'lsa, muammo yopiladi.
func handleTelegramReply(u tgUpdate) {
	m := u.Message
	if m == nil || m.ReplyTo == nil || strings.TrimSpace(m.Text) == "" {
		return
	}

	var is OrderIssue
	err := DB.Where("tg_message_id = ? AND state = ?", m.ReplyTo.MessageID, IssueOpen).
		First(&is).Error
	if err != nil {
		return // bu reply muammoga tegishli emas
	}

	who := strings.TrimSpace(m.From.FirstName + " " + m.From.LastName)
	if m.From.Username != "" {
		who = "@" + m.From.Username
	}
	if who == "" {
		who = "xodim"
	}

	if err := ResolveIssue(DB, &is, strings.TrimSpace(m.Text), who, ResolvedViaTelegram); err != nil {
		log.Printf("telegram: muammoni yopib bo'lmadi (%s): %v", is.OrderSN, err)
		return
	}
	log.Printf("telegram: %s muammosi %s tomonidan hal qilindi", is.OrderSN, who)

	// Xodim javobidan mijozga xabar tayyorlanadi: LLM uni mijoz tiliga
	// moslab yozadi, so'ng odatdagi qoida bo'yicha ketadi (avto-javob
	// yoqiq bo'lsa darhol, aks holda tasdiqlash navbatiga).
	holat := "mijozga javob tayyorlanmadi"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if in, err := AnswerFromStaffReply(ctx, &is, m.Text, who); err != nil {
		log.Printf("telegram: mijozga javob tayyorlanmadi (%s): %v", is.OrderSN, err)
	} else {
		switch in.Status {
		case StatusSent:
			holat = "mijozga yuborildi"
		case StatusPending:
			holat = "mijozga javob tayyor — admin tasdig'i kutilmoqda"
		default:
			holat = "mijozga javob tayyorlandi, lekin yuborilmadi: " + in.Error
		}
	}

	// Xodimga qisqa tasdiq — javobi hisobga olingani ko'rinsin.
	if _, err := SendTelegramMessage(
		fmt.Sprintf("✅ %s — hal qilindi deb belgilandi (%s).\n%s", is.OrderSN, who, holat),
		m.MessageID); err != nil {
		log.Printf("telegram: tasdiq yuborilmadi: %v", err)
	}
}
