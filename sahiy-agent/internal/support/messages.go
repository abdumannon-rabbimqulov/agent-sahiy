package support

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
// Mijoz xabari bo'lmasa id=0 bo'ladi.
func LastClientMessage(msgs []Message) (int64, string) {
	var id int64
	var text string
	for _, m := range msgs {
		if m.SenderType == "client" && m.ID > id {
			id, text = m.ID, m.Message
		}
	}
	return id, text
}
