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
	UpdateID int64        `json:"update_id"`
	Message  *tgMsgUpdate `json:"message"`
}

type tgMsgUpdate struct {
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

// handleTelegramReply - bitta update. Bot xabariga reply bo'lsa, o'sha
// xabar qaysi turga tegishliligi aniqlanadi:
//
//   - "⚠️ Muammoli buyurtma(lar)" / "🔁 Hali hal bo'lmagan" — ochiq
//     muammo(lar) yopiladi va mijozga javob tayyorlanadi;
//   - "🆘 Yordam kerak" — yopiladigan buyurtma yo'q, mijozga javob
//     xuddi shu yo'l bilan tayyorlanadi.
func handleTelegramReply(u tgUpdate) {
	m := u.Message
	if m == nil || m.ReplyTo == nil || strings.TrimSpace(m.Text) == "" {
		return
	}

	who := strings.TrimSpace(m.From.FirstName + " " + m.From.LastName)
	if m.From.Username != "" {
		who = "@" + m.From.Username
	}
	if who == "" {
		who = "xodim"
	}

	// Bitta xabarda bir mijozning bir necha buyurtmasi bo'lishi mumkin —
	// reply ularning HAMMASINI yopadi.
	var issues []OrderIssue
	err := DB.Where("tg_message_id = ? AND state = ?", m.ReplyTo.MessageID, IssueOpen).
		Order("id asc").Find(&issues).Error
	if err == nil && len(issues) > 0 {
		handleIssueReply(m, issues, who)
		return
	}

	// "🆘 Yordam kerak" xabariga reply.
	var in Interaction
	if err := DB.Where("help_message_id = ?", m.ReplyTo.MessageID).
		Order("id desc").First(&in).Error; err == nil {
		handleHelpReply(m, &in, who)
	}
}

// staffReplyStatus - xodimga guruhda qaytariladigan qisqa holat.
//
// Eng muhimi ikkinchi holat: model javobni qayta yoza olmagan bo'lsa,
// xodimning XOM matni mijozga yuborilmaydi (u ichki tilda yozilgan).
// Xodim buni bilib tursin — javob "ketdi" deb o'ylab qolmasin.
func staffReplyStatus(in *Interaction) string {
	switch {
	case in.Status == StatusSent:
		return "mijozga yuborildi"
	case in.Status == StatusPending && in.Error != "":
		return "⚠️ AI javobni mijoz tiliga o'gira olmadi (" + in.Error + ").\n" +
			"Xom matn mijozga YUBORILMADI — panelda tahrirlab yuborish kerak."
	case in.Status == StatusPending:
		return "mijozga javob tayyor — admin tasdig'i kutilmoqda"
	default:
		return "mijozga javob tayyorlandi, lekin yuborilmadi: " + in.Error
	}
}

// handleHelpReply - guruhdagi "yordam kerak" xabariga reply: xodim
// javobidan mijozga xabar tayyorlanadi. Muammoli buyurtma yo'li bilan
// bir xil ishlaydi, faqat yopiladigan buyurtma yozuvi yo'q.
func handleHelpReply(m *tgMsgUpdate, src *Interaction, who string) {
	log.Printf("telegram: suhbat %d — yordam so'roviga %s javob berdi", src.ConversationID, who)

	// Shu suhbat bo'yicha tasdiqlanmagan AI qoralamasi bo'lsa navbatdan
	// chiqariladi: javobni endi xodim berdi, eski qoralama keyinroq
	// tasodifan yuborilib, mijoz ikki xil javob olmasin.
	supersedePending(src.ConversationID)

	holat := "mijozga javob tayyorlanmadi"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if in, err := AnswerFromStaffHelp(ctx, src, m.Text, who); err != nil {
		log.Printf("telegram: suhbat %d — mijozga javob tayyorlanmadi: %v", src.ConversationID, err)
	} else {
		holat = staffReplyStatus(in)
	}

	if _, err := SendTelegramMessage(
		fmt.Sprintf("✅ Javob qabul qilindi (%s).\n%s", who, holat), m.MessageID); err != nil {
		log.Printf("telegram: tasdiq yuborilmadi: %v", err)
	}
}

// handleIssueReply - guruhdagi "muammoli buyurtma" xabariga reply: muammo
// hal qilindi deb belgilanadi va mijozga javob tayyorlanadi.
func handleIssueReply(m *tgMsgUpdate, issues []OrderIssue, who string) {
	var closed []string
	for i := range issues {
		if err := ResolveIssue(DB, &issues[i], strings.TrimSpace(m.Text), who, ResolvedViaTelegram); err != nil {
			log.Printf("telegram: muammoni yopib bo'lmadi (%s): %v", issues[i].OrderSN, err)
			continue
		}
		closed = append(closed, issues[i].OrderSN)
	}
	if len(closed) == 0 {
		return
	}
	sns := strings.Join(closed, ", ")
	log.Printf("telegram: %s muammosi %s tomonidan hal qilindi", sns, who)

	// Xodim javobidan mijozga xabar tayyorlanadi: LLM uni mijoz tiliga
	// moslab yozadi, so'ng odatdagi qoida bo'yicha ketadi (avto-javob
	// yoqiq bo'lsa darhol, aks holda tasdiqlash navbatiga).
	holat := "mijozga javob tayyorlanmadi"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Buyurtmalar bitta mijozniki — mijozga ham bitta javob tayyorlanadi.
	if in, err := AnswerFromStaffReply(ctx, issues, m.Text, who); err != nil {
		log.Printf("telegram: mijozga javob tayyorlanmadi (%s): %v", sns, err)
	} else {
		holat = staffReplyStatus(in)
	}

	// Xodimga qisqa tasdiq — javobi hisobga olingani ko'rinsin.
	if _, err := SendTelegramMessage(
		fmt.Sprintf("✅ %s — hal qilindi deb belgilandi (%s).\n%s", sns, who, holat),
		m.MessageID); err != nil {
		log.Printf("telegram: tasdiq yuborilmadi: %v", err)
	}
}
