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

// ErrAnsweredByStaff - javob tayyorlangandan keyin, u mijozga
// yetib bormasidan oldin suhbatga BIZ tomondan (xodim yoki boshqa
// murojaat) javob yozilgan. Tayyor javob endi eskirgan — yuborilmaydi.
var ErrAnsweredByStaff = errors.New("suhbatga biz tomondan javob berilgan — tayyor javob yuborilmadi")

// ErrStaffOnly - "faqat mutaxassis javoblari" rejimi yoqilgan: AI
// suhbatlarga o'zi kirmaydi, model faqat xodim javobini mijoz tiliga
// o'girish uchun ishlatiladi (settings.go: StaffOnlyMode).
var ErrStaffOnly = errors.New("faqat mutaxassis javoblari rejimi — AI suhbatga kirmaydi")

// ErrAlreadyStudied - shu mijoz xabari allaqachon o'rganilgan (zanjir
// bir marta yurgan). Mijoz yangi xabar yozmaguncha qayta o'rganilmaydi.
var ErrAlreadyStudied = errors.New("bu mijoz xabari allaqachon o'rganilgan — yangi xabar yo'q")

// alreadyStudied - shu suhbatning `lastID` xabari (yoki undan yangisi)
// ilgari ishlangan bo'lsa true.
//
// Nega kerak: muammo xodimlar guruhiga topshirilganda mijozga hech
// narsa yozilmaydi — suhbatdagi oxirgi so'z MIJOZNIKI bo'lib qolaveradi.
// Faqat "oxirgi so'z kimniki" tekshiruvi bunday suhbatni har safar
// yangidek ko'rsatardi: qo'lda skanerlash (ScanOnce) uni qayta-qayta
// modelga berib, o'sha muammo guruhga takror tushardi. Mijoz yangi
// xabar yozsa id o'sadi va zanjir odatdagidek yuradi.
func alreadyStudied(conversationID, lastID int64) bool {
	if DB == nil || lastID <= 0 {
		return false
	}
	var st ConversationState
	if err := DB.First(&st, "conversation_id = ?", conversationID).Error; err != nil {
		return false // yozuv yo'q — birinchi marta ko'rilmoqda
	}
	return lastID <= st.LastMessageID
}

// alreadyAnswered - suhbatdagi oxirgi so'z BIZNIKI bo'lsa true.
//
// Zanjir boshida ham shunday tekshiruv bor (ErrAlreadyAnswered), lekin
// u yetarli emas: zanjir model javobini kutib turgan (yoki javob admin
// tasdig'ini kutib turgan) paytda xodim mijozga o'zi yozib qo'yishi
// mumkin. Shu sababli tekshiruv yuborishdan OLDIN takrorlanadi.
//
// Xabarlarni olishda xato bo'lsa false qaytadi: aloqa uzilgani uchun
// tayyor javobni ushlab qolmaymiz.
//
// Ikkinchi qiymat — olingan tarix: salom tekshiruvi (greeting.go) ham
// shu xabarlarga tayanadi, ikkinchi so'rov kerak bo'lmasin.
func alreadyAnswered(conversationID int64) (bool, []Message) {
	msgs, err := fetchHistory(conversationID)
	if err != nil {
		log.Printf("agent: suhbat %d — yuborishdan oldingi tekshiruv o'tmadi: %v", conversationID, err)
		return false, nil
	}
	return lastWordOurs(msgs), msgs
}

// lastWordOurs - suhbatdagi oxirgi xabar biz tomondanmi.
func lastWordOurs(msgs []Message) bool {
	if len(msgs) == 0 {
		return false
	}
	return !msgs[len(msgs)-1].FromClient()
}

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
	// "Faqat mutaxassis javoblari" rejimi: yangi mijoz xabari modelga
	// bormaydi. `force` — admin paneldan ataylab ishga tushirgani,
	// u ishlayveradi.
	if !force && StaffOnlyMode() {
		return nil, ErrStaffOnly
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
	// Mijoz yangi hech narsa yozmagan bo'lsa (shu xabar allaqachon
	// o'rganilgan) — model chaqirilmaydi. Muammo xodimlar guruhida
	// turgan bo'lsa ham shunday: javobni xodim beradi, AI uni qayta
	// o'rganmaydi.
	if !force && alreadyStudied(conversationID, msgs[len(msgs)-1].ID) {
		return nil, ErrAlreadyStudied
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
	// Mijoz yozgan raqamlar — model ularni tashlab ketsa ham qidiruv
	// baribir shu raqamlar bo'yicha ketadi.
	chatSN, chatEx := ExtractNumbers(msgs)

	// 2. Zanjir.
	llm := ActiveLLM()
	if !llm.Ready() {
		in.Error = ErrNoLLMKey.Error()
		saveOrLog(in)
		return in, ErrNoLLMKey
	}

	var (
		usage    Usage
		dataCtx  []string      // oldingi bosqichlarda yig'ilgan tizim ma'lumoti
		alerts   []string      // kod topgan holatlar (viloyat mos emas va h.k.) — xodimga
		issues   []*OrderIssue // shu zanjirda yangi ochilgan muammoli buyurtmalar
		langCtx  string        // birinchi promtdan chiqqan til ("uzb"/"rus"), bir marta uzatiladi
		promtID  = StartPromtID()
		maxSteps = MaxSteps()
	)

	// Salom: shu mijozga bugun birinchi javobimiz bo'lsa, model javobni
	// salom bilan boshlashi kerak (greeting.go). Oxirgi qaror yuborish
	// paytida qabul qilinadi (deliverChat) — bu faqat ton uchun ko'rsatma.
	if needGreeting(clientID, conversationID, msgs) {
		dataCtx = append(dataCtx, greetingGuidance)
	}

	// Bekor qilish / pul qaytarish so'rovi: modelga qat'iy taqiq
	// beriladi va murojaat xodimlar guruhiga chiqadi (cancel.go).
	cancelAsk := WantsCancel(msgs)
	if cancelAsk {
		dataCtx = append(dataCtx, cancelGuidance)
		alerts = append(alerts, cancelAlert)
		log.Printf("agent: suhbat %d — mijoz bekor qilish/pul qaytarish so'radi, xodimga topshirildi",
			conversationID)
	}

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
	if maxSteps > 0 && HasClientImage(msgs) && (len(chatSN) > 0 || len(chatEx) > 0) {
		// Matnda raqam bor — rasmni o'qish ortiqcha. Lekin bu ham
		// panelda ko'rinib tursin: ilgari rasm haqida hech qayerda
		// hech narsa yozilmasdi va "rasm o'qildimi yoki yo'qmi" degan
		// savolga javob topib bo'lmasdi.
		nums := strings.Join(mergeNumbers(chatSN, chatEx, 10), ", ")
		in.Steps = append(in.Steps, AgentStep{
			PromtTitle:     "Rasm o'qilmadi — matnda raqam bor",
			RequestContext: fmt.Sprintf("Mijoz rasm yubordi (%d ta)", len(ClientImageLinks(msgs))),
			RawResponse:    "Matndagi raqam(lar): " + nums,
			CreatedAt:      time.Now(),
		})
		log.Printf("agent: suhbat %d — rasm o'qilmadi, matnda raqam bor: %s", conversationID, nums)
	}

	if maxSteps > 0 && len(chatSN) == 0 && len(chatEx) == 0 && HasClientImage(msgs) {
		img, ok := ReadNumbersFromMessages(ctx, msgs)

		var natija string
		if ok {
			// Raqam topildi — birinchi promtga shu raqamlar bilan kiramiz.
			chatSN = mergeNumbers(chatSN, img.OrderSN, 10)
			chatEx = mergeNumbers(chatEx, img.Express, 10)
			in.NumbersFromImage = true
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
			in.ImageNoNumber = true
			log.Printf("agent: suhbat %d — rasmdan buyurtma raqami chiqmadi", conversationID)

			// OCR tushunmadi (past sifat, burchak, boshqa format va h.k.) —
			// rasmni xodim o'z ko'zi bilan ko'rsin. Eng oxirgi (ko'pi bilan
			// 3 ta) rasm guruhga yuboriladi.
			sendUnreadableImages(conversationID, clientID, img)
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

		userMsg := buildUserMessage(transcript, dataCtx)
		raw, u, err := llm.Generate(ctx, p.Promt, userMsg)
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

		// Til birinchi promtdan chiqadi ("uzb"/"rus") — bir marta olinib,
		// keyingi HAMMA bosqichga dataCtx orqali uzatiladi.
		if langCtx == "" && a.HasLanguage() {
			lang, _ := json.Marshal(map[string]bool{"uzb": a.Uzb, "rus": a.Rus})
			langCtx = string(lang)
			dataCtx = append(dataCtx, "Til: "+langCtx)
		}

		// Kod tizimdan ma'lumot oladi va keyingi bosqichga beradi.
		if a.NeedsData() {
			// Model to'qib chiqargan raqam tashlanadi: qidiruv butun
			// adminka bazasidan ketadi va bunday raqam BOSHQA odamning
			// buyurtmasiga tushib, hech kim so'ramagan muammo ochilib
			// ketardi (numbers.go: KeepMentioned).
			if kept := KeepMentioned(a.OrderSN, msgs, chatSN); len(kept) != len(a.OrderSN) {
				log.Printf("agent: suhbat %d — model to'qigan buyurtma raqami tashlandi: %v → %v",
					conversationID, a.OrderSN, kept)
				a.OrderSN = kept
			}
			if kept := KeepMentioned(a.ExpressNum, msgs, chatEx); len(kept) != len(a.ExpressNum) {
				log.Printf("agent: suhbat %d — model to'qigan trek raqami tashlandi: %v → %v",
					conversationID, a.ExpressNum, kept)
				a.ExpressNum = kept
			}
			// Model qaytargan raqamlarga suhbatdan topilganlarini
			// qo'shamiz — eski buyurtma ham topilsin.
			a.OrderSN = mergeNumbers(a.OrderSN, chatSN, 10)
			a.ExpressNum = mergeNumbers(a.ExpressNum, chatEx, 10)
			data, _, found, fresh := fetchSystemData(a, clientID, conversationID)
			dataCtx = append(dataCtx, data)
			alerts = append(alerts, found...)
			issues = append(issues, fresh...)
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

	// Taqiqqa qaramay model javobida bekor qilish haqida yozgan bo'lsa,
	// javob mijozga AVTOMATIK ketmaydi: avval admin o'qib chiqsin.
	holdForAdmin := false
	if cancelAsk && MentionsCancel(in.ChatReply) {
		holdForAdmin = true
		alerts = append(alerts, cancelReplyAlert)
		log.Printf("agent: suhbat %d — javobda bekor qilish haqida gap bor, avto-javob to'xtatildi",
			conversationID)
	}

	// Kod topgan holatlar model nima yozganidan qat'i nazar xodimga
	// yetkaziladi: model ularni ko'rmasligi yoki "muammo yo'q" deb
	// o'tkazib yuborishi mumkin, lekin bu tekshirishni talab qiladi.
	in.Alerts = alerts
	in.HelpText = withAlerts(in.HelpText, alerts)

	if in.Error != "" {
		in.Status = StatusFailed
	} else if in.ChatReply == "" && in.HelpText == "" {
		in.Status = StatusFailed
		in.Error = "model na chat, na help qaytardi"
	}

	// 3. help — Telegram guruhiga tasdiqsiz ketadi (quyida, murojaat
	//    saqlangandan keyin: DeliverHelp). Sozlamadan o'chirilgan bo'lsa
	//    faqat bazada qoladi va statistikada hisoblanadi
	//    ("needed_staff", support/stats.go).

	// 4. chat — mijozga. Avto-javob yoqilgan bo'lsa darhol, aks holda
	//    admin tasdig'ini kutadi.
	switch {
	case in.Status == StatusFailed:
		// zanjir xatosi — hech narsa yuborilmaydi

	case in.ChatReply != "":
		if holdForAdmin || !sendIfAuto(in, "avto") {
			in.Status = StatusPending
		}

	default:
		// Faqat help bor edi, mijozga yoziladigan narsa yo'q: help
		// guruhga ketadi, murojaatning o'zi esa admin panelda ko'rinib
		// tursin deb "pending" holatida qoladi.
		in.Status = StatusPending
	}

	if err := SaveInteraction(DB, in); err != nil {
		return in, fmt.Errorf("bazaga yozish: %w", err)
	}
	// help — guruhga darhol, tasdiq kutmasdan (murojaat saqlangandan
	// keyin: xabar id'si shu yozuvga yoziladi). Telegram ishlamasa
	// murojaat baribir saqlangan, log yetarli.
	if err := DeliverStaffNotice(in, issues); err != nil {
		log.Printf("agent: suhbat %d — guruhga xabar ketmadi: %v", conversationID, err)
	}
	log.Printf("agent: suhbat %d — %s, %d bosqich, %s", conversationID, in.Status, in.StepsCount, usage)
	return in, nil
}

// DeliverChat mijozga javob yuboradi va javob yetib borsa o'sha xabarlarni
// "o'qilgan" deb belgilaydi. Mijozga ketadigan yagona yo'l shu.
//
// Tayyor javob generatsiyadan keyin DARHOL yuborilsa (avto-javob),
// xodim shu millisekundlar ichida ulgurishi amalda mumkin emas —
// shuning uchun qayta tekshiruv (yana bitta tashqi so'rov) o'tkazib
// yuboriladi: qarang deliverChat.
func DeliverChat(in *Interaction) error {
	return deliverChat(in, true)
}

func deliverChat(in *Interaction, recheck bool) error {
	if in.ChatReply == "" {
		return nil
	}
	// Javob tayyorlangandan beri xodim mijozga o'zi javob yozgan
	// bo'lishi mumkin — bunda bizning javobimiz takror bo'lib tushadi.
	//
	// Ikki holat to'sib qo'yilmaydi: `Forced` — admin ataylab qo'lda
	// yuborayotgani, va `SourceTelegram` — matn xodimning o'zinikidan
	// kelib chiqqan (xodim guruhda reply qilgan), demak yuborilishi
	// ataylab so'ralgan.
	var msgs []Message
	if recheck && !in.Forced && in.Source != SourceTelegram {
		ours, history := alreadyAnswered(in.ConversationID)
		if ours {
			return ErrAnsweredByStaff
		}
		msgs = history
	}

	// Salom: kunning birinchi javobi bo'lsa qo'shiladi, aks holda model
	// qo'ygan salom olib tashlanadi (greeting.go). Qaror aynan shu yerda,
	// yuborish oldidan qabul qilinadi — qoralama kecha tayyorlangan yoki
	// bir mijozning ikkinchi suhbatida allaqachon salomlashgan bo'lishi
	// mumkin.
	if text := applyGreeting(in, msgs); text != in.ChatReply {
		in.ChatReply = text
		// Yuborishdan OLDIN saqlanadi: transport xatosidan keyingi qayta
		// urinishda matn o'zgarmasin (WithGreeting idempotent).
		saveFlag(in, "chat_reply", text)
	}
	// Avval rasm, keyin matn: xodim guruhda ham shu tartibda yuboradi
	// va mijoz izohni rasmga qarab o'qiydi. Rasm ketmasa matn ham
	// yuborilmaydi — yarim javob chalkashtiradi.
	if in.ImageURL != "" && !in.ImageSent {
		if err := SendImageToClient(in.ConversationID, in.ImageURL); err != nil {
			return fmt.Errorf("rasm: %w", err)
		}
		in.ImageSent = true
		saveFlag(in, "image_sent", true)
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

// withAlerts - kod topgan holatlarni help matniga qo'shadi. Model help
// yozmagan bo'lsa ham xabar ketadi; takrorlanmaydi.
func withAlerts(help string, alerts []string) string {
	if len(alerts) == 0 {
		return help
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(help))
	for _, a := range alerts {
		if strings.Contains(b.String(), a) {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("⚠️ " + a)
	}
	return b.String()
}

// helpText - guruhga ketadigan "xodim kerak" xabari. Faqat yopiladigan
// muammoli buyurtma bo'lmaganda ishlatiladi: buyurtma bor bo'lsa xulosa
// o'sha xabarning ichiga qo'shiladi (DeliverStaffNotice).
//
// Ko'rinishi muammoli buyurtma xabari bilan BIR XIL
// (support/notify_text.go): sarlavha, mijoz, suhbat, tana va reply
// haqida bir qator.
func helpText(in *Interaction) string {
	var b strings.Builder
	b.WriteString(guruhSarlavha("🆘 Yordam kerak", in.ClientID, in.ClientID, in.ConversationID))
	b.WriteString("\n" + strings.TrimSpace(in.HelpText) + "\n")
	b.WriteString(guruhFooter(false))
	return b.String()
}

// DeliverStaffNotice - zanjir oxirida guruhga ketadigan YAGONA xabar.
//
// Ilgari bir muammo guruhga ikki marta tushardi: "⚠️ Muammoli buyurtma"
// ni DetectIssues zanjir o'rtasida yuborar, keyin esa xuddi shu muammo
// haqida AI ning "🆘 Yordam kerak" xabari ketardi — bir xil mijoz, bir
// xil buyurtma, ikki xil ko'rinishda. Endi ikkalasi bitta xabar:
// buyurtma tafsilotlari va AI xulosasi yonma-yon turadi, xodim bitta
// reply bilan ham muammoni yopadi, ham mijozga javob beradi.
//
// Muammo topilmagan bo'lsa (yopiladigan buyurtma yo'q) xabar eski
// "🆘 Yordam kerak" ko'rinishida ketaveradi — tuzilishi baribir bir xil
// (support/notify_text.go).
func DeliverStaffNotice(in *Interaction, issues []*OrderIssue) error {
	// Xodimning o'z javobidan tug'ilgan murojaat qaytib guruhga chiqmaydi.
	if in == nil || in.Source == SourceTelegram {
		return nil
	}

	// Sozlama o'chirilgan bo'lsa AI xulosasi guruhga chiqmaydi. Kod topgan
	// holat (Alerts) bo'lsa — baribir chiqadi: uni model emas, tizim topgan.
	help := strings.TrimSpace(in.HelpText)
	if in.HelpSent || (!HelpToTelegramOn() && len(in.Alerts) == 0) {
		help = ""
	}

	// Muammoli buyurtma bor: xulosa ham shu xabarning ichiga kiradi.
	if len(issues) > 0 {
		msgID := NotifyIssues(issues, help)
		if msgID == 0 {
			return fmt.Errorf("muammoli buyurtma xabari guruhga ketmadi")
		}
		// Xulosa shu xabarga ilindi — qayta yuborilmasin. Reply esa
		// muammo yo'li bilan ishlanadi (telegram_updates.go: muammolar
		// birinchi tekshiriladi), shuning uchun help_message_id faqat
		// takror yuborishni to'xtatish uchun yoziladi.
		if help != "" {
			markHelpSent(in, msgID)
		}
		return nil
	}

	// Muammo yo'q — faqat AI xulosasi.
	if help == "" {
		return nil
	}
	msgID, err := SendTelegramIssue(helpText(in))
	if err != nil {
		return fmt.Errorf("help: %w", err)
	}
	markHelpSent(in, msgID)
	return nil
}

// markHelpSent - xulosa guruhga ketgani va qaysi xabarga ilingani.
func markHelpSent(in *Interaction, msgID int64) {
	in.HelpSent = true
	in.HelpMessageID = msgID
	RememberTelegramPost(msgID, in.ConversationID, in.ClientID, "help", in.ID)
	if DB != nil && in.ID > 0 {
		DB.Model(in).Updates(map[string]any{"help_sent": true, "help_message_id": msgID})
	}
}

// DeliverHelp - admin tasdiqlaganda ishlatiladigan yo'l: zanjir paytida
// Telegram ishlamay qolgan bo'lsa AI xulosasi qayta yuboriladi. Bu yerda
// muammoli buyurtmalar ro'yxati qo'lda bo'lmaydi (u zanjirga tegishli),
// shuning uchun xabar "🆘 Yordam kerak" ko'rinishida ketadi.
func DeliverHelp(in *Interaction) error {
	if in == nil || strings.TrimSpace(in.HelpText) == "" {
		return nil
	}
	return DeliverStaffNotice(in, nil)
}

// Deliver - admin tasdiqlaganda: chat mijozga ketadi, help hali
// yuborilmagan bo'lsa (masalan Telegram ishlamay qolgan edi) qayta uriniladi.
func Deliver(in *Interaction) error {
	// Xatolar errors.Join bilan birlashtiriladi: chaqiruvchi
	// ErrAnsweredByStaff'ni errors.Is orqali ajrata olishi kerak
	// (matnga aylantirilsa bu imkoniyat yo'qoladi).
	var errs []error
	if err := DeliverChat(in); err != nil {
		errs = append(errs, err)
	}
	if err := DeliverHelp(in); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// saveFlag - bazadagi bitta bayroqni yangilaydi (yozuv hali saqlanmagan
// bo'lsa hech narsa qilinmaydi — qiymat struct'da qolib, keyin saqlanadi).
func saveFlag(in *Interaction, field string, val any) {
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
// ma'lumot. Boshqa hech qanday ko'rsatma qo'shilmaydi — til, salom va
// hokazo qoidalarning barchasini promtning o'zi (DB) belgilaydi.
func buildUserMessage(transcript string, data []string) string {
	var b strings.Builder
	b.WriteString("Suhbatning oxirgi xabarlari (eskisidan yangisiga). ")
	b.WriteString(`"type": "client" — mijoz yozgan, "type": "agent" — biz yozgan javob:` + "\n")
	b.WriteString(transcript)
	if len(data) > 0 {
		b.WriteString("\n\nTizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):\n")
		b.WriteString(strings.Join(data, "\n"))
	}
	return b.String()
}

// MaxUnreadableImagesToGroup - OCR raqam topa olmaganda guruhga ko'pi
// bilan nechta rasm yuboriladi.
const MaxUnreadableImagesToGroup = 3

// sendUnreadableImages - OCR rasmdan hech qanday buyurtma/trek raqami topa
// olmasa (past sifat, burchak, tanish bo'lmagan chek formati va h.k.),
// mijoz yuborgan eng oxirgi rasmlarni (ko'pi bilan
// MaxUnreadableImagesToGroup ta) xodimlar guruhiga yuboradi — xodim o'z
// ko'zi bilan ko'rib buyurtma raqamini aniqlay oladi. Xatolik bo'lsa
// (masalan Telegram sozlanmagan) faqat logga yoziladi, zanjirni to'xtatmaydi.
func sendUnreadableImages(conversationID, clientID int64, img ImageNumbers) {
	links := img.Links
	if len(links) > MaxUnreadableImagesToGroup {
		links = links[:MaxUnreadableImagesToGroup]
	}
	for i, link := range links {
		var caption string
		if i == 0 {
			// Sarlavha boshqa guruh xabarlari bilan bir xil: ichida
			// "Suhbat: #<id>" turadi — xodim ham, kod ham (reply
			// kelganda) qaysi suhbat ekanini shundan biladi.
			caption = guruhSarlavha("🖼 Mijoz rasm yubordi — raqam avtomatik o'qilmadi",
				clientID, clientID, conversationID) +
				"\nRasmdagi buyurtma/trek raqamini xodim o'zi ko'rsin." +
				guruhFooter(false)
		}
		msgID, err := SendTelegramPhoto(link, caption)
		if err != nil {
			log.Printf("agent: mijoz %d — rasm guruhga yuborilmadi: %v", clientID, err)
			continue
		}
		// Xodim shu RASMGA reply qilsa ham javob o'z suhbatini topsin.
		RememberTelegramPost(msgID, conversationID, clientID, "image", 0)
	}
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
//
// Uchinchi qaytadigan qiymat — KOD topgan, xodimga aytilishi kerak
// bo'lgan holatlar (masalan posilka mijoz viloyatidan boshqa filialda).
// Ular modelga ham izoh bo'lib boradi, ham guruh xabariga qo'shiladi:
// model ularni o'zi topishi yoki o'tkazib yuborishi mumkin emas.
//
// To'rtinchi qiymat — shu chaqiruvda YANGI ochilgan muammoli buyurtmalar.
// Ular guruhga shu yerdan yuborilmaydi: zanjir tugagach, AI xulosasi
// bilan birga BITTA xabar bo'lib ketadi (DeliverStaffNotice).
func fetchSystemData(a AgentJSON, clientID, conversationID int64) (string, bool, []string, []*OrderIssue) {
	out := map[string]any{}
	var alerts []string
	var issues []*OrderIssue
	// mismatches - posilka mijoz viloyatiga tushmagan holatlar. Ular
	// adminka ro'yxatiga ham qaytib ta'sir qiladi: o'sha buyurtmaning
	// "filialdan olib keting" ko'rsatmasi noto'g'ri bo'lib qoladi.
	var mismatches []BranchMismatch
	numbers := a.Numbers()
	// Ikki manba bir-birini to'ldiradi: adminkadagi status posilka
	// KELGANINI aytmaydi, buni faqat yetkazma ro'yxati aytadi. Shuning
	// uchun ikkalasi ham yig'ilgach solishtiriladi (quyida).
	var views []OrderView
	var deliveryRows []DeliveryOrder
	haveDelivery := false
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
		// Mijoz order_sn yoki trek raqamini aniq yozmagan bo'lsa (butun
		// ro'yxat so'ralgan bo'ladi) — to'lanmagan buyurtmalar modelga
		// yuborilmaydi: ular hali muammo emas, faqat token yeydi va
		// modelni chalkashtiradi. Mijoz aniq raqam yozgan bo'lsa, o'sha
		// buyurtma to'lanmagan bo'lsa ham ko'rsatiladi — savol aynan shu
		// haqida.
		if len(numbers) == 0 {
			rows = onlyPaidOrders(rows)
		}
		// Begona buyurtma modelga ham, muammo ro'yxatiga ham
		// tushmaydi (qarang: onlyOwnOrders).
		var foreign []string
		rows, foreign = onlyOwnOrders(rows, clientID)
		if len(foreign) > 0 {
			out["begona_buyurtma"] = map[string]any{
				"raqamlar": foreign,
				"korsatma": foreignOrderNote,
			}
			log.Printf("agent: suhbat %d — begona buyurtma(lar) chiqarib tashlandi: %v (mijoz %d)",
				conversationID, foreign, clientID)
		}
		// Mijoz turi (B2C/B2B) — yetkazish tarifini tushuntirish uchun.
		out["mijoz_turi"] = CustomerType(rows)
		// Muammoli buyurtmalarni aniqlash. Xabar bu yerdan ketmaydi —
		// yangi muammolar yuqoriga qaytadi va zanjir oxirida AI xulosasi
		// bilan bitta xabar bo'lib yuboriladi.
		var fresh []*OrderIssue
		views, fresh = DetectIssues(rows, clientID, conversationID)
		issues = append(issues, fresh...)
		if HasPendingOrders(views) {
			pending = true
		}
		if len(errs) > 0 {
			out["adminka_error"] = strings.Join(errs, "; ")
		}
	}

	// Adminkada "tranzaksiya yopilgan" (status 6) buyurtma bo'lsa,
	// yetkazma ma'lumoti model so'ramagan bo'lsa ham olinadi: usiz
	// posilka mijozga yetgan-yetmagani BILINMAYDI va model "buyurtmangiz
	// yakunlangan" deb yozib yuborardi.
	if a.Adminka && !a.Dashboard && needsArrivalCheck(views) {
		a.Dashboard = true
		log.Printf("agent: suhbat %d — tranzaksiya yopilgan buyurtma bor, yetkazma ham tekshirildi",
			conversationID)
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
			// Trek raqami bo'yicha qidiruv ham butun bazadan ketadi —
			// begona posilka bu yerda ham chiqarib tashlanadi.
			if bad := onlyOwnDelivery(&rows, clientID); len(bad) > 0 {
				log.Printf("agent: suhbat %d — begona yetkazma(lar) chiqarib tashlandi: %v (mijoz %d)",
					conversationID, bad, clientID)
				if _, ok := out["begona_buyurtma"]; !ok {
					out["begona_buyurtma"] = map[string]any{
						"raqamlar": bad,
						"korsatma": foreignOrderNote,
					}
				}
			}
			// Mijoz aniq buyurtma/trek raqami yozgan bo'lsa, yetkazma
			// ro'yxati O'SHA buyurtmaning posilkasi bilan cheklanadi.
			// Aks holda ro'yxatda mijozning boshqa posilkalari ham
			// qoladi va model ularning izohini so'ralgan buyurtmaga
			// ko'chirib yozadi ("filialdan olib ketgan bo'lishingiz
			// mumkinmi?") — so'ralgan posilka esa hali Xitoyda.
			askedOnly := false
			if len(numbers) > 0 {
				asked := trimAll(a.ExpressNum)
				for _, v := range views {
					if t := strings.TrimSpace(v.ExpressNum); t != "" {
						asked = append(asked, t)
					}
				}
				// Faqat bog'lanish aniq bo'lganda filtrlaymiz: adminka
				// buyurtmani topgan (treki bor-yo'qligi endi ma'lum)
				// yoki mijoz trek raqamining o'zini yozgan.
				if len(views) > 0 || len(asked) > 0 {
					kept := FilterDeliveryByTracks(rows, asked)
					if len(kept) != len(rows) {
						log.Printf("agent: suhbat %d — yetkazma ro'yxati so'ralgan buyurtma bilan cheklandi: %d/%d yozuv",
							conversationID, len(kept), len(rows))
					}
					askedOnly = len(kept) == 0
					rows = kept
				}
			}
			deliveryRows, haveDelivery = rows, true
			brief, bad := BriefDelivery(rows)
			if askedOnly {
				// Ro'yxat bo'shab qoldi — modelga nega bo'shligini va
				// nima qilmaslik kerakligini aytib qo'yamiz.
				brief.Note = askedOnlyNote
			}
			out["yetkazma"] = brief
			// Kod topgan yetkazma holatlari (punktda qotib qolgan,
			// muddati o'tgan) xodimlar guruhiga ham chiqadi.
			alerts = append(alerts, brief.Alerts...)
			mismatches = append(mismatches, bad...)
			for _, m := range bad {
				alerts = append(alerts, m.Text())
			}
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

	// Buyurtmalar ro'yxati oxirida yig'iladi: yetkazma ma'lumoti ham
	// olingan bo'lsa, har bir buyurtmaga posilkasi O'zbekistonga
	// kelgan-kelmagani yoziladi.
	if a.Adminka {
		briefs := BriefOrders(views, clientID)
		if haveDelivery {
			MarkArrival(briefs, deliveryRows)
		}
		// Posilka boshqa viloyatga tushgan bo'lsa, shu buyurtmaning
		// "qayerdan olib ketish" ko'rsatmasi o'chiriladi — aks holda model
		// mijozga uning O'Z viloyatidagi filialni aytib yuboradi.
		MarkMismatch(briefs, mismatches)
		out["adminka"] = briefs
	}

	// Kod topgan holatlar modelga ham ko'rsatiladi. Ilgari ular faqat
	// xodimlar guruhiga ketardi, modelga esa buyurtma ichidagi kichik
	// `izoh` bo'lib borardi — model uni o'qimay, qolgan (ishonchli
	// ko'rinadigan) maydonlarga qarab mijozga noto'g'ri ko'rsatma yozardi.
	// Endi ular eng yuqorida, aniq taqiq bilan turadi.
	if len(alerts) > 0 {
		out[alertKey] = map[string]any{
			"holatlar": alerts,
			"korsatma": alertGuidance,
		}
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), pending, alerts, issues
	}
	return string(raw), pending, alerts, issues
}

// needsArrivalCheck - ro'yxatda "tranzaksiya yopilgan" (status 6),
// treki bor buyurtma bormi. Bunday buyurtma yo'lga chiqqan, lekin
// kelgan-kelmagani faqat yetkazma ro'yxatidan bilinadi.
func needsArrivalCheck(views []OrderView) bool {
	for _, v := range views {
		if v.Status == StatusFinished && strings.TrimSpace(v.ExpressNum) != "" {
			return true
		}
	}
	return false
}

// HasPendingOrders - mijozda hali qo'liga tegmagan buyurtma bormi.
//
// Adminkadagi "yakunlangan" (status 6) KELGANI EMAS — u Xitoy
// tomonidagi tranzaksiya yopilganini bildiradi. Shuning uchun status
// bo'yicha "keldi" deb hisoblanmaydi: to'langan har qanday buyurtma
// yetkazma ma'lumoti bilan tasdiqlanmaguncha "yo'lda" sanaladi.
// To'lanmagani hisobga olinmaydi — u hali yo'lga chiqmagan.
func HasPendingOrders(views []OrderView) bool {
	for _, v := range views {
		if v.Paid {
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
//
// Javob shu funksiyada RunChain tugagandan keyin bir zumda ketadi —
// "xodim ulgurib javob yozdi" tekshiruvi (yana bitta tashqi so'rov)
// bu yerda shart emas, faqat kechikishi mumkin bo'lgan Deliver
// (admin tasdig'i) yo'lida kerak. Har bir suhbatga tashqi API'ga
// ketadigan bitta so'rovni tejaydi — ko'p suhbatli navbatda sezilarli.
func sendIfAuto(in *Interaction, handledBy string) bool {
	if !AutoReplyOn() {
		return false
	}
	if err := deliverChat(in, false); err != nil {
		if errors.Is(err, ErrAnsweredByStaff) {
			// Xato emas: xodim ulgurgan, javob endi kerak emas.
			in.Status = StatusRejected
			in.Error = ErrAnsweredByStaff.Error()
			log.Printf("agent: suhbat %d — xodim javob bergan, tayyor javob yuborilmadi", in.ConversationID)
		} else {
			in.Status = StatusFailed
			in.Error = err.Error()
		}
	} else {
		in.markSent(handledBy)
	}
	return true
}

// foreignOrderNote - so'ralgan buyurtma BOSHQA mijozniki bo'lganda
// modelga beriladigan tayyor ko'rsatma.
//
// Nega kerak: raqam bo'yicha qidiruv adminkaning va yetkazmaning BUTUN
// bazasidan ketadi (mijoz o'z raqamini noto'g'ri yozishi mumkin).
// Ilgari bunday buyurtma modelga "boshqa mijozning buyurtmasi" degan
// belgi bilan baribir berilardi va model uning holatini, sanasini,
// qayerdaligini mijozga aytib yuborardi — bu begona odamning ma'lumoti.
// Xodimlar guruhiga ham shu odam so'ramagan muammo bo'lib chiqardi.
const foreignOrderNote = "Bu buyurtma BOSHQA mijozga tegishli — uning ma'lumoti " +
	"ataylab berilmadi. Mijozga bu buyurtma haqida HECH QANDAY tafsilot aytma: " +
	"holati, sanasi, qayerdaligi, nomi — hech biri. Faqat shuni ayt: bu raqam " +
	"mijozning akkauntiga tegishli emas, o'z buyurtma raqamini tekshirib yuborsin " +
	"(yoki buyurtma egasi o'zi murojaat qilsin). Raqamni \"tuzatib\" o'zingdan " +
	"boshqasini taklif qilma."

// onlyOwnOrders - faqat SHU mijozning buyurtmalarini qoldiradi.
//
// Egasi noma'lum (user_id bo'sh) yozuv qoldiriladi: adminka ba'zan bu
// maydonni bermaydi, borini yashirib qo'yishdan ko'ra ko'rsatgan
// ma'qul. Mijoz noma'lum bo'lsa (clientID = 0) filtr ishlamaydi.
//
// Ikkinchi qiymat — chiqarib tashlangan buyurtma raqamlari (logga va
// modelga tushuntirish uchun).
func onlyOwnOrders(rows []AdminkaOrder, clientID int64) ([]AdminkaOrder, []string) {
	if clientID <= 0 {
		return rows, nil
	}
	out := make([]AdminkaOrder, 0, len(rows))
	var foreign []string
	for _, o := range rows {
		if o.UserID > 0 && o.UserID != clientID {
			foreign = append(foreign, firstNonEmpty(o.OrderSN, o.ExpressNum))
			continue
		}
		out = append(out, o)
	}
	return out, foreign
}

// onlyOwnDelivery - yetkazma yozuvlaridan begonalarini olib tashlaydi.
// Ro'yxat joyida o'zgaradi; qaytadigan qiymat — tashlangan treklar.
func onlyOwnDelivery(rows *[]DeliveryOrder, clientID int64) []string {
	if clientID <= 0 || rows == nil {
		return nil
	}
	out := make([]DeliveryOrder, 0, len(*rows))
	var foreign []string
	for _, d := range *rows {
		if d.UserID > 0 && d.UserID != clientID {
			foreign = append(foreign, d.ExpressNum)
			continue
		}
		out = append(out, d)
	}
	*rows = out
	return foreign
}
