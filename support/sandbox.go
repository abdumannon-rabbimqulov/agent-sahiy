// Test chat (sandbox): admin paneldan AI agentni mijoz bo'lib sinab
// ko'rish. Suhbat tarixi brauzerda turadi va har so'rovda to'liq
// yuboriladi — tashqi support tizimida bunday suhbat yo'q.
//
// Nega kerak: promt zanjirini tekshirishning yagona yo'li haqiqiy
// suhbatni (RunChain) qayta yurgizish edi — ya'ni o'z matningni yozib
// ko'rib bo'lmasdi, ustiga-ustak javob bazaga tushardi, Telegram
// guruhga chiqardi va avto-javob yoqilgan bo'lsa HAQIQIY mijozga
// ketardi. Sandbox shu uchtasini ham o'chiradi.
//
// O'chmaydigani — tashqi tizimlardan ma'lumot O'QISH (adminka
// buyurtmalari, yetkazma, mijoz tili). Aynan shu javobni haqiqiy
// suhbatdagiday qiladi, shuning uchun test chat ko'rsatilgan
// `client_id` bo'yicha haqiqiy ma'lumotni ko'radi.
package support

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrSandboxNoMessages - suhbat bo'sh yoki oxirgi so'z mijozda emas.
var ErrSandboxNoMessages = errors.New("test chat: oxirgi xabar mijozdan bo'lishi kerak")

// SandboxMessage - paneldan keladigan bitta xabar. Shakl modelga
// ketadigan transkript bilan bir xil (agent.go: TranscriptMessage):
// `type` — "client" yoki "agent".
type SandboxMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// sandboxMessages - paneldan kelgan ro'yxatni zanjir kutadigan
// []Message ga aylantiradi.
//
// Bo'sh xabarlar tashlanadi. Ro'yxat HistoryLimit() gacha kesiladi —
// haqiqiy zanjir ham shunchasini ko'radi, test undan ko'proq ko'rsa
// natija haqiqiydan farq qilardi.
//
// Vaqt hozirgi qilib qo'yiladi: "mijoz hali yozib tugatmagan"
// tekshiruvi (CLIENT_QUIET_SEC) sandbox'da baribir o'tkazib
// yuboriladi, lekin maydon bo'sh qolsa loglar chalkashadi.
func sandboxMessages(in []SandboxMessage) []Message {
	if n := HistoryLimit(); len(in) > n {
		in = in[len(in)-n:]
	}
	now := time.Now().Format(time.RFC3339)
	out := make([]Message, 0, len(in))
	for _, m := range in {
		text := strings.TrimSpace(m.Message)
		if text == "" {
			continue
		}
		kind := "agent"
		if strings.TrimSpace(m.Type) == "client" {
			kind = "client"
		}
		out = append(out, Message{
			ID:         int64(len(out) + 1),
			Message:    text,
			SenderType: kind,
			CreatedAt:  now,
		})
	}
	return out
}

// RunChainSandbox - test chat uchun zanjirni yuritadi.
//
// Tarix tashqaridan beriladi, natija esa faqat qaytariladi: bazaga
// yozilmaydi (Interaction.ID nolligicha qoladi), Telegramga chiqmaydi
// va mijozga yuborilmaydi.
//
// clientID 0 bo'lishi mumkin — u holda buyurtma qidiruvi mijozga
// bog'lanmaydi (zanjir "begona buyurtma" filtrini ishlatmaydi).
func RunChainSandbox(ctx context.Context, conversationID, clientID int64,
	msgs []SandboxMessage) (*Interaction, error) {
	list := sandboxMessages(msgs)
	// Zanjir mijozning oxirgi xabariga javob yozadi. Oxirgi so'z bizda
	// bo'lsa javob yozadigan narsa yo'q — buni 400 bilan aytgan
	// tushunarliroq, zanjirni behuda yurgizgandan ko'ra.
	if len(list) == 0 || !list[len(list)-1].FromClient() {
		return nil, ErrSandboxNoMessages
	}
	return runChain(ctx, conversationID, clientID, runOpts{Sandbox: true, Msgs: list})
}
