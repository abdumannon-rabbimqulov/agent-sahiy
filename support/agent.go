// Agent zanjiri: promt -> Groq -> JSON -> kod harakat qiladi -> keyingi promt.
//
// Promt matnini admin yozadi; kod faqat javobdagi kalitlarga qarab
// yo'naltiradi (contract.go dagi AgentJSON).
package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
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
// Modelga eng ko'pi 10 ta xabar ketadi: uzun tarix na foyda beradi, na
// token. HISTORY_LIMIT bilan kamaytirish mumkin, ko'paytirish emas.
func HistoryLimit() int {
	n := envInt("HISTORY_LIMIT", DefaultHistoryLimit)
	if n > DefaultHistoryLimit {
		n = DefaultHistoryLimit
	}
	return n
}

// ErrAgentDisabled - AI agent panel orqali o'chirib qo'yilgan.
var ErrAgentDisabled = errors.New("AI agent o'chirilgan (sozlamalar: agent_enabled)")

// RunChain bitta suhbat uchun zanjirni yuritadi va natijani bazaga yozadi.
// Xato bo'lsa ham interaksiya saqlanadi (status=failed) — panelda ko'rinadi.
func RunChain(ctx context.Context, conversationID, clientID int64) (*Interaction, error) {
	// O'chirilgan bo'lsa hech narsa qilinmaydi: model'ga so'rov ham
	// ketmaydi, bazaga ham yozilmaydi (behuda "failed" yozuvlar
	// to'planmasin).
	if !AgentEnabled() {
		return nil, ErrAgentDisabled
	}

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
	// Aynan shu murojaatda javob berilayotgan (javobsiz qolgan) mijoz
	// xabarlari — javob yuborilgandan keyin shular o'qilgan deb belgilanadi.
	in.MessageIDs = JoinIDs(UnansweredClientIDs(msgs))
	transcript := formatTranscript(msgs)
	hint := scriptHint(msgs)

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

		userMsg := buildUserMessage(transcript, hint, dataCtx)
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

	// 3. help — TASDIQSIZ, darhol xodimlar guruhiga (zanjir yarim yo'lda
	//    to'xtagan bo'lsa ham: xodimlar muammodan xabardor bo'lsin).
	if in.HelpText != "" {
		if err := DeliverHelp(in); err != nil {
			log.Printf("agent: suhbat %d help yuborilmadi: %v", conversationID, err)
			if in.Error == "" {
				in.Error = err.Error()
			} else {
				in.Error += " | " + err.Error()
			}
		}
	}

	// 4. chat — mijozga. Avto-javob yoqilgan bo'lsa darhol, aks holda
	//    admin tasdig'ini kutadi.
	switch {
	case in.Status == StatusFailed:
		// zanjir xatosi — hech narsa yuborilmaydi

	case in.ChatReply != "":
		if AutoReplyOn() {
			if err := DeliverChat(in); err != nil {
				in.Status = StatusFailed
				in.Error = err.Error()
			} else {
				in.Status = StatusSent
				in.HandledBy = "avto"
				now := time.Now()
				in.SentAt = &now
			}
		} else {
			in.Status = StatusPending
		}

	default:
		// Faqat help bor edi — mijozga yoziladigan narsa yo'q, ya'ni
		// tasdiqlashga ham hojat yo'q.
		if in.HelpSent {
			in.Status = StatusSent
			in.HandledBy = "avto"
			now := time.Now()
			in.SentAt = &now
		} else {
			in.Status = StatusFailed
			if in.Error == "" {
				in.Error = "help yuborilmadi"
			}
		}
	}

	if err := SaveInteraction(DB, in); err != nil {
		return in, fmt.Errorf("bazaga yozish: %w", err)
	}
	log.Printf("agent: suhbat %d — %s, %d bosqich, %s", conversationID, in.Status, in.StepsCount, usage)
	return in, nil
}

// DeliverChat mijozga javob yuboradi va javob yetib borsa o'sha xabarlarni
// "o'qilgan" deb belgilaydi. Mijozga ketadigan yagona yo'l shu.
func DeliverChat(in *Interaction) error {
	if in.ChatReply == "" {
		return nil
	}
	if err := SendToClient(in.ConversationID, in.ChatReply); err != nil {
		return fmt.Errorf("chat: %w", err)
	}

	// Javob mijozga yetib bordi — endi xabarlarni o'qilgan deb belgilaymiz.
	if !in.ReadMarked {
		ids := SplitIDs(in.MessageIDs)
		if err := MarkReadCached(ids); err != nil {
			// Javob ketgan, faqat belgi qo'yilmadi — murojaatni xato
			// deb hisoblamaymiz, log yetarli.
			log.Printf("agent: suhbat %d xabarlari o'qilgan deb belgilanmadi: %v", in.ConversationID, err)
		} else {
			in.ReadMarked = true
			saveFlag(in, "read_marked", true)
			log.Printf("agent: suhbat %d — %d ta xabar o'qilgan deb belgilandi", in.ConversationID, len(ids))
		}
	}
	return nil
}

// DeliverHelp xodimlar guruhiga (Telegram) xabar yuboradi.
//
// help TASDIQ KUTMAYDI: mijozga hech narsa ketmaydi, xodimlar esa muammodan
// darhol xabardor bo'lishi kerak. Tasdiqlash faqat mijozga yoziladigan
// chat javobiga tegishli.
func DeliverHelp(in *Interaction) error {
	if in.HelpText == "" || in.HelpSent {
		return nil
	}
	text := fmt.Sprintf("🆘 Suhbat #%d (mijoz %d)\n\n%s", in.ConversationID, in.ClientID, in.HelpText)
	if in.ClientMessage != "" {
		text += "\n\nMijoz xabari: " + in.ClientMessage
	}
	if err := SendTelegram(text); err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	in.HelpSent = true
	saveFlag(in, "help_sent", true)
	log.Printf("agent: suhbat %d — help xodimlar guruhiga yuborildi", in.ConversationID)
	return nil
}

// Deliver - admin tasdiqlaganda: chat mijozga ketadi, help hali
// yuborilmagan bo'lsa (masalan Telegram ishlamay qolgan edi) qayta uriniladi.
func Deliver(in *Interaction) error {
	var errs []string
	if err := DeliverChat(in); err != nil {
		errs = append(errs, err.Error())
	}
	if err := DeliverHelp(in); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// saveFlag - bazadagi bitta bayroqni yangilaydi (yozuv hali saqlanmagan
// bo'lsa hech narsa qilinmaydi — qiymat struct'da qolib, keyin saqlanadi).
func saveFlag(in *Interaction, field string, val bool) {
	if DB != nil && in.ID > 0 {
		DB.Model(in).Update(field, val)
	}
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

// TranscriptMessage - modelga ketadigan bitta xabar. `type` — xabarni kim
// yozgani: "client" (mijoz) yoki "agent" (biz tomon: agent yoki xodim).
//
// Sana yuborilmaydi: modelga xabarlar tartibi yetarli, sana esa faqat
// token sarflaydi va javobda chalkashlik keltirib chiqaradi (haqiqiy
// sanalar "Tizimdagi ma'lumot" blokida keladi).
type TranscriptMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SenderKind - xabarni kim yozgani: "client" yoki "agent".
func (m Message) SenderKind() string {
	if m.FromClient() {
		return "client"
	}
	return "agent"
}

// formatTranscript oxirgi xabarlarni modelga JSON ro'yxat qilib beradi —
// har biri o'z turi bilan, eskisidan yangisiga.
func formatTranscript(msgs []Message) string {
	// Xavfsizlik uchun yana bir marta kesamiz: modelga 10 tadan ortiq
	// xabar ketmasligi kerak (server ko'proq qaytarib yuborsa ham).
	if n := HistoryLimit(); len(msgs) > n {
		msgs = msgs[len(msgs)-n:]
	}

	out := make([]TranscriptMessage, 0, len(msgs))
	for _, m := range msgs {
		text := strings.TrimSpace(m.Message)
		if text == "" {
			continue
		}
		out = append(out, TranscriptMessage{
			Type:    m.SenderKind(),
			Message: text,
		})
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// Til aniqlash uchun belgilar. Ro'yxatlar qisqa ataylab: maqsad — aniq
// holatni ushlash, shubhali holatda esa modelga tilni o'zi tanlashiga
// ruxsat berish.
var (
	// O'zbekcha kirillga xos harflar.
	uzCyrLetters = []rune{'ў', 'қ', 'ғ', 'ҳ'}
	// Ruschaga xos harflar (o'zbekcha kirillda deyarli uchramaydi).
	ruLetters = []rune{'ы', 'ъ', 'э', 'ё', 'щ'}
	// Tez-tez uchraydigan so'zlar.
	uzWords = []string{"качон", "қачон", "буюртма", "керак", "йўқ", "йук", "бор",
		"учун", "нима", "туриб", "бўлди", "булди", "менинг", "олдим", "жўнат",
		"жунат", "етказ", "савол", "рахмат", "раҳмат", "яна", "ҳали", "хали"}
	ruWords = []string{"что", "когда", "почему", "заказ", "здравствуйте", "привет",
		"спасибо", "получил", "пришл", "отправ", "где", "мой", "моя", "мне",
		"это", "как", "уже", "ещё", "еще", "вопрос", "ответ", "деньги"}
)

// lastClientMessage - oxirgi bo'sh bo'lmagan mijoz xabari.
func lastClientMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() && strings.TrimSpace(msgs[i].Message) != "" {
			return msgs[i].Message
		}
	}
	return ""
}

// scriptHint - mijozning oxirgi xabari qaysi alifboda va (aniq bo'lsa)
// qaysi tilda ekanini modelga aytadi.
//
// Promt matnining o'zi bunga yetmaydi: promt o'zbekcha yozilgani uchun
// model ruscha xabarga ham o'zbekcha javob yozib yuboradi, kirill xabarga
// esa lotinda javob beradi. Bu yerda taxmin emas — matndagi harflar va
// so'zlar hisobi.
func scriptHint(msgs []Message) string {
	last := lastClientMessage(msgs)
	if last == "" {
		return ""
	}
	low := strings.ToLower(last)

	var cyr, lat int
	for _, r := range last {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case unicode.Is(unicode.Latin, r):
			lat++
		}
	}
	if cyr == 0 && lat == 0 {
		return ""
	}

	const keepLang = " Mijozning TILINI o'zgartirma: qaysi tilda yozgan bo'lsa, " +
		"javob ham aynan o'sha tilda bo'lsin."

	if cyr <= lat {
		return "MUHIM: mijoz LOTIN alifboda yozgan — javobni ham lotin alifboda yoz." + keepLang
	}

	// Kirill: rus tilimi yoki o'zbekcha kirillmi?
	uz, ru := 0, 0
	for _, r := range uzCyrLetters {
		uz += strings.Count(low, string(r))
	}
	for _, r := range ruLetters {
		ru += strings.Count(low, string(r))
	}
	for _, w := range uzWords {
		if strings.Contains(low, w) {
			uz += 2
		}
	}
	for _, w := range ruWords {
		if strings.Contains(low, w) {
			ru += 2
		}
	}

	switch {
	case ru > uz:
		return "MUHIM: mijoz RUS tilida yozgan — javobni ham rus tilida yoz. " +
			"O'zbekchaga o'girma."
	case uz > ru:
		return "MUHIM: mijoz O'ZBEK tilida, KIRILL alifboda yozgan — javobni " +
			"ham o'zbekcha kirill alifboda yoz. Lotinga ham, ruschaga ham o'girma."
	default:
		return "MUHIM: mijoz KIRILL alifboda yozgan — javobni ham kirill " +
			"alifboda yoz, lotinga o'girma." + keepLang
	}
}

// buildUserMessage modelga ketadigan matn: suhbat + alifbo ko'rsatmasi +
// tizimdan olingan ma'lumot.
func buildUserMessage(transcript, hint string, data []string) string {
	var b strings.Builder
	b.WriteString("Suhbatning oxirgi xabarlari (eskisidan yangisiga). ")
	b.WriteString(`"type": "client" — mijoz yozgan, "type": "agent" — biz yozgan javob:` + "\n")
	b.WriteString(transcript)
	if hint != "" {
		b.WriteString("\n\n" + hint)
	}
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
