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

// photoOnlyReply - xodim izohsiz, faqat rasm yuborgan holat uchun
// mijozga ketadigan qisqa matn (rasm o'zi asosiy javob).
const photoOnlyReply = "Quyidagi rasmni yubordik — savolingiz bo'lsa yozing."

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
func AnswerFromStaffReply(ctx context.Context, issues []OrderIssue, reply, who, imageURL string) (*Interaction, error) {
	if len(issues) == 0 {
		return nil, fmt.Errorf("muammo berilmagan")
	}
	is := &issues[0]
	return answerFromStaff(ctx, is.ConversationID, is.ClientID, issueNumbers(issues), reply, who, imageURL)
}

// AnswerFromStaffHelp - "🆘 Yordam kerak" xabariga kelgan reply.
// Muammoli buyurtma xabaridan farqi faqat shu: yopiladigan buyurtma
// yozuvi yo'q, mijozga javob esa AYNAN bir xil yo'l bilan tayyorlanadi.
func AnswerFromStaffHelp(ctx context.Context, src *Interaction, reply, who, imageURL string) (*Interaction, error) {
	if src == nil {
		return nil, fmt.Errorf("murojaat berilmagan")
	}
	return answerFromStaff(ctx, src.ConversationID, src.ClientID, nil, reply, who, imageURL)
}

// AnswerFromStaffPost - guruhdagi boshqa xabarimizga (mijoz rasmi,
// eslatma va h.k.) kelgan reply. Yopiladigan muammo yozuvi ham,
// manba murojaat ham yo'q — faqat suhbat ma'lum.
func AnswerFromStaffPost(ctx context.Context, p *TelegramPost, reply, who, imageURL string) (*Interaction, error) {
	if p == nil {
		return nil, fmt.Errorf("xabar yozuvi berilmagan")
	}
	return answerFromStaff(ctx, p.ConversationID, p.ClientID, nil, reply, who, imageURL)
}

// answerFromStaff - ikkala yo'l uchun umumiy qism: xodim matnini LLM
// bilan mijoz tiliga moslab yozadi va odatdagi qoida bo'yicha yuboradi.
func answerFromStaff(ctx context.Context, conversationID, clientID int64,
	sns []string, reply, who, imageURL string) (*Interaction, error) {

	if conversationID <= 0 {
		return nil, fmt.Errorf("suhbat id yo'q")
	}

	// Javobda ko'rsatiladigan raqamlar: xodim o'z matnida raqam yozgan
	// bo'lsa o'shalar, aks holda guruh xabaridagilar (effectiveNumbers).
	numSN, numEx := effectiveNumbers(reply, sns)
	showNums := mergeNumbers(numSN, numEx, 2*staffNumbersMax)

	in := &Interaction{
		ConversationID: conversationID,
		ClientID:       clientID,
		Source:         SourceTelegram,
		Status:         StatusPending,
		// Zaxira: xodim matni o'z holicha, buyurtma raqami bilan.
		ChatReply: WithOrderSN(strings.TrimSpace(reply), showNums),
		// Rasm javob bilan birga ketadi: DeliverChat uni MATNDAN OLDIN
		// yuboradi.
		ImageURL: imageURL,
	}
	// Xodim faqat rasm yuborgan bo'lsa (izohsiz) — matn o'rniga qisqa
	// tayyor jumla qo'yiladi: support serveri bo'sh xabarni qabul
	// qilmaydi va mijoz rasmni izohsiz ko'rib chalkashmasin.
	if strings.TrimSpace(reply) == "" && imageURL != "" {
		in.ChatReply = WithOrderSN(photoOnlyReply, showNums)
	}

	// Xodim yozgan raqam guruh xabaridagi buyurtmalardan boshqa bo'lsa —
	// xatoni xodimning o'zi darhol ko'rsin. Javob TO'SILMAYDI: raqamni
	// odam ataylab yozgan va u bilan hech qanday qidiruv ketmaydi, faqat
	// javob matnida chop etiladi.
	in.NumberNote = foreignNumberNote(numSN, sns)

	// Suhbat tarixi — til va kontekst uchun.
	msgs, err := fetchHistory(conversationID)
	if err != nil {
		log.Printf("xodim javobi: suhbat %d tarixini olib bo'lmadi: %v", conversationID, err)
	}
	in.ClientMessage = lastClientMessage(msgs)
	in.MessageIDs = JoinIDs(UnansweredClientIDs(msgs))

	// Faqat rasm yuborilgan bo'lsa LLM chaqirilmaydi: qayta yozadigan
	// matn yo'q, tayyor jumla ishlatiladi (token ham tejaladi).
	if strings.TrimSpace(reply) == "" && imageURL != "" {
		sendIfAuto(in, who)
		if err := SaveInteraction(DB, in); err != nil {
			return in, fmt.Errorf("bazaga yozish: %w", err)
		}
		log.Printf("xodim javobi: suhbat %d — faqat rasm, %s (%s)", conversationID, in.Status, who)
		return in, nil
	}

	// LLM bilan mijoz tiliga moslab yozamiz.
	usage, err := rewriteStaffReply(ctx, in, numSN, numEx, reply, msgs)
	if err != nil {
		in.Error = fmt.Sprintf("xodim javobini qayta yozib bo'lmadi: %v", err)
		log.Printf("xodim javobi: %v — xodim matni qoralama bo'lib qoldi", err)
		// XOM MATN MIJOZGA KETMAYDI. Xodim guruhda ichki tilda,
		// qisqa yozadi ("sotuvchi bilan gaplashilmoqda") — uni mijozga
		// o'sha holicha yuborib bo'lmaydi. Avto-javob yoqiq bo'lsa ham
		// qoralama panelda tasdiqlashni kutadi: admin tahrirlab
		// yuboradi. (Ilgari sendIfAuto shartsiz chaqirilar va model
		// ishlamaganda xom matn to'g'ridan-to'g'ri mijozga ketardi.)
		in.Status = StatusPending
	} else {
		in.applyUsage(usage)
		in.StepsCount = len(in.Steps)
		// Avto-javob yoqiq bo'lsa darhol mijozga.
		sendIfAuto(in, who)
	}

	if err := SaveInteraction(DB, in); err != nil {
		return in, fmt.Errorf("bazaga yozish: %w", err)
	}
	log.Printf("xodim javobi: suhbat %d — %s (%s)", conversationID, in.Status, who)
	return in, nil
}

// rewriteStaffReply - LLM chaqiruvi. Muvaffaqiyatli bo'lsa in.ChatReply
// mijozga mos matn bilan almashadi.
func rewriteStaffReply(ctx context.Context, in *Interaction, sns, express []string,
	reply string, msgs []Message) (Usage, error) {

	// "Faqat mutaxassis javoblari" rejimining butun MA'NOSI shu:
	// model aynan xodim javobini mijoz tiliga o'girish uchun ishlaydi.
	// Shuning uchun bu yo'l `agent_enabled` o'chirilgan bo'lsa ham
	// ochiq qoladi — aks holda rejim yoqilgan holda xodimning javobi
	// mijozga umuman yetmay qolardi.
	if !AgentEnabled() && !StaffOnlyMode() {
		return Usage{}, fmt.Errorf("AI agent o'chirilgan")
	}
	llm := ActiveLLM()
	if !llm.Ready() {
		return Usage{}, ErrNoLLMKey
	}

	p, err := GetPromt(DB, StaffPromtID())
	if err != nil {
		return Usage{}, fmt.Errorf("promt %d topilmadi", StaffPromtID())
	}

	// Modelga faqat MA'LUMOT beriladi, ko'rsatma emas: qaysi buyurtma,
	// qaysi til, xodim nima degani. Qanday yozish — bazadagi promtda
	// (prompt_flags.go dagi izohga qarang).
	//
	// Ichki holat matni ("status_label") ataylab yuborilmaydi — model uni
	// javobga ko'chirib, mijozga ichki atamalarni chiqarib yuborardi.
	info := map[string]any{
		"xodim_javobi": strings.TrimSpace(reply),
	}
	// Bo'sh maydon YUBORILMAYDI: "order_sn" bo'sh satr bo'lib ketsa,
	// model uni bajarishga urinib raqamni to'qib chiqarardi
	// ("buyurtmangiz (DG…)"). Maydon yo'qligi — "raqam noma'lum" degani,
	// bu qoida promtda yozilgan.
	if len(sns) > 0 {
		info["order_sn"] = sns
	}
	// Trek raqami ALOHIDA kalitda: `order_sn` ichiga qo'shilsa model uni
	// "buyurtma raqamingiz" deb yozib yuboradi.
	if len(express) > 0 {
		info["trek_raqami"] = express
	}
	// Mijozning tili — bazadan (client_lang.go). Bu yo'lda tilni hech
	// kim aniqlamaydi: model suhbat tarixiga qarab taxmin qilardi va
	// tarix oxirida bizning o'zbekcha xabarimiz tursa, rus tilida yozib
	// yurgan mijozga o'zbekcha javob ketardi.
	if lang := ClientLangJSON(in.ClientID, msgs); lang != "" {
		info["til"] = json.RawMessage(lang)
	}
	// Kunning birinchi javobi bo'lsa model javobni salom bilan boshlaydi
	// (greeting.go). Oxirgi qaror baribir yuborish paytida qabul
	// qilinadi — bu yerda faqat model matnni iliq boshlashi uchun.
	salom := needGreeting(in.ClientID, in.ConversationID, msgs)
	if salom {
		info["salom"] = true
	}
	raw, _ := json.MarshalIndent(info, "", "  ")

	var b strings.Builder
	b.WriteString("Suhbatning oxirgi xabarlari (eskisidan yangisiga). ")
	b.WriteString(`"type": "client" — mijoz yozgan, "type": "agent" — biz yozgan javob:` + "\n")
	b.WriteString(formatTranscript(msgs))
	b.WriteString("\n\nXodim javobi:\n")
	b.Write(raw)
	userMsg := b.String()

	out, usage, err := llm.Generate(ctx, p.Promt, userMsg)

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
	in.ChatReply = WithOrderSN(a.Chat, mergeNumbers(sns, express, 2*staffNumbersMax))
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

// WithOrderSN - javob matnida buyurtma (yoki trek) raqami borligini
// kafolatlaydi.
//
// Model (yoki xodim) raqamni yozmagan bo'lsa, matn oldiga qo'shiladi:
// mijoz javob qaysi buyurtmasi haqida ekanini bilishi kerak. Matnda
// allaqachon bor raqam takrorlanmaydi — "DG 60732205", "dg-60732205" va
// kirillcha "ДГ60732205" ham BOR deb hisoblanadi (containsNum).
//
// Raqam birinchi qator FAQAT SALOMDAN iborat bo'lsa, undan KEYIN
// qo'yiladi. Ikki sabab: mijozga "Assalomu alaykum! / DG… — matn" tabiiy
// o'qiladi, va yuborish paytidagi WithoutGreeting (greeting.go) "faqat
// salom" qatorini butunlay o'chiradi — raqam o'sha qatorda tursa, u bilan
// birga yo'qolib ketardi.
func WithOrderSN(text string, sns []string) string {
	text = strings.TrimSpace(text)
	if text == "" || len(sns) == 0 {
		return text
	}
	var missing []string
	for _, sn := range sns {
		if sn = strings.TrimSpace(sn); sn != "" && !containsNum(text, sn) {
			missing = append(missing, sn)
		}
	}
	if len(missing) == 0 {
		return text
	}
	prefix := strings.Join(missing, ", ") + " — "

	// Birinchi qator faqat salom bo'lsa — raqam undan keyin.
	if line, rest, ok := strings.Cut(text, "\n"); ok && isGreetingOnly(line) {
		return line + "\n" + prefix + strings.TrimLeft(rest, "\n")
	}
	return prefix + text
}

// staffNumbersMax - xodim javobidan olinadigan raqamlar chegarasi
// (zanjirdagi bilan bir xil).
const staffNumbersMax = 10

// effectiveNumbers - javobda mijozga ko'rsatiladigan raqamlar.
//
// Xodim javobida raqam yozgan bo'lsa — FAQAT o'shalar ishlatiladi: guruh
// xabari to'rt buyurtma haqida bo'lsa ham, xodim bittasini nomma-nom
// aytgan bo'lsa javob aynan o'sha buyurtma haqida. Raqam yozmagan bo'lsa
// eski qoida: guruh xabariga biriktirilgan raqamlar (`sns`).
//
// KeepMentioned bu yerda ataylab ISHLATILMAYDI: u model to'qigan raqamni
// suhbat tarixi bo'yicha filtrlaydi, xodim xabari esa tarixda yo'q —
// filtr xodim yozgan raqamni o'chirib tashlardi (aynan shu xato tufayli
// mijozga raqamsiz javob ketardi).
func effectiveNumbers(reply string, sns []string) (orderSN, express []string) {
	sn, ex := numbersFromText(reply)
	// Karta raqami trek bo'lib ketmasin: xodim pul qaytarish mavzusida
	// karta raqamini yozishi mumkin, u mijozga "trek raqamingiz" bo'lib
	// ko'rinmasligi kerak.
	clean := make([]string, 0, len(ex))
	for _, e := range ex {
		if !cardLike(e) {
			clean = append(clean, e)
		}
	}
	ex = clean

	if len(sn) == 0 && len(ex) == 0 {
		return mergeNumbers(sns, nil, staffNumbersMax), nil
	}
	return mergeNumbers(sn, nil, staffNumbersMax), mergeNumbers(ex, nil, staffNumbersMax)
}

// foreignNumberNote - xodim yozgan raqamlardan qaysilari guruh xabaridagi
// buyurtmalarga tegishli emasligi haqida qisqa eslatma (bo'sh satr —
// hammasi joyida yoki taqqoslashga asos yo'q).
func foreignNumberNote(staffSN, sns []string) string {
	if len(sns) == 0 || len(staffSN) == 0 {
		return ""
	}
	var foreign []string
	for _, sn := range staffSN {
		known := false
		for _, s := range sns {
			if containsNum(s, sn) {
				known = true
				break
			}
		}
		if !known {
			foreign = append(foreign, sn)
		}
	}
	if len(foreign) == 0 {
		return ""
	}
	return "ℹ️ " + strings.Join(foreign, ", ") + " — bu xabardagi buyurtma emas, " +
		"lekin javobda shu raqam ko'rsatiladi."
}
