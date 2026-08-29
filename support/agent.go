// Agent zanjiri: promt -> Groq -> JSON -> kod harakat qiladi -> keyingi promt.
//
// Promt matnini admin yozadi; kod faqat javobdagi kalitlarga qarab
// yo'naltiradi (contract.go dagi AgentJSON).
package support

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// Zanjir sozlamalari (.env).
const (
	DefaultStartPromtID  = 1
	DefaultMaxSteps      = 5
	DefaultHistoryLimit  = 10
	DefaultOrdersPerCall = 20
)

// StartPromtID - zanjir qaysi promtdan boshlanadi.
func StartPromtID() uint { return uint(envInt("START_PROMPT_ID", DefaultStartPromtID)) }

// MaxSteps - eng ko'p necha bosqich.
func MaxSteps() int { return envInt("AGENT_MAX_STEPS", DefaultMaxSteps) }

// HistoryLimit - modelga ko'rsatiladigan oxirgi xabarlar soni.
func HistoryLimit() int { return envInt("HISTORY_LIMIT", DefaultHistoryLimit) }

// RunChain bitta suhbat uchun zanjirni yuritadi va natijani bazaga yozadi.
// Xato bo'lsa ham interaksiya saqlanadi (status=failed) — panelda ko'rinadi.
func RunChain(ctx context.Context, conversationID, clientID int64) (*Interaction, error) {
	in := &Interaction{
		ConversationID: conversationID,
		ClientID:       clientID,
		Status:         StatusFailed,
	}

	// 1. Suhbat tarixi.
	msgs, err := fetchHistory(conversationID)
	if err != nil {
		in.Error = fmt.Sprintf("xabarlarni olish: %v", err)
		saveOrLog(in)
		return in, err
	}
	if len(msgs) == 0 {
		in.Error = "suhbatda xabar yo'q"
		saveOrLog(in)
		return in, fmt.Errorf("suhbat %d: xabar yo'q", conversationID)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() {
			in.ClientMessage = msgs[i].Message
			break
		}
	}
	transcript := formatTranscript(msgs)

	// 2. Zanjir.
	groq := GroqFromEnv()
	if !groq.Ready() {
		in.Error = ErrNoGroqKey.Error()
		saveOrLog(in)
		return in, ErrNoGroqKey
	}

	var (
		usage    Usage
		dataCtx  []string // oldingi bosqichlarda yig'ilgan tizim ma'lumoti
		promtID  = StartPromtID()
		maxSteps = MaxSteps()
	)

	for step := 1; step <= maxSteps; step++ {
		p, err := GetPromt(DB, promtID)
		if err != nil {
			in.Error = fmt.Sprintf("promt %d topilmadi", promtID)
			break
		}

		userMsg := buildUserMessage(transcript, dataCtx)
		raw, u, err := groq.Generate(ctx, p.Promt, userMsg)
		usage = usage.Add(u)

		in.Steps = append(in.Steps, AgentStep{
			StepNo:           step,
			PromtID:          p.ID,
			PromtTitle:       p.Title,
			RequestContext:   userMsg,
			RawResponse:      raw,
			PromptTokens:     u.PromptTokens,
			CachedTokens:     u.CachedTokens,
			CompletionTokens: u.CompletionTokens,
			DurationMS:       u.DurationMS,
			CreatedAt:        time.Now(),
		})

		if err != nil {
			in.Error = fmt.Sprintf("%d-bosqich (promt %d): %v", step, promtID, err)
			break
		}

		a, err := ParseAgentJSON(raw)
		if err != nil {
			in.Error = fmt.Sprintf("%d-bosqich (promt %d): %v", step, promtID, err)
			break
		}

		// Modelning javobi: oxirgi bo'sh bo'lmagan matn ustun keladi.
		if a.Chat != "" {
			in.ChatReply = a.Chat
		}
		if a.Help != "" {
			in.HelpText = a.Help
		}

		// Kod tizimdan ma'lumot oladi va keyingi bosqichga beradi.
		if a.NeedsData() {
			dataCtx = append(dataCtx, fetchSystemData(a, clientID))
		}

		next, more := a.NextPromt()
		if !more {
			in.Error = ""
			in.Status = StatusPending
			break
		}
		if next == promtID {
			in.Error = fmt.Sprintf("promt %d o'zini chaqirdi — zanjir to'xtatildi", promtID)
			break
		}
		promtID = next

		if step == maxSteps {
			in.Error = fmt.Sprintf("bosqichlar chegarasi (%d) tugadi, zanjir tugallanmadi", maxSteps)
		}
	}

	in.StepsCount = len(in.Steps)
	in.Model = usage.Model
	in.PromptTokens = usage.PromptTokens
	in.CachedTokens = usage.CachedTokens
	in.CompletionTokens = usage.CompletionTokens
	in.Calls = usage.Calls
	in.CostUSD = usage.Cost()

	if in.Error != "" {
		in.Status = StatusFailed
	} else if in.ChatReply == "" && in.HelpText == "" {
		in.Status = StatusFailed
		in.Error = "model na chat, na help qaytardi"
	}

	// 3. Avto-javob yoqilgan bo'lsa darhol yuboramiz.
	if in.Status == StatusPending && AutoReplyOn() {
		if err := Deliver(in); err != nil {
			in.Status = StatusFailed
			in.Error = err.Error()
		} else {
			in.Status = StatusSent
			in.HandledBy = "avto"
			now := time.Now()
			in.SentAt = &now
		}
	}

	if err := SaveInteraction(DB, in); err != nil {
		return in, fmt.Errorf("bazaga yozish: %w", err)
	}
	log.Printf("agent: suhbat %d — %s, %d bosqich, %s", conversationID, in.Status, in.StepsCount, usage)
	return in, nil
}

// Deliver interaksiya natijasini manzillarga yuboradi:
// chat -> mijozga (support chat), help -> Telegram guruhga.
func Deliver(in *Interaction) error {
	var errs []string
	if in.ChatReply != "" {
		if err := SendToClient(in.ConversationID, in.ChatReply); err != nil {
			errs = append(errs, "chat: "+err.Error())
		}
	}
	if in.HelpText != "" {
		text := fmt.Sprintf("🆘 Suhbat #%d (mijoz %d)\n\n%s", in.ConversationID, in.ClientID, in.HelpText)
		if in.ClientMessage != "" {
			text += "\n\nMijoz xabari: " + in.ClientMessage
		}
		if err := SendTelegram(text); err != nil {
			errs = append(errs, "telegram: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// fetchHistory suhbatning oxirgi xabarlarini oladi (token eskirsa yangilaydi).
func fetchHistory(conversationID int64) ([]Message, error) {
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return nil, err
	}
	msgs, err := FetchMessages(creds.BaseURL, token, conversationID, HistoryLimit())
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			msgs, err = FetchMessages(creds.BaseURL, token, conversationID, HistoryLimit())
		}
	}
	return msgs, err
}

// formatTranscript xabarlarni modelga tushunarli matnga aylantiradi.
func formatTranscript(msgs []Message) string {
	var b strings.Builder
	for _, m := range msgs {
		who := "XODIM"
		if m.FromClient() {
			who = "MIJOZ"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, strings.TrimSpace(m.Message))
	}
	return strings.TrimSpace(b.String())
}

// buildUserMessage modelga ketadigan matn: suhbat + tizimdan olingan ma'lumot.
func buildUserMessage(transcript string, data []string) string {
	var b strings.Builder
	b.WriteString("Suhbat:\n")
	b.WriteString(transcript)
	if len(data) > 0 {
		b.WriteString("\n\nTizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):\n")
		b.WriteString(strings.Join(data, "\n"))
	}
	return b.String()
}

// fetchSystemData model so'ragan manbalardan ma'lumot oladi va JSON matn
// qilib qaytaradi. Xato bo'lsa ham matn qaytadi — model buni ko'radi.
func fetchSystemData(a AgentJSON, clientID int64) string {
	out := map[string]any{}
	numbers := a.Numbers()

	if a.Adminka {
		adm := AdminkaFromEnv()
		var rows []AdminkaOrder
		var errs []string
		if len(numbers) == 0 {
			r, err := FetchOrders(adm, OrderFilter{UserID: clientID, Size: DefaultOrdersPerCall})
			rows, errs = appendResult(rows, errs, r, err)
		}
		for _, n := range numbers {
			f := OrderFilter{Size: DefaultOrdersPerCall}
			if strings.HasPrefix(strings.ToUpper(n), "DG") {
				f.OrderSN = n
			} else {
				f.ExpressNum = n
			}
			r, err := FetchOrders(adm, f)
			rows, errs = appendResult(rows, errs, r, err)
		}
		out["adminka"] = rows
		if len(errs) > 0 {
			out["adminka_error"] = strings.Join(errs, "; ")
		}
	}

	if a.Dashboard {
		svc := ServiceFromEnv()
		token, err := ServiceToken(svc, ServiceTokenFile)
		if err != nil {
			out["dashboard_error"] = err.Error()
		} else {
			// Yetkazma faqat trek raqami bilan qidiriladi; DG buyurtma
			// raqami bu yerda ishlamaydi, shuning uchun trek bo'lmasa
			// mijozning barcha yetkazmalari olinadi.
			tracks := trimAll(a.ExpressNum)
			var rows []DeliveryOrder
			var errs []string
			if len(tracks) == 0 {
				r, err := fetchDeliveryRetry(svc, token, DeliveryFilter{UserID: clientID, Size: DefaultOrdersPerCall})
				rows, errs = appendDelivery(rows, errs, r, err)
			}
			for _, n := range tracks {
				r, err := fetchDeliveryRetry(svc, token, DeliveryFilter{TrackNumber: n, Size: DefaultOrdersPerCall})
				rows, errs = appendDelivery(rows, errs, r, err)
			}
			out["dashboard"] = rows
			if len(errs) > 0 {
				out["dashboard_error"] = strings.Join(errs, "; ")
			}
		}
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}

// fetchDeliveryRetry - token eskirgan bo'lsa bir marta yangilab qayta uriniladi.
func fetchDeliveryRetry(svc Service, token string, f DeliveryFilter) ([]DeliveryOrder, error) {
	rows, err := FetchDelivery(svc, token, f)
	if err == ErrUnauthorized {
		if token, err = ServiceRefresh(svc, ServiceTokenFile); err == nil {
			rows, err = FetchDelivery(svc, token, f)
		}
	}
	return rows, err
}

// trimAll bo'sh satrlarni tashlab, chekkalarini tozalaydi.
func trimAll(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appendResult(rows []AdminkaOrder, errs []string, r []AdminkaOrder, err error) ([]AdminkaOrder, []string) {
	if err != nil {
		return rows, append(errs, err.Error())
	}
	return append(rows, r...), errs
}

func appendDelivery(rows []DeliveryOrder, errs []string, r []DeliveryOrder, err error) ([]DeliveryOrder, []string) {
	if err != nil {
		return rows, append(errs, err.Error())
	}
	return append(rows, r...), errs
}

// saveOrLog - erta xatoda ham interaksiyani saqlaymiz (panelda ko'rinsin).
func saveOrLog(in *Interaction) {
	if DB == nil {
		return
	}
	if err := SaveInteraction(DB, in); err != nil {
		log.Printf("agent: interaksiyani saqlab bo'lmadi: %v", err)
	}
}
