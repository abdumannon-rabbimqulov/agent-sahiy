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
)

// Zanjir sozlamalari (.env).
const (
	DefaultStartPromtID   = 1
	DefaultMaxSteps       = 5
	DefaultHistoryLimit   = 10
	DefaultOrdersPerCall  = 20
	DefaultClientQuietSec = 20
)

// StartPromtID - zanjir qaysi promtdan boshlanadi.
func StartPromtID() uint { return uint(envInt("START_PROMPT_ID", DefaultStartPromtID)) }

// MaxSteps - eng ko'p necha bosqich.
func MaxSteps() int { return envInt("AGENT_MAX_STEPS", DefaultMaxSteps) }

// ClientQuietSec - mijozning oxirgi xabaridan beri kamida shuncha soniya
// o'tmaguncha zanjir ishga tushmaydi (.env: CLIENT_QUIET_SEC).
//
// Sabab: mijoz ko'pincha bir fikrni bir necha qisqa xabarga bo'lib yozadi
// (har gap — alohida xabar). Shu tekshiruvsiz har bir xabar alohida to'liq
// zanjirni (bir necha Groq chaqiruvi) ishga tushirar edi — mijoz yozib
// bo'lmasdanoq. 0 — o'chirilgan (tekshiruv qilinmaydi).
func ClientQuietSec() int { return envInt("CLIENT_QUIET_SEC", DefaultClientQuietSec) }

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

// ErrAlreadyAnswered - suhbatdagi oxirgi so'z biz tomondan aytilgan,
// ya'ni mijoz javobsiz qolmagan.
var ErrAlreadyAnswered = errors.New("suhbatga javob berilgan — yangi mijoz xabari yo'q")

// ErrClientStillTyping - mijozning oxirgi xabaridan beri hali
// CLIENT_QUIET_SEC soniya o'tmagan — u ketma-ket yana yozayotgan bo'lishi
// mumkin. Zanjir hozircha ishga tushmaydi (keyingi poll siklida qayta
// tekshiriladi), token behuda ketmasin.
var ErrClientStillTyping = errors.New("mijoz hali yozib tugatmagan bo'lishi mumkin — kutilmoqda")

// RunChain bitta suhbat uchun zanjirni yuritadi va natijani bazaga yozadi.
// Xato bo'lsa ham interaksiya saqlanadi (status=failed) — panelda ko'rinadi.
func RunChain(ctx context.Context, conversationID, clientID int64) (*Interaction, error) {
	return runChain(ctx, conversationID, clientID, false)
}

// RunChainForce - tekshiruvsiz ishga tushirish (paneldan qo'lda qayta
// urinish uchun): oxirgi so'z biz tomondan bo'lsa ham zanjir yuradi.
func RunChainForce(ctx context.Context, conversationID, clientID int64) (*Interaction, error) {
	return runChain(ctx, conversationID, clientID, true)
}

func runChain(ctx context.Context, conversationID, clientID int64, force bool) (*Interaction, error) {
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
		Forced:         force,
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

	// Oxirgi so'z biz tomondan bo'lsa — mijoz javob kutmayapti.
	// Bunday suhbatni qayta ishlash behuda token va takroriy javob
	// (hatto muammo sifatida qayta ko'tarilishi) demakdir.
	if !force && !msgs[len(msgs)-1].FromClient() {
		return nil, ErrAlreadyAnswered
	}
	// Mijoz ketma-ket bir necha qisqa xabar yozishi odatiy holat (har gap
	// alohida xabar). Oxirgi xabardan beri hali "jim turish oralig'i"
	// o'tmagan bo'lsa — u hali yozib tugatmagan bo'lishi mumkin, zanjirni
	// shu daqiqada ishga tushirib, keyingi xabari uchun yana bir marta
	// (yana AGENT_MAX_STEPS gacha Groq chaqiruvi bilan) qaytadan ishlashdan
	// qochamiz. `force` bu tekshiruvni chetlab o'tadi — qo'lda ishga
	// tushirish har doim darhol ishlashi kerak.
	if !force {
		if t, ok := parseAnyTime(msgs[len(msgs)-1].CreatedAt); ok {
			if quiet := ClientQuietSec(); quiet > 0 && time.Since(t) < time.Duration(quiet)*time.Second {
				return nil, ErrClientStillTyping
			}
		}
	}
	// Mijoz yana yozgan bo'lsa, shu suhbat uchun hali tasdiqlanmagan
	// (pending) eski javoblar ENDI ESKIRGAN — ular yangi xabarni hisobga
	// olmagan holda yozilgan edi. Admin ularni tasodifan tasdiqlab
	// yubormasligi uchun bekor qilamiz; hozir tayyorlanayotgan yangi
	// javob ularning o'rnini bosadi.
	supersedePending(conversationID)
	in.ClientMessage = lastClientMessage(msgs)
	// Aynan shu murojaatda javob berilayotgan (javobsiz qolgan) mijoz
	// xabarlari — javob yuborilgandan keyin shular o'qilgan deb belgilanadi.
	in.MessageIDs = JoinIDs(UnansweredClientIDs(msgs))
	transcript := formatTranscript(msgs)
	// Bugun bu suhbatda biz hali yozmaganmiz — javob salom bilan boshlanadi.
	greet := NeedsGreeting(msgs)
	// Mijoz yozgan raqamlar — model ularni tashlab ketsa ham qidiruv
	// baribir shu raqamlar bo'yicha ketadi.
	chatSN, chatEx := ExtractNumbers(msgs)

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
		// probed - "tushunmadim" fallback'i bir marta ishlagan.
		probed bool
	)

	// Xayrlashish: mijozning oxirgi so'zi "rahmat" / "hop" bo'lsa,
	// savol yo'q — modelga bormaymiz, tayyor matn bilan chiroyli
	// xayrlashamiz (farewell.go). maxSteps = 0 — zanjir yurmaydi,
	// token sarflanmaydi.
	if reply, ok := Farewell(msgs); ok {
		in.ChatReply = reply
		in.Status = StatusPending // avto-javob yoqilgan bo'lsa quyida yuboriladi
		maxSteps = 0
		// Panelda "nega 0 bosqich" ko'rinib tursin.
		in.Steps = append(in.Steps, AgentStep{
			StepNo:      1,
			PromtTitle:  "Xayrlashish (model chaqirilmadi)",
			RawResponse: reply,
			CreatedAt:   time.Now(),
		})
		log.Printf("agent: suhbat %d — mijoz minnatdorchilik bildirdi, xayrlashildi", conversationID)
	}

	// Rasm: mijoz raqamni yozmay, skrinshot yoki chek tashlagan bo'lishi
	// mumkin. Asosiy model rasmni ko'rmaydi ("[rasm yuborildi]"), shuning
	// uchun rasmni tesseract OCR o'qiydi (image_numbers.go) — modelsiz,
	// tokensiz.
	//
	// Faqat matnda raqam TOPILMAGANDA ochiladi: matnda raqam bo'lsa rasm
	// ortiqcha ish, javob baribir o'sha raqam bo'yicha yoziladi.
	if maxSteps > 0 && len(chatSN) == 0 && len(chatEx) == 0 && HasClientImage(msgs) {
		img, ok := ReadNumbersFromMessages(ctx, msgs)

		var natija string
		if ok {
			// Raqam topildi — birinchi promtga shu raqamlar bilan kiramiz.
			chatSN = mergeNumbers(chatSN, img.OrderSN, 10)
			chatEx = mergeNumbers(chatEx, img.Express, 10)
			dataCtx = append(dataCtx, "Mijoz yuborgan rasmdan o'qilgan raqamlar: "+
				strings.Join(img.All(), ", ")+
				". Mijoz shu buyurtma haqida yozmoqda — raqamni qaytadan so'rama.")
			natija = "TOPILDI: " + strings.Join(img.All(), ", ")
			log.Printf("agent: suhbat %d — rasmdan raqam topildi: %v", conversationID, img.All())
		} else {
			// Raqam chiqmadi (rasmda yo'q yoki o'qilmadi) — model buni
			// bilsin va raqamni mijozdan so'rasin. Zanjir to'xtamaydi.
			dataCtx = append(dataCtx, imageNoNumberHint)
			natija = "RASMDAN BUYURTMA RAQAMI CHIQMADI — raqam mijozdan so'raladi"
			log.Printf("agent: suhbat %d — rasmdan buyurtma raqami chiqmadi", conversationID)
		}

		// Bosqich panelga yoziladi: suhbat tafsilotida qaysi rasm
		// o'qilgani, OCR nima chiqargani va natija ochiq tursin.
		in.Steps = append(in.Steps, AgentStep{
			PromtTitle:     "Rasmni o'qish — " + imageReader(img),
			RequestContext: imageStepContext(img),
			RawResponse:    imageStepResult(img, natija),
			CreatedAt:      time.Now(),
		})
	}

	for step := 1; step <= maxSteps; step++ {
		p, err := GetPromt(DB, promtID)
		if err != nil {
			in.Error = fmt.Sprintf("promt %d topilmadi", promtID)
			break
		}

		userMsg := buildUserMessage(transcript, dataCtx, greet)
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
		// `pending` — mijozning hali kelmagan buyurtmasi topildimi.
		fetched, pending := false, false
		if a.NeedsData() {
			// Model qaytargan raqamlarga suhbatdan topilganlarini
			// qo'shamiz — eski buyurtma ham topilsin.
			a.OrderSN = mergeNumbers(a.OrderSN, chatSN, 10)
			a.ExpressNum = mergeNumbers(a.ExpressNum, chatEx, 10)
			data, p := fetchSystemData(a, clientID, conversationID)
			dataCtx = append(dataCtx, data)
			fetched, pending = true, p
		}

		// Model mijoz muammosini tushunmadi. Darhol "buyurtma raqamini
		// bering" deb so'ramaymiz: avval KOD adminka va dashboardni o'zi
		// ko'radi. Kelmagan buyurtma bo'lsa — o'sha ma'lumot bilan model
		// qaytadan chaqiriladi (mijoz katta ehtimol o'sha buyurtma
		// haqida yozgan). Bo'lmasa — javob o'z holicha qoladi, ya'ni
		// mijozdan buyurtma raqami so'raladi.
		//
		// Bir marta qilinadi: ikkinchi marta ham tushunmasa, so'rash eng
		// to'g'ri yo'l. Oxirgi bosqichda ham qilinmaydi — qayta so'rashga
		// bosqich qolmaydi.
		if a.IsUnclear() && !probed && step < maxSteps {
			probed = true
			if !fetched {
				// Raqamsiz so'rov — mijozning hamma buyurtmasi olinadi.
				probe := AgentJSON{Adminka: true, Dashboard: true,
					OrderSN: chatSN, ExpressNum: chatEx}
				data, p := fetchSystemData(probe, clientID, conversationID)
				pending = p
				if pending {
					dataCtx = append(dataCtx, data)
				}
			}
			if pending {
				dataCtx = append(dataCtx, unclearHint)
				log.Printf("agent: suhbat %d — model tushunmadi, kelmagan buyurtma topildi: qayta so'raldi", conversationID)
				continue // xuddi shu promt, endi ma'lumot bilan
			}
			log.Printf("agent: suhbat %d — model tushunmadi, kelmagan buyurtma yo'q: buyurtma raqami so'raladi", conversationID)
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

	// Bosqich raqamlari ketma-ket bo'lsin: rasm bosqichi zanjirdan
	// oldin turadi, shuning uchun raqamlar oxirida qo'yiladi.
	for i := range in.Steps {
		in.Steps[i].StepNo = i + 1
	}
	in.StepsCount = len(in.Steps)
	in.applyUsage(usage)

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
		if !sendIfAuto(in, "avto") {
			in.Status = StatusPending
		}

	default:
		// Faqat help bor edi — mijozga yoziladigan narsa yo'q, ya'ni
		// tasdiqlashga ham hojat yo'q.
		if in.HelpSent {
			in.markSent("avto")
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

	// Javob berildi — suhbat "hal qilindi" holatiga o'tkaziladi.
	// Mijoz yana yozsa, support tizimining o'zi uni qayta ochadi.
	if AutoResolveOn() && !in.ChatResolved {
		if err := ResolveChat(in.ConversationID); err != nil {
			// Javob ketgan — yopilmagani murojaatni buzmaydi.
			log.Printf("agent: suhbat %d yopilmadi: %v", in.ConversationID, err)
		} else {
			in.ChatResolved = true
			saveFlag(in, "chat_resolved", true)
			log.Printf("agent: suhbat %d — hal qilindi deb belgilandi", in.ConversationID)
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
	return withToken(func(baseURL, token string) ([]Message, error) {
		return FetchMessages(baseURL, token, conversationID, HistoryLimit())
	})
}

// unclearHint - "tushunmadim" fallback'ida ma'lumot bilan birga
// ketadigan ko'rsatma: mijoz aynan shu buyurtmalar haqida yozgan
// bo'lishi ehtimoli katta.
const unclearHint = "Sen mijoz muammosini tushunmading, shuning uchun " +
	"uning buyurtmalari tizimdan olindi. Yuqorida mijozning HALI KELMAGAN " +
	"buyurtmalari bor — mijoz katta ehtimol o'shalar haqida yozgan. " +
	"Shu ma'lumotga tayanib javob yoz; endi \"" + AskHelpText + "\" deb so'rama (boshqa tilda ham)."

// imageNoNumberHint - mijoz rasm yubordi, lekin undan buyurtma yoki trek
// raqami chiqmadi. Model buni bilmasa "rasmingizni ko'rdim" deb noto'g'ri
// javob yozib yuborishi mumkin.
const imageNoNumberHint = "Mijoz rasm yubordi, lekin RASMDAN BUYURTMA RAQAMI CHIQMADI " +
	"(rasm OCR bilan o'qildi). Rasm mazmuniga tayanma — uni ko'ra olmaysan. " +
	"Buyurtma boshqa yo'l bilan aniqlanmasa, mijozdan buyurtma (DG…) yoki " +
	"trek raqamini yozishini xushmuomala so'ra."

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
		// Mijoz rasm yuborsa, xabar matni — havola. Model rasmni ko'ra
		// olmaydi, uzun havola esa faqat token yeydi. Shuning uchun
		// tushunarli belgi bilan almashtiramiz.
		if isImageLink(text) {
			text = "[rasm yuborildi]"
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

// imageExts - rasm havolasini aniqlash uchun.
var imageExts = []string{".jpg", ".jpeg", ".png", ".webp", ".heic", ".gif"}

// isImageLink - xabar butunlay rasm havolasidan iboratmi.
func isImageLink(s string) bool {
	if !strings.HasPrefix(s, "http") || strings.ContainsAny(s, " \n") {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "chat-images") {
		return true
	}
	for _, e := range imageExts {
		if strings.Contains(low, e) {
			return true
		}
	}
	return false
}

// buildUserMessage modelga ketadigan matn: suhbat + tizimdan olingan
// ma'lumot + salomlashish ko'rsatmasi.
//
// Til haqida ko'rsatma bu yerda QO'SHILMAYDI: uni promtning o'zi
// aytadi. Ilgari kod alifboni o'zi aniqlab qo'shib yuborardi, lekin
// oxirgi xabar rasm bo'lsa ("[rasm yuborildi]") noto'g'ri til
// tanlanardi — masalan ruscha yozgan mijozga "lotin alifboda yoz"
// degan ko'rsatma ketardi.
func buildUserMessage(transcript string, data []string, greet bool) string {
	var b strings.Builder
	b.WriteString("Suhbatning oxirgi xabarlari (eskisidan yangisiga). ")
	b.WriteString(`"type": "client" — mijoz yozgan, "type": "agent" — biz yozgan javob:` + "\n")
	b.WriteString(transcript)
	if len(data) > 0 {
		b.WriteString("\n\nTizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):\n")
		b.WriteString(strings.Join(data, "\n"))
	}
	// Salom kuniga bir marta: yangi kunning birinchi javobi salom bilan
	// boshlanadi, kun davomidagi keyingi javoblarda takrorlanmaydi.
	//
	// Salom — javobning BOSHI, o'zi emas: model baribir mijoz muammosini
	// hal qilishi kerak, tushunmasa esa so'rashi kerak.
	b.WriteString("\n\n")
	if greet {
		b.WriteString("Bugun bu suhbatda biz hali yozmadik — chat javobini salom bilan boshla. ")
		b.WriteString("Salomni MIJOZNING tilida yoz: o'zbekcha lotin — \"" + GreetingText + "\", ")
		b.WriteString("o'zbekcha kirill — \"" + GreetingUzCyr + "\", rus tilida — \"" + GreetingRU + "\". ")
		b.WriteString("Faqat shu ikki so'z, boshqa salomlashish qo'shma. ")
		b.WriteString("Salom — javobning boshi, o'zi emas: undan keyin mijoz muammosiga javob yoz. ")
	} else {
		b.WriteString("Bugun bu suhbatda allaqachon yozganmiz — javobda salomlashma, to'g'ridan-to'g'ri mavzuga o't. ")
	}
	b.WriteString("Avval yuqoridagi xabarlarning HAMMASINI o'qib, mijozning muammosini o'zing aniqla: ")
	b.WriteString("mijoz muammosini oxirgi xabarda emas, oldingi xabarlarida aytgan bo'lishi mumkin. ")
	b.WriteString("Muammo tushunarli bo'lsa to'g'ridan-to'g'ri javob yoz. ")
	b.WriteString("Shu xabarlarning hech biridan muammo tushunilmasa — o'zingdan to'qima, ")
	b.WriteString(`javobingga "tushunmadim": true qo'sh va chat matnida shu savolni MIJOZNING tilida so'ra: `)
	b.WriteString(`o'zbekcha lotin — "` + AskHelpText + `", o'zbekcha kirill — "` + AskHelpUzCyr + `", rus tilida — "` + AskHelpRU + `". `)
	b.WriteString("Bunda kod mijozning buyurtmalarini tizimdan olib senga qaytadan beradi: ")
	b.WriteString("kelmagan buyurtmasi bo'lsa, savol o'rniga o'sha buyurtma haqida javob yozasan.")
	return b.String()
}

// imageReader - rasmni nima o'qigani (panel sarlavhasi uchun).
func imageReader(img ImageNumbers) string {
	if img.Model != "" {
		return img.Model
	}
	return "tesseract " + OCRLangs()
}

// imageStepContext - panelda "nima qilindi" bo'limi: qaysi rasmlar
// o'qilgani va nima bilan.
func imageStepContext(img ImageNumbers) string {
	var b strings.Builder
	b.WriteString("Mijoz rasm yubordi, matnda buyurtma raqami yo'q edi — rasm o'qildi.\n")
	b.WriteString("O'qigan: " + imageReader(img) + " (OCR, modelsiz — token sarflanmaydi)\n\n")
	if len(img.Links) == 0 {
		b.WriteString("Rasm: —\n")
	}
	for i, link := range img.Links {
		b.WriteString(fmt.Sprintf("%d-rasm: %s\n", i+1, link))
	}
	return b.String()
}

// imageStepResult - panelda "natija" bo'limi: OCR ning xom matni va
// undan chiqarilgan xulosa.
func imageStepResult(img ImageNumbers, natija string) string {
	var b strings.Builder
	b.WriteString("Natija: " + natija + "\n")
	b.WriteString(fmt.Sprintf("O'qilgan rasm: %d ta\n", img.Images))
	if img.Text != "" {
		b.WriteString("Rasmdagi matn: " + img.Text + "\n")
	}
	if len(img.OrderSN) > 0 {
		b.WriteString("Buyurtma raqami: " + strings.Join(img.OrderSN, ", ") + "\n")
	}
	if len(img.Express) > 0 {
		b.WriteString("Trek raqami: " + strings.Join(img.Express, ", ") + "\n")
	}
	if img.Raw != "" {
		b.WriteString("\nXom natija (" + imageReader(img) + "):\n" + img.Raw + "\n")
	}
	return b.String()
}

// fetchSystemData model so'ragan manbalardan ma'lumot oladi va JSON matn
// qilib qaytaradi.
//
// Modelga XOM javob berilmaydi: bitta buyurtma ~10 KB, undan javob yozish
// uchun 6-7 maydon kerak. Shu yerda saralanadi (context.go) — token ham
// tejaladi, model ham chalkashmaydi.
func fetchSystemData(a AgentJSON, clientID, conversationID int64) (string, bool) {
	out := map[string]any{}
	numbers := a.Numbers()
	// pending - mijozning hali kelmagan (yakunlanmagan) buyurtmasi
	// topildimi. Model muammoni tushunmaganda shu bo'yicha qaror
	// qilinadi: bor bo'lsa — modelga qaytadan beriladi, yo'q bo'lsa —
	// mijozdan buyurtma raqami so'raladi.
	pending := false

	if a.Adminka {
		adm := AdminkaFromEnv()
		var rows []AdminkaOrder
		var errs []string
		if len(numbers) == 0 {
			r, err := FetchOrders(adm, OrderFilter{UserID: clientID, Size: DefaultOrdersPerCall})
			rows, errs = appendResult(rows, errs, r, err)
		}
		for _, n := range numbers {
			r, err := findOrderByNumber(adm, n)
			rows, errs = appendResult(rows, errs, r, err)
		}

		rows = dedupOrders(rows)
		// Mijoz turi (B2C/B2B) — yetkazish tarifini tushuntirish uchun.
		out["mijoz_turi"] = CustomerType(rows)
		// Muammoli buyurtmalarni aniqlash (kerak bo'lsa guruhga xabar ketadi).
		views := DetectIssues(rows, clientID, conversationID)
		out["adminka"] = BriefOrders(views)
		if HasPendingOrders(views) {
			pending = true
		}
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
				rows, errs = appendResult(rows, errs, r, err)
			}
			for _, n := range tracks {
				r, err := fetchDeliveryRetry(svc, token, DeliveryFilter{TrackNumber: n, Size: DefaultOrdersPerCall})
				rows, errs = appendResult(rows, errs, r, err)
			}
			brief := BriefDelivery(rows)
			out["yetkazma"] = brief
			// Mijozning qo'liga tegmagan yetkazmasi: filialda kutayotgani,
			// yo'ldagisi va holati noaniq bo'lgani — uchalasi ham
			// "hali olinmagan" hisoblanadi.
			if len(brief.Pending) > 0 || len(brief.InDelivery) > 0 || len(brief.NeedCheck) > 0 {
				pending = true
			}
			if len(errs) > 0 {
				out["dashboard_error"] = strings.Join(errs, "; ")
			}
		}
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), pending
	}
	return string(raw), pending
}

// HasPendingOrders - mijozda hali kelmagan (yakunlanmagan) buyurtma bormi.
//
// Yakunlangan (status 6) buyurtma "kelgan" hisoblanadi; to'lanmagani ham
// hisobga olinmaydi — u hali yo'lga chiqmagan, muammo emas.
func HasPendingOrders(views []OrderView) bool {
	for _, v := range views {
		if v.Paid && v.Status != StatusFinished {
			return true
		}
	}
	return false
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

// findOrderByNumber - buyurtmani raqami bo'yicha qidiradi.
//
// Avval aniq maydon bo'yicha (DG… → order_sn, qolgani → express_num),
// natija bo'lmasa `keyword` bilan qayta uriniladi: eski buyurtmalarda
// trek boshqa maydonda saqlangan bo'lishi mumkin. Buyurtma yoshi
// ahamiyatsiz — qidiruv butun baza bo'yicha ketadi.
func findOrderByNumber(adm Adminka, n string) ([]AdminkaOrder, error) {
	f := OrderFilter{Size: DefaultOrdersPerCall}
	if strings.HasPrefix(strings.ToUpper(n), "DG") {
		f.OrderSN = n
	} else {
		f.ExpressNum = n
	}
	rows, err := FetchOrders(adm, f)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	return FetchOrders(adm, OrderFilter{Keyword: n, Size: DefaultOrdersPerCall})
}

// dedupOrders - bir buyurtma bir necha marta tushib qolmasin (raqamlar
// bo'yicha alohida so'rovlar bir xil buyurtmani qaytarishi mumkin).
func dedupOrders(rows []AdminkaOrder) []AdminkaOrder {
	seen := map[string]bool{}
	out := make([]AdminkaOrder, 0, len(rows))
	for _, o := range rows {
		if o.OrderSN != "" && seen[o.OrderSN] {
			continue
		}
		seen[o.OrderSN] = true
		out = append(out, o)
	}
	return out
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

// appendResult - so'rov natijasini yig'ib boradi: xato bo'lsa matni
// errs ro'yxatiga tushadi, aks holda satrlar qo'shiladi. Bitta manba
// ishlamasa ham qolganlari modelga yetib borsin.
func appendResult[T any](rows []T, errs []string, r []T, err error) ([]T, []string) {
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

// supersedePending - shu suhbat uchun hali tasdiqlanmagan (pending)
// eski javoblarni "rejected" deb belgilaydi. Mijoz yangi xabar yozgach
// eski javob suhbat holatini aks ettirmay qoladi — admin panelda
// tasdiqlash navbatida qolib, keyin tasodifan (eskirgan holda)
// yuborilishining oldi olinadi. Yozuv o'chirilmaydi — tarixda
// "rejected" bo'lib ko'rinib turadi, faqat navbatdan chiqadi.
func supersedePending(conversationID int64) {
	if DB == nil {
		return
	}
	res := DB.Model(&Interaction{}).
		Where("conversation_id = ? AND status = ?", conversationID, StatusPending).
		Updates(map[string]any{
			"status": StatusRejected,
			"error":  "mijoz yangi xabar yozdi — eski javob eskirdi",
		})
	if res.Error != nil {
		log.Printf("agent: suhbat %d — eski javoblarni bekor qilish: %v", conversationID, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("agent: suhbat %d — %d ta eski (tasdiqlanmagan) javob bekor qilindi",
			conversationID, res.RowsAffected)
	}
}

// lastClientMessage - suhbatdagi eng oxirgi mijoz xabari matni.
func lastClientMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() {
			return msgs[i].Message
		}
	}
	return ""
}

// sendIfAuto - avto-javob yoqiq bo'lsa javobni darhol mijozga yuboradi
// va holatni yangilaydi. O'chiq bo'lsa false qaytaradi: javob admin
// tasdig'ini kutadi.
func sendIfAuto(in *Interaction, handledBy string) bool {
	if !AutoReplyOn() {
		return false
	}
	if err := DeliverChat(in); err != nil {
		in.Status = StatusFailed
		in.Error = err.Error()
	} else {
		in.markSent(handledBy)
	}
	return true
}
