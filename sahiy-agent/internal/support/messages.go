package support

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"sahiy-agent/internal/client"
)

// messagePath — kerak bo'lsa shu yerdan o'zgartiring.
const messagePath = "/api/v1/support.chat.message/conversation"

// Message — bitta chat xabari.
type Message struct {
	ID             int64           `json:"id"`
	SenderID       *int64          `json:"sender_id"`
	SenderType     string          `json:"sender_type"` // "client" yoki "agent"
	ConversationID int64           `json:"conversation_id"`
	Message        string          `json:"message"`
	Content        json.RawMessage `json:"content"` // tuzilishi noma'lum → xom
	Status         int             `json:"status"`
	SupportField   json.RawMessage `json:"support_field"` // tuzilishi noma'lum → xom
	SeenAt         *string         `json:"seen_at"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// messagesResponse — server javobi ({ "data": [ ... ] }).
type messagesResponse struct {
	Data []Message `json:"data"`
}

// FetchMessages bitta suhbatning xabarlarini olib keladi.
func FetchMessages(c *client.Client, conversationID int64, page, limit int) ([]Message, error) {
	path := fmt.Sprintf("%s/%d?page=%d&limit=%d", messagePath, conversationID, page, limit)
	body, status, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("xabarlarni olish muvaffaqiyatsiz (status %d): %s", status, string(body))
	}

	var resp messagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("javobni o'qib bo'lmadi: %w\nXom javob: %s", err, string(body))
	}
	return resp.Data, nil
}

// IsImage — xabar rasmmi. Server `content` maydonida oddiy satr yuboradi:
// "text" yoki "image"; rasm URL'i esa `message` maydonida keladi.
func (m Message) IsImage() bool {
	return strings.TrimSpace(string(m.Content)) == `"image"`
}

// ImageMessages mijoz yuborgan rasm xabarlarini qaytaradi — eng yangisi
// birinchi, limit tagacha (limit <= 0 bo'lsa hammasi).
func ImageMessages(msgs []Message, limit int) []Message {
	var out []Message
	for _, m := range msgs {
		if m.SenderType == "client" && m.IsImage() && m.Message != "" {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ImageURLs oynadagi mijoz rasmlarining asl havolalarini qaytaradi
// (eng yangisi birinchi, limit tagacha; limit <= 0 — hammasi).
// Rasmlar yuklab olinmaydi va tahlil qilinmaydi — faqat havola saqlanadi.
func ImageURLs(msgs []Message, limit int) []string {
	var out []string
	for _, m := range ImageMessages(msgs, limit) {
		out = append(out, m.Message)
	}
	return out
}

// MinHistory — Gemini'ga beriladigan eng kam xabar soni. Agent hech qachon
// bundan kam kontekst bilan javob yozmaydi (suhbatda shuncha xabar bo'lsa).
const MinHistory = 10

// Transcript xabarlarni vaqt bo'yicha tartiblab, LLM (Gemini) promptiga
// beriladigan o'qiladigan matn ko'rinishiga aylantiradi.
// Masalan:
//
//	client: Salom, buyurtmam qayerda?
//	agent: Tekshirib ko'raman...
func Transcript(msgs []Message) string {
	text, _ := TranscriptTail(msgs, 0)
	return text
}

// TranscriptTail suhbatning oxirgi n ta xabaridan transkript yasaydi va
// nechta xabar ishlatilganini qaytaradi. n < MinHistory bo'lsa MinHistory
// ishlatiladi; n <= 0 bo'lsa cheklov qo'yilmaydi (hammasi).
//
// Tail (oxirgi) olinishi muhim: server qanday tartibda qaytarishidan qat'i
// nazar, id bo'yicha saralangandan keyin eng yangi xabarlar tanlanadi.
func TranscriptTail(msgs []Message, n int) (string, int) {
	sorted := make([]Message, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	if n > 0 {
		if n < MinHistory {
			n = MinHistory
		}
		if len(sorted) > n {
			sorted = sorted[len(sorted)-n:]
		}
	}

	var b strings.Builder
	for _, m := range sorted {
		text := m.Message
		// Rasm xabarida `message` — bu URL. Uni xom holda Gemini'ga
		// bermaymiz: rasm alohida tahlil qilinib, natijasi qo'shiladi.
		if m.IsImage() {
			text = "[rasm]"
		} else if text == "" {
			text = string(m.Content)
		}
		role := m.SenderType
		if role == "" {
			role = "unknown"
		}
		fmt.Fprintf(&b, "%s: %s\n", role, text)
	}
	return b.String(), len(sorted)
}

// LastClientMessage oxirgi (eng katta id'li) mijoz xabarini qaytaradi.
// Mijoz xabari bo'lmasa id=0 bo'ladi. Rasm xabarida `message` — bu URL,
// shuning uchun matn o'rniga "[rasm]" qaytadi (xom havola dashboardga ham,
// xodimlar guruhiga ham tushmasin).
func LastClientMessage(msgs []Message) (int64, string) {
	var id int64
	var text string
	for _, m := range msgs {
		if m.SenderType == "client" && m.ID > id {
			id, text = m.ID, m.Message
			if m.IsImage() {
				text = "[rasm]"
			}
		}
	}
	return id, text
}

// --- Yangi xabarlar oynasi (token tejash) ---

// timeLayouts — server `created_at` ni turli formatlarda qaytarishi mumkin.
var timeLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999",
}

// Time xabarning yaratilgan vaqtini qaytaradi. Sana o'qilmasa ok=false —
// bunday xabar hech qachon "eski" deb hisoblanmaydi.
func (m Message) Time() (time.Time, bool) {
	s := strings.TrimSpace(m.CreatedAt)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Age xabarning yoshi (vaqti noma'lum bo'lsa 0, ok=false).
func (m Message) Age(now time.Time) (time.Duration, bool) {
	t, ok := m.Time()
	if !ok {
		return 0, false
	}
	return now.Sub(t), true
}

// Stale — oxirgi mijoz xabari maxAge dan eski bo'lsa true. Bunday suhbatga
// AI chaqirilmaydi (eski, allaqachon dolzarbligini yo'qotgan murojaat).
// maxAge <= 0 bo'lsa tekshiruv o'chiq.
func Stale(msgs []Message, maxAge time.Duration) (bool, time.Duration) {
	if maxAge <= 0 {
		return false, 0
	}
	var newest Message
	for _, m := range msgs {
		if m.SenderType == "client" && m.ID > newest.ID {
			newest = m
		}
	}
	age, ok := newest.Age(time.Now())
	if !ok {
		return false, 0
	}
	return age > maxAge, age
}

// Window — AI'ga beriladigan xabarlar oynasi: javob berilmagan YANGI
// xabarlar va ulardan oldingi `before` ta kontekst xabari.
//
// Oyna hech qachon MinHistory (10) tadan kam bo'lmaydi — suhbatda shuncha
// xabar bo'lsa, agent oxirgi 10 tasini baribir o'qiydi. `max` yuqori
// chegara (HISTORY_LIMIT).
//
// Xabar yoshi bu yerda tekshirilmaydi: eski suhbatga umuman javob berish
// kerakmi degan qarorni Stale() alohida qabul qiladi. Yoshi bo'yicha
// filtrlash kontekstni yeb qo'yardi — eski xabarlar tashlanib, modelga
// suhbatning bitta oxirgi qatori borardi.
func Window(msgs []Message, afterID int64, before, max int) []Message {
	sorted := make([]Message, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	// Yangi xabarlar qayerdan boshlanadi.
	start := len(sorted)
	for i, m := range sorted {
		if m.ID > afterID {
			start = i
			break
		}
	}
	if before < 0 {
		before = 0
	}
	if afterID == 0 {
		// Birinchi ko'rish: hammasi "yangi" — oxirgi before+1 tasi yetadi.
		start = len(sorted)
		before++
	}
	if start-before < 0 {
		before = start
	}
	out := sorted[start-before:]

	// Kontekst juda qisqa chiqsa — oxirgi MinHistory tagacha kengaytiramiz.
	if len(out) < MinHistory && len(sorted) > len(out) {
		n := MinHistory
		if n > len(sorted) {
			n = len(sorted)
		}
		out = sorted[len(sorted)-n:]
	}
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}
