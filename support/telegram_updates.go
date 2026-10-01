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
	"regexp"
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
	// Caption - rasm bilan yuborilgan xabarda matn `text` da emas,
	// shu yerda keladi. Ilgari faqat `text` o'qilardi va rasmli javob
	// umuman e'tiborsiz qolardi.
	Caption string `json:"caption"`
	// Photo - Telegram bitta rasmni bir necha o'lchamda beradi,
	// oxirgisi eng kattasi (tgPhoto.Best).
	Photo []struct {
		FileID   string `json:"file_id"`
		FileSize int64  `json:"file_size"`
	} `json:"photo"`
	// Document - rasm "fayl sifatida" yuborilgan holat.
	Document *struct {
		FileID   string `json:"file_id"`
		FileName string `json:"file_name"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
	} `json:"document"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	From struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	} `json:"from"`
	ReplyTo *struct {
		MessageID int64 `json:"message_id"`
		// Text, Caption - javob berilayotgan xabarning o'z matni.
		// Bazada yozuv topilmaganda (eski xabarlar) suhbat shundan
		// aniqlanadi: sarlavhada "Suhbat: #<id>" turadi.
		Text    string `json:"text"`
		Caption string `json:"caption"`
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
	if m == nil || m.ReplyTo == nil {
		return
	}
	// Rasmli javobda matn `caption` da keladi. Matn ham, rasm ham
	// bo'lmasa — javob emas (masalan sticker), e'tiborsiz qoldiriladi.
	if strings.TrimSpace(m.Text) == "" {
		m.Text = strings.TrimSpace(m.Caption)
	}
	if strings.TrimSpace(m.Text) == "" && m.fileID() == "" {
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
		return
	}

	// Qolgan hamma holat: guruhga biz yuborgan boshqa xabarga reply —
	// masalan mijozning RASMIGA (OCR o'qiy olmagani uchun yuborilgan)
	// yoki muammosi allaqachon yopilgan eslatmaga. Ilgari bunday javob
	// hech qayerga bog'lanmay yo'qolardi.
	if p := FindTelegramPost(m.ReplyTo.MessageID); p != nil {
		handlePostReply(m, p, who)
		return
	}

	// Bazada yozuv yo'q (xabar telegram_posts jadvali paydo bo'lishidan
	// oldin ketgan) — suhbat xabarning o'z matnidan o'qiladi.
	if p := postFromText(m.ReplyTo.Text + "\n" + m.ReplyTo.Caption); p != nil {
		handlePostReply(m, p, who)
		return
	}

	log.Printf("telegram: %d-xabarga javob keldi, lekin u qaysi suhbatga tegishli ekani noma'lum",
		m.ReplyTo.MessageID)
}

// handlePostReply - guruhga yuborilgan xabarimizga (rasm, eslatma va
// h.k.) kelgan reply: mijozga javob odatdagi yo'l bilan tayyorlanadi.
func handlePostReply(m *tgMsgUpdate, p *TelegramPost, who string) {
	log.Printf("telegram: suhbat %d — guruhdagi %s xabariga %s javob berdi",
		p.ConversationID, p.Kind, who)

	supersedePending(p.ConversationID)
	markHelpAnsweredIn(p.ConversationID, who)

	img, ok := staffReplyImage(m)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	holat := "mijozga javob tayyorlanmadi"
	if in, err := AnswerFromStaffPost(ctx, p, m.Text, who, img); err != nil {
		log.Printf("telegram: suhbat %d — mijozga javob tayyorlanmadi: %v", p.ConversationID, err)
	} else {
		holat = staffReplyStatus(in)
	}

	if _, err := SendTelegramMessage(
		fmt.Sprintf("✅ Javob qabul qilindi (%s).\n%s", who, holat), m.MessageID); err != nil {
		log.Printf("telegram: tasdiq yuborilmadi: %v", err)
	}
}

// staffReplyStatus - xodimga guruhda qaytariladigan qisqa holat.
//
// Eng muhimi ikkinchi holat: model javobni qayta yoza olmagan bo'lsa,
// xodimning XOM matni mijozga yuborilmaydi (u ichki tilda yozilgan).
// Xodim buni bilib tursin — javob "ketdi" deb o'ylab qolmasin.
func staffReplyStatus(in *Interaction) string {
	return staffImageNote(in) + staffSendStatus(in)
}

// staffImageNote - javobga rasm biriktirilgan bo'lsa qisqa satr. Xodim
// rasm ketgan-ketmaganini bilib tursin.
func staffImageNote(in *Interaction) string {
	if in == nil || in.ImageURL == "" {
		return ""
	}
	return "🖼 Rasm ham yuboriladi (matndan oldin).\n"
}

func staffSendStatus(in *Interaction) string {
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

	// Yordam so'roviga javob keldi — endi u "javobsiz" hisoblanmaydi
	// (panel hisoboti shu maydonga qaraydi). Mijozga javob tayyorlash
	// keyin bo'ladi va u ishlamay qolsa ham, xodim javob berganini
	// yo'qotmaymiz.
	markHelpAnswered(src, who)
	markHelpAnsweredIn(src.ConversationID, who)

	holat := "mijozga javob tayyorlanmadi"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	img, ok := staffReplyImage(m)
	if !ok {
		return
	}
	if in, err := AnswerFromStaffHelp(ctx, src, m.Text, who, img); err != nil {
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

	// Shu suhbat bo'yicha guruhga ketgan yordam so'rovlari endi javob
	// olgan hisoblanadi. Xabar id'si bo'yicha emas, SUHBAT bo'yicha
	// belgilanadi: xodim ko'pincha 🔁 eslatma xabariga reply qiladi,
	// uning id'si esa asl "yordam kerak" xabarinikidan boshqa
	// (sendRemind har eslatmada yangi id yozadi) — shuning uchun
	// hisobotda javoblar umuman ko'rinmay qolardi.
	seen := map[int64]bool{}
	for i := range issues {
		cid := issues[i].ConversationID
		if cid <= 0 || seen[cid] {
			continue
		}
		seen[cid] = true
		markHelpAnsweredIn(cid, who)
	}

	// Xodim javobidan mijozga xabar tayyorlanadi: LLM uni mijoz tiliga
	// moslab yozadi, so'ng odatdagi qoida bo'yicha ketadi (avto-javob
	// yoqiq bo'lsa darhol, aks holda tasdiqlash navbatiga).
	holat := "mijozga javob tayyorlanmadi"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Buyurtmalar bitta mijozniki — mijozga ham bitta javob tayyorlanadi.
	img, ok := staffReplyImage(m)
	if !ok {
		return
	}
	if in, err := AnswerFromStaffReply(ctx, issues, m.Text, who, img); err != nil {
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

// markHelpAnswered - guruhdagi yordam so'roviga mutaxassis javob
// berganini yozib qo'yadi. Birinchi javob saqlanadi: keyingi replylar
// vaqtni surib yubormasin.
func markHelpAnswered(src *Interaction, who string) {
	if DB == nil || src == nil || src.ID == 0 || src.HelpAnsweredAt != nil {
		return
	}
	now := time.Now()
	if err := DB.Model(src).Updates(map[string]any{
		"help_answered_at": &now,
		"help_answered_by": who,
	}).Error; err != nil {
		log.Printf("telegram: yordam so'rovi javob berilgan deb belgilanmadi (%d): %v", src.ID, err)
		return
	}
	src.HelpAnsweredAt = &now
	src.HelpAnsweredBy = who
}

// markHelpAnsweredIn - suhbat bo'yicha guruhga ketgan va hali javobsiz
// turgan yordam so'rovlarining HAMMASINI "javob berilgan" deb
// belgilaydi.
//
// Xodim bitta reply bilan mijozning butun savoliga javob beradi —
// o'sha suhbat bo'yicha qolgan so'rovlar ham yopiq hisoblanadi.
func markHelpAnsweredIn(conversationID int64, who string) {
	if DB == nil || conversationID <= 0 {
		return
	}
	now := time.Now()
	res := DB.Model(&Interaction{}).
		Where("conversation_id = ? AND help_sent AND help_answered_at IS NULL", conversationID).
		Updates(map[string]any{"help_answered_at": &now, "help_answered_by": who})
	if res.Error != nil {
		log.Printf("telegram: suhbat %d — yordam so'rovi belgilanmadi: %v", conversationID, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("telegram: suhbat %d — %d ta yordam so'rovi javob berilgan deb belgilandi (%s)",
			conversationID, res.RowsAffected, who)
	}
}

// fileID - xabarga biriktirilgan rasmning file_id'si. Rasm bir necha
// o'lchamda keladi — eng kattasi olinadi (oxirgisi). "Fayl sifatida"
// yuborilgan rasm `document` da keladi; rasm bo'lmagan hujjat (pdf,
// doc) olinmaydi — mijozga faqat rasm yuboriladi.
func (m *tgMsgUpdate) fileID() string {
	if n := len(m.Photo); n > 0 {
		return m.Photo[n-1].FileID
	}
	if m.Document != nil && strings.HasPrefix(strings.ToLower(m.Document.MimeType), "image/") {
		return m.Document.FileID
	}
	return ""
}

// staffImageURL - xodim biriktirgan rasmni Telegramdan olib, support
// omboriga yuklaydi va mijozga yuborsa bo'ladigan havolani qaytaradi.
//
// Xato bo'lsa bo'sh qiymat qaytadi va javob MATNSIZ qolmaydi: rasm
// ketmasa ham xodimning izohi mijozga boradi, xodimga esa guruhda
// ogohlantirish yoziladi.
func staffImageURL(m *tgMsgUpdate) string {
	id := m.fileID()
	if id == "" {
		return ""
	}
	name, data, err := TelegramFile(id)
	if err != nil {
		log.Printf("telegram: rasmni olib bo'lmadi: %v", err)
		return ""
	}
	url, err := UploadToStorage(name, data)
	if err != nil {
		log.Printf("telegram: rasm omborga yuklanmadi: %v", err)
		return ""
	}
	log.Printf("telegram: xodim rasmi yuklandi (%d bayt) → %s", len(data), url)
	return url
}

// staffReplyImage - rasmni tayyorlaydi va javob davom etsa bo'ladimi
// shuni aytadi.
//
// Ikkinchi qiymat false bo'lsa yuboradigan hech narsa qolmagan: xodim
// faqat rasm yuborgan, rasm esa yuklanmagan. Bunday paytda jim
// qolmaymiz — xodim guruhda ogohlantirish oladi, aks holda u javob
// ketdi deb o'ylab qoladi.
func staffReplyImage(m *tgMsgUpdate) (string, bool) {
	img := staffImageURL(m)
	if img != "" || strings.TrimSpace(m.Text) != "" {
		return img, true
	}
	if _, err := SendTelegramMessage(
		"⚠️ Rasmni yuklab bo'lmadi va izoh ham yo'q — mijozga hech narsa yuborilmadi. "+
			"Iltimos, rasmni qayta yuboring yoki izoh yozing.", m.MessageID); err != nil {
		log.Printf("telegram: ogohlantirish yuborilmadi: %v", err)
	}
	return "", false
}

var (
	// suhbatRe - guruh xabari sarlavhasidagi "Suhbat: #49463".
	suhbatRe = regexp.MustCompile(`(?i)suhbat:?\s*#?(\d+)`)
	// mijozRe - "Mijoz: 8520720" yoki eski ko'rinish "Mijoz 8520720".
	mijozRe = regexp.MustCompile(`(?i)mijoz:?\s*(\d+)`)
)

// postFromText - guruh xabarining matnidan qaysi suhbat ekanini
// aniqlaydi. Avval "Suhbat: #<id>" (aniq), u bo'lmasa "Mijoz: <id>"
// bo'yicha o'sha mijozning eng oxirgi suhbati olinadi.
//
// Bu telegram_posts jadvali paydo bo'lishidan OLDIN guruhga ketgan
// xabarlar uchun: ularga kelgan javob ham yo'qolib ketmasin.
func postFromText(text string) *TelegramPost {
	if strings.TrimSpace(text) == "" || DB == nil {
		return nil
	}
	if g := suhbatRe.FindStringSubmatch(text); len(g) == 2 {
		if id, err := strconv.ParseInt(g[1], 10, 64); err == nil && id > 0 {
			p := &TelegramPost{ConversationID: id, Kind: "matn"}
			var st ConversationState
			if err := DB.First(&st, "conversation_id = ?", id).Error; err == nil {
				p.ClientID = st.ClientID
			}
			return p
		}
	}
	g := mijozRe.FindStringSubmatch(text)
	if len(g) != 2 {
		return nil
	}
	clientID, err := strconv.ParseInt(g[1], 10, 64)
	if err != nil || clientID <= 0 {
		return nil
	}
	// Mijozning eng oxirgi suhbati: guruhga xabar o'sha suhbatdan
	// ketgan bo'ladi.
	var st ConversationState
	if err := DB.Where("client_id = ?", clientID).
		Order("updated_at desc").First(&st).Error; err != nil {
		return nil
	}
	return &TelegramPost{ConversationID: st.ConversationID, ClientID: clientID, Kind: "matn"}
}
