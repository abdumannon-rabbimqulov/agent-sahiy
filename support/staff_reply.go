// Xodimning Telegram guruhdagi javobini mijozga yetkazish.
//
// Xodim guruhda qisqa, ichki tilda yozadi ("omborda qoldi, ertaga
// jo'natamiz"). Uni mijozga o'sha holicha yuborib bo'lmaydi: mijoz
// tilida, xushmuomala va tushunarli qilib qayta yozish kerak — buni LLM
// qiladi. Keyin javob odatdagi qoida bo'yicha ketadi: avto-javob yoqiq
// bo'lsa darhol, aks holda tasdiqlash navbatiga.
package support

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// DefaultStaffPromtID - xodim javobini qayta yozadigan promt.
const DefaultStaffPromtID = 5

// StaffPromtID - .env dagi STAFF_REPLY_PROMPT_ID (default 5).
func StaffPromtID() uint { return uint(envInt("STAFF_REPLY_PROMPT_ID", DefaultStaffPromtID)) }

// AnswerFromStaffReply xodimning javobidan mijozga xabar tayyorlaydi.
//
// Ro'yxatda bitta mijozning bir nechta buyurtmasi bo'lishi mumkin (guruhga
// ketgan bitta xabar) — mijozga baribir BITTA javob tayyorlanadi, unda
// hamma buyurtma raqami ko'rsatiladi.
//
// LLM ishlamasa ham javob YO'QOLMAYDI: xodim matni o'z holicha qoralama
// bo'lib navbatga tushadi va admin uni tahrirlab yuborishi mumkin.
func AnswerFromStaffReply(ctx context.Context, issues []OrderIssue, reply, who string) (*Interaction, error) {
	if len(issues) == 0 {
		return nil, fmt.Errorf("muammo berilmagan")
	}
	is := &issues[0]
	if is.ConversationID <= 0 {
		return nil, fmt.Errorf("suhbat id yo'q")
	}

	sns := issueNumbers(issues)

	in := &Interaction{
		ConversationID: is.ConversationID,
		ClientID:       is.ClientID,
		Source:         SourceTelegram,
		Status:         StatusPending,
		// Zaxira: xodim matni o'z holicha, buyurtma raqami bilan.
		ChatReply: WithOrderSN(strings.TrimSpace(reply), sns),
	}

	// Suhbat tarixi — til va kontekst uchun.
	msgs, err := fetchHistory(is.ConversationID)
	if err != nil {
		log.Printf("xodim javobi: suhbat %d tarixini olib bo'lmadi: %v", is.ConversationID, err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() {
			in.ClientMessage = msgs[i].Message
			break
		}
	}
	in.MessageIDs = JoinIDs(UnansweredClientIDs(msgs))

	// LLM bilan mijoz tiliga moslab yozamiz.
	if usage, err := rewriteStaffReply(ctx, in, sns, reply, msgs); err != nil {
		in.Error = fmt.Sprintf("xodim javobini qayta yozib bo'lmadi: %v", err)
		log.Printf("xodim javobi: %v — xodim matni qoralama bo'lib qoldi", err)
	} else {
		in.Model = usage.Model
		in.PromptTokens = usage.PromptTokens
		in.CachedTokens = usage.CachedTokens
		in.CompletionTokens = usage.CompletionTokens
		in.Calls = usage.Calls
		in.CostUSD = usage.Cost()
		in.StepsCount = len(in.Steps)
	}

	// Avto-javob yoqiq bo'lsa darhol mijozga.
	if AutoReplyOn() {
		if err := DeliverChat(in); err != nil {
			in.Status = StatusFailed
			in.Error = err.Error()
		} else {
			in.Status = StatusSent
			in.HandledBy = who
			now := time.Now()
			in.SentAt = &now
		}
	}

	if err := SaveInteraction(DB, in); err != nil {
		return in, fmt.Errorf("bazaga yozish: %w", err)
	}
	log.Printf("xodim javobi: suhbat %d — %s (%s)", is.ConversationID, in.Status, who)
	return in, nil
}

// rewriteStaffReply - LLM chaqiruvi. Muvaffaqiyatli bo'lsa in.ChatReply
// mijozga mos matn bilan almashadi.
func rewriteStaffReply(ctx context.Context, in *Interaction, sns []string,
	reply string, msgs []Message) (Usage, error) {

	if !AgentEnabled() {
		return Usage{}, fmt.Errorf("AI agent o'chirilgan")
	}
	groq := GroqFromEnv()
	if !groq.Ready() {
		return Usage{}, ErrNoGroqKey
	}

	p, err := GetPromt(DB, StaffPromtID())
	if err != nil {
		return Usage{}, fmt.Errorf("promt %d topilmadi", StaffPromtID())
	}

	// Modelga faqat kerakli narsa: qaysi buyurtma va xodim nima degani.
	// Ichki holat matni ("status_label") ataylab yuborilmaydi — model uni
	// javobga ko'chirib, mijozga ichki atamalarni chiqarib yuborardi.
	info := map[string]any{
		"order_sn":     strings.Join(sns, ", "),
		"xodim_javobi": strings.TrimSpace(reply),
	}
	raw, _ := json.MarshalIndent(info, "", "  ")

	var b strings.Builder
	b.WriteString("Suhbatning oxirgi xabarlari (eskisidan yangisiga). ")
	b.WriteString(`"type": "client" — mijoz yozgan, "type": "agent" — biz yozgan javob:` + "\n")
	b.WriteString(formatTranscript(msgs))
	b.WriteString("\n\nXodim javobi:\n")
	b.Write(raw)
	// Mijoz bir nechta buyurtma haqida yozgan bo'lishi mumkin — javob
	// qaysi buyurtma haqida ekani matnning o'zida ko'rinishi kerak.
	b.WriteString("\n\nJavob matnida buyurtma raqamini (order_sn) albatta yoz — ")
	b.WriteString("mijoz javob qaysi buyurtmasi haqida ekanini bilsin.")
	userMsg := b.String()

	out, usage, err := groq.Generate(ctx, p.Promt, userMsg)

	in.Steps = append(in.Steps, AgentStep{
		StepNo:           1,
		PromtID:          p.ID,
		PromtTitle:       p.Title,
		RequestContext:   userMsg,
		RawResponse:      out,
		PromptTokens:     usage.PromptTokens,
		CachedTokens:     usage.CachedTokens,
		CompletionTokens: usage.CompletionTokens,
		DurationMS:       usage.DurationMS,
		CreatedAt:        time.Now(),
	})
	if err != nil {
		return usage, err
	}

	a, err := ParseAgentJSON(out)
	if err != nil {
		return usage, err
	}
	if a.Chat == "" {
		return usage, fmt.Errorf("model bo'sh javob qaytardi")
	}
	// Model raqamni tashlab ketsa — kod o'zi qo'shadi.
	in.ChatReply = WithOrderSN(a.Chat, sns)
	if a.Help != "" {
		in.HelpText = a.Help
	}
	return usage, nil
}

// issueNumbers - muammolardagi buyurtma raqamlari.
func issueNumbers(issues []OrderIssue) []string {
	out := make([]string, 0, len(issues))
	for _, is := range issues {
		if sn := strings.TrimSpace(is.OrderSN); sn != "" {
			out = append(out, sn)
		}
	}
	return out
}

// WithOrderSN - javob matnida buyurtma raqami borligini kafolatlaydi.
//
// Model (yoki xodim) raqamni yozmagan bo'lsa, matn oldiga qo'shiladi:
// mijoz javob qaysi buyurtmasi haqida ekanini bilishi kerak. Matnda
// allaqachon bor raqam takrorlanmaydi.
func WithOrderSN(text string, sns []string) string {
	text = strings.TrimSpace(text)
	if text == "" || len(sns) == 0 {
		return text
	}
	var missing []string
	for _, sn := range sns {
		if !strings.Contains(text, sn) {
			missing = append(missing, sn)
		}
	}
	if len(missing) == 0 {
		return text
	}
	return strings.Join(missing, ", ") + " — " + text
}
