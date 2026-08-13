package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"gorm.io/gorm"

	"sahiy-agent/internal/ai"
	"sahiy-agent/internal/auth"
	"sahiy-agent/internal/category"
	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
	"sahiy-agent/internal/db"
	"sahiy-agent/internal/escalation"
	"sahiy-agent/internal/gemini"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/ollama"
	"sahiy-agent/internal/openai"
	"sahiy-agent/internal/orders"
	"sahiy-agent/internal/pricing"
	"sahiy-agent/internal/service"
	"sahiy-agent/internal/settings"
	"sahiy-agent/internal/store"
	"sahiy-agent/internal/support"
	"sahiy-agent/internal/telegram"
	"sahiy-agent/internal/tgtext"
	"sahiy-agent/internal/userbot"
	"sahiy-agent/internal/web"
)

const (
	envPath      = ".env"
	pollInterval = 30 * time.Second
	// fetchLimit — serverdan bir so'rovda olinadigan xabarlar soni.
	// Kontekstga qanchasi kirishini HISTORY_LIMIT hal qiladi; bu yerdan
	// ko'proq olinishi tail (eng yangi N ta) to'g'ri chiqishi uchun.
	fetchLimit = 50
	// maxImages — bitta suhbatdan eslab qolinadigan rasm havolalari soni
	// (rasmlar tahlil qilinmaydi — faqat "rasm bor" belgisi uchun).
	maxImages = 3
)

// staffChannel — xodimlar guruhiga xabar yuboradigan kanal
// (userbot yoki Bot API).
type staffChannel interface {
	// Send matnni yuboradi; code — monospace (nusxalanadigan) bo'laklar.
	Send(text string, code []tgtext.Span) (int64, error)
}

type app struct {
	cfg       *config.Config
	ai        *ai.Client
	track     *support.Tracker
	hist      *store.Store
	esc       *escalation.Store
	cats      *category.Store
	ord       *orders.Lookup  // buyurtma holati (ikkinchi sayt)
	staff     staffChannel    // nil bo'lishi mumkin
	db        *gorm.DB        // byudjet ogohlantirishi bir marta yuborilishi uchun
	local     *ollama.Client  // lokal model (AI_PROVIDER=ollama bo'lsa), aks holda nil
	set       *settings.Store // dashboarddan boshqariladigan sozlamalar
	cachePath string          // token cache fayli (DataDir ostida)
}

func main() {
	cfg, err := config.Load(envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config xatosi:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Yordamchi buyruqlar.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			if err := loginCmd(ctx, cfg); err != nil {
				fmt.Fprintln(os.Stderr, "login xatosi:", err)
				os.Exit(1)
			}
			return
		case "groups":
			if err := listGroups(ctx, cfg); err != nil {
				fmt.Fprintln(os.Stderr, "guruhlarni olish xatosi:", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "noma'lum buyruq: %s (login | groups)\n", os.Args[1])
			os.Exit(1)
		}
	}

	// Postgres (GORM).
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL .env da bo'lishi kerak")
		os.Exit(1)
	}
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "postgres xatosi:", err)
		os.Exit(1)
	}
	fmt.Println("✓ Postgres ulandi")

	a := &app{
		cfg:       cfg,
		track:     support.NewTracker(database),
		hist:      store.New(database),
		esc:       escalation.New(database),
		cats:      category.New(database),
		db:        database,
		set:       settings.New(database),
		cachePath: filepath.Join(cfg.DataDir, "token.json"),
	}
	// Sozlamalar: .env dagi qiymat faqat birinchi marta yoziladi, keyin
	// dashboarddagi tugma ustun turadi.
	if err := a.set.Init(settings.AutoReply, cfg.AutoReply); err != nil {
		fmt.Fprintln(os.Stderr, "sozlama xatosi:", err)
	}
	if err := a.set.Init(settings.AIEnabled, true); err != nil {
		fmt.Fprintln(os.Stderr, "sozlama xatosi:", err)
	}

	backend, local := pickBackend(cfg)
	a.ai, a.local = ai.New(backend, cfg.AgentPrompt), local

	// Ikkinchi sayt (service API) — buyurtma holatini id/track bo'yicha ko'rish.
	svc := service.New(cfg.ServiceBaseURL, service.LoginRequest{
		Phone:      cfg.ServicePhone,
		Password:   cfg.ServicePassword,
		APKType:    cfg.ServiceAPKType,
		DeviceID:   cfg.ServiceDeviceID,
		DeviceName: cfg.ServiceDeviceName,
		DeviceType: cfg.ServiceDeviceType,
		FcmToken:   cfg.ServiceFcmToken,
	}, filepath.Join(cfg.DataDir, "service-token.json"))
	a.ord = orders.New(svc)
	if svc.Enabled() {
		fmt.Printf("📦 Buyurtma qidiruvi yoqilgan (%s)\n", cfg.ServiceBaseURL)
	} else {
		fmt.Println("ℹ️  Buyurtma qidiruvi o'chiq — .env da SERVICE_PHONE/SERVICE_PASSWORD yo'q")
	}

	// Narx: .env dagi qiymat kod jadvalidan ustun turadi.
	pricing.Set(cfg.PriceIn, cfg.PriceCachedIn, cfg.PriceOut)
	if a.local != nil {
		// Lokal model bepul — narx jadvaliga nol narx bilan qo'shiladi.
		pricing.MarkFree(a.local.Model)
		fmt.Printf("🖥  Lokal model: %s (RAM: keep_alive=%s, kontekst=%d token)\n",
			a.local.Model, a.local.KeepAlive, a.local.NumCtx)
		if err := a.local.Check(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
			if cfg.OpenAIAPIKey != "" {
				fmt.Fprintln(os.Stderr, "   Zaxira sifatida OpenAI ishlatiladi.")
			}
		}
	}
	if a.ai.Ready() {
		fmt.Printf("🧠 AI: %s\n", a.ai.Name())
		if _, ok := pricing.Lookup(a.modelName()); ok {
			fmt.Printf("💰 Narx — %s\n", pricing.Describe(a.modelName()))
		} else {
			fmt.Printf("⚠️  %s narxi noma'lum — .env da AI_PRICE_IN va AI_PRICE_OUT ni o'rnating\n"+
				"   (tokenlar baribir saqlanadi, narx qo'yilgach qayta hisoblash mumkin)\n", a.modelName())
		}
		if a.set.Bool(settings.AutoReply, cfg.AutoReply) {
			fmt.Println("📤 Avto-javob YOQILGAN — AI javoblari mijozga darhol ketadi")
		} else {
			fmt.Println("👁  Avto-javob o'chirilgan — javoblar dashboardda tasdiqlashni kutadi")
		}
		if cfg.BudgetUSD > 0 {
			fmt.Printf("🎯 Oylik byudjet: $%.2f (oshsa xodimlar guruhiga ogohlantirish)\n", cfg.BudgetUSD)
		}
	} else {
		fmt.Println("⚠️  AI kaliti yo'q — .env da OPENAI_API_KEY yoki GEMINI_API_KEY o'rnating")
	}

	// Web dashboard.
	srv := web.New(a.hist, a.cats, a.esc, a.set, web.Options{
		Addr:      cfg.WebAddr,
		AdminUser: cfg.AdminUser,
		AdminPass: cfg.AdminPass,
		Dev:       cfg.WebDev,
		SendReply: a.sendReply, // dashboarddan tasdiqlangan javob
	})
	if cfg.AdminUser == "" || cfg.AdminPass == "" {
		fmt.Println("⚠️  Dashboard parolsiz ochiq — .env da ADMIN_USER/ADMIN_PASS o'rnating")
	}
	go func() {
		fmt.Printf("🌐 Dashboard: http://localhost%s\n", cfg.WebAddr)
		if err := srv.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "web server xatosi:", err)
		}
	}()

	// Telegram: userbot (afzal) yoki Bot API.
	a.setupTelegram(ctx)

	if st, err := a.hist.Stats(); err == nil {
		fmt.Printf("📊 Tarix: %s\n", st)
	}
	fmt.Printf("🚀 Agent ishga tushdi. Har %s da tekshiradi.\n", pollInterval)

	a.runCycle(ctx)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n👋 To'xtatilmoqda...")
			if sqlDB, err := database.DB(); err == nil {
				sqlDB.Close()
			}
			return
		case <-ticker.C:
			a.runCycle(ctx)
		}
	}
}

// pickBackend .env asosida AI provayderini tanlaydi. AI_PROVIDER berilmasa:
// OPENAI_API_KEY bo'lsa OpenAI, aks holda Gemini.
func pickBackend(cfg *config.Config) (ai.Backend, *ollama.Client) {
	switch cfg.AIProvider {
	case "openai":
		return openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL), nil
	case "gemini":
		return gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel), nil
	case "ollama":
		// Lokal model asosiy; OpenAI kaliti bo'lsa u zaxira bo'lib qoladi —
		// Ollama o'chib qolsa mijoz javobsiz qolmaydi.
		local := ollama.New(cfg.OllamaURL, cfg.OllamaModel, ollama.Options{
			KeepAlive:   cfg.OllamaKeepAlive,
			NumCtx:      cfg.OllamaNumCtx,
			MaxTokens:   cfg.OllamaMaxTokens,
			Temperature: cfg.OllamaTemperature,
			Timeout:     cfg.OllamaTimeout,
		})
		if cfg.OpenAIAPIKey != "" {
			return &ai.Fallback{
				Primary:   local,
				Secondary: openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL),
			}, local
		}
		return local, local
	}
	// Avtomatik tanlashda lokal model hech qachon o'zi tanlanmaydi.
	if cfg.OpenAIAPIKey != "" {
		return openai.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, cfg.OpenAIBaseURL), nil
	}
	return gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel), nil
}

// setupTelegram userbot yoki Bot API kanalini sozlaydi.
func (a *app) setupTelegram(ctx context.Context) {
	cfg := a.cfg

	// 1) Userbot (MTProto) — API_ID/API_HASH va ALLOWED_GROUPS bo'lsa.
	if cfg.TgAPIID != 0 && cfg.TgAPIHash != "" && len(cfg.AllowedGroups) > 0 {
		target := escalationTarget(cfg)
		if target == 0 {
			fmt.Fprintln(os.Stderr, "⚠️  Eskalatsiya guruhi aniqlanmadi — TELEGRAM_CHAT_ID yoki ALLOWED_GROUPS ni tekshiring")
			return
		}
		// requireSession=true: sessiya bo'lmasa kod so'ramaydi (takroriy login yo'q).
		ub := userbot.New(
			cfg.TgAPIID, cfg.TgAPIHash, cfg.TgPhone, cfg.TgSession,
			cfg.AllowedGroups,
			a.onStaffReply, // reply -> mijozga
			noPrompt("Telegram kod"),
			noPrompt("2FA parol"),
			true,
		)
		ch := &ubChannel{bot: ub, target: target, ctx: ctx}
		go func() {
			if err := ub.Run(ctx); err != nil {
				if errors.Is(err, userbot.ErrNoSession) {
					ch.disabled.Store(true)
					fmt.Fprintf(os.Stderr,
						"⚠️  Telegram sessiyasi yo'q (%s). Eskalatsiya o'chiq.\n"+
							"   Bir marta bajaring:  docker compose run --rm agent login\n", cfg.TgSession)
					return
				}
				if !errors.Is(err, context.Canceled) {
					fmt.Fprintln(os.Stderr, "userbot xatosi:", err)
				}
			}
		}()
		a.staff = ch
		fmt.Printf("📨 Userbot eskalatsiya yoqilgan (guruh %d)\n", target)
		return
	}

	// 2) Bot API — TELEGRAM_TOKEN bo'lsa.
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		bot := telegram.New(cfg.TelegramToken)
		a.staff = &botChannel{bot: bot, chatID: cfg.TelegramChatID}
		go a.pollBotAPI(ctx, bot)
		fmt.Println("📨 Bot API eskalatsiya yoqilgan")
	}
}

// runCycle bitta tekshirish tsikli.
func (a *app) runCycle(ctx context.Context) {
	ts := time.Now().Format("15:04:05")

	if !a.set.Bool(settings.AIEnabled, true) {
		// Xabarlar javobsiz kutib turadi — tracker yangilanmaydi, shuning
		// uchun AI yoqilganda hammasiga javob yoziladi.
		fmt.Printf("[%s] ⏸  AI o'chirilgan (dashboarddan yoqing)\n", ts)
		return
	}

	c, senderID, err := a.apiClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] token xatosi: %v\n", ts, err)
		return
	}

	chats, err := support.FetchConversations(c, 1, 20, support.FilterRequest{
		Type: "client", State: []int{1, 2, 3},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] chatlar xatosi: %v\n", ts, err)
		return
	}

	// Birinchi ishga tushish: mavjud suhbatlarga javob bermay, faqat
	// hozirgi holatni eslab qolamiz (BACKFILL=true bo'lmasa).
	known, err := a.track.Count()
	if err == nil && known == 0 && !a.cfg.Backfill {
		a.baseline(c, chats)
		fmt.Printf("[%s] birinchi ishga tushish: %d ta suhbat javobsiz belgilandi (BACKFILL=true bilan javob beradi)\n", ts, len(chats))
		return
	}

	fresh := a.track.Candidates(chats)
	if len(fresh) == 0 {
		fmt.Printf("[%s] yangi xabar yo'q (jami %d)\n", ts, len(chats))
		return
	}

	catalog, err := a.cats.Catalog()
	if err != nil {
		fmt.Fprintln(os.Stderr, "    kategoriyalar xatosi:", err)
	}

	handled := 0
	for _, ch := range fresh {
		if ctx.Err() != nil {
			return
		}
		if a.handleChat(ctx, c, senderID, ch, catalog) {
			handled++
		}
	}
	if handled > 0 {
		fmt.Printf("[%s] 🔔 %d ta xabarga ishlov berildi\n", ts, handled)
		if st, err := a.hist.Stats(); err == nil {
			fmt.Printf("[%s] 📊 %s\n", ts, st)
		}
		a.checkBudget()
	}
}

// checkBudget oylik xarajat chegaradan oshgan bo'lsa xodimlar guruhiga bir
// marta ogohlantirish yuboradi. Agent to'xtamaydi — faqat xabar beradi.
// Takrorlanmasligi uchun oy kaliti `kv` jadvalida belgilanadi.
func (a *app) checkBudget() {
	if a.cfg.BudgetUSD <= 0 {
		return
	}
	spent, err := a.hist.MonthCost()
	if err != nil || spent < a.cfg.BudgetUSD {
		return
	}
	key := "budget_warned_" + time.Now().Format("2006-01")
	if v, err := db.GetSetting(a.db, key); err != nil || v != "" {
		return // allaqachon ogohlantirilgan (yoki bazaga kirib bo'lmadi)
	}

	msg := fmt.Sprintf("⚠️ AI byudjeti oshdi\nShu oyda sarflandi: $`%.4f`\nChegara: $`%.2f`\n"+
		"Agent ishlashda davom etmoqda.", spent, a.cfg.BudgetUSD)
	fmt.Fprintf(os.Stderr, "⚠️  AI byudjeti oshdi: $%.4f / $%.2f\n", spent, a.cfg.BudgetUSD)
	if a.staff != nil {
		text, code := tgtext.Build(msg)
		if _, err := a.staff.Send(text, code); err != nil {
			fmt.Fprintln(os.Stderr, "byudjet ogohlantirishi yuborilmadi:", err)
			return // keyingi tsiklda qayta urinamiz
		}
	}
	if err := db.SetSetting(a.db, key, fmt.Sprintf("%.6f", spent)); err != nil {
		fmt.Fprintln(os.Stderr, "byudjet belgisi saqlanmadi:", err)
	}
}

// baseline mavjud suhbatlarni javobsiz "ko'rilgan" deb belgilaydi.
func (a *app) baseline(c *client.Client, chats []support.Conversation) {
	for _, ch := range chats {
		msgs, err := support.FetchMessages(c, ch.ID, 1, a.fetchLimit())
		if err != nil {
			fmt.Fprintln(os.Stderr, "    baseline xabarlar xatosi:", err)
			continue
		}
		lastID, _ := support.LastClientMessage(msgs)
		if err := a.track.Commit(ch.ID, lastID); err != nil {
			fmt.Fprintln(os.Stderr, "    baseline saqlanmadi:", err)
		}
	}
}

// handleChat bitta suhbatga ishlov beradi. Javob berilgan bo'lsa true.
func (a *app) handleChat(ctx context.Context, c *client.Client, senderID int64,
	ch support.Conversation, catalog string) bool {

	msgs, err := support.FetchMessages(c, ch.ID, 1, a.fetchLimit())
	if err != nil {
		fmt.Fprintln(os.Stderr, "    xabarlar xatosi:", err)
		return false
	}
	lastID, lastText := support.LastClientMessage(msgs)
	if lastID == 0 || a.track.Handled(ch.ID, lastID) {
		return false // yangi mijoz xabari yo'q
	}

	// Eski xabar (masalan 2 kunlik) — AI'ga yubormaymiz, token behuda ketmasin.
	// Ko'rilgan deb belgilanadi, shu suhbatga keyin yangi xabar kelsa javob beriladi.
	if stale, age := support.Stale(msgs, a.cfg.MaxMessageAge); stale {
		fmt.Printf("  ⏭  #%d — oxirgi xabar %s oldin (limit %s), AI'ga yuborilmadi\n",
			ch.ID, age.Round(time.Hour), a.cfg.MaxMessageAge)
		_ = a.track.Commit(ch.ID, lastID)
		return false
	}

	fmt.Printf("  === #%d | %s | %q ===\n", ch.ID, ch.ClientName, ch.Title)

	if !a.ai.Ready() {
		fmt.Println("    (AI kaliti yo'q)")
		return false
	}

	// tr — agent nimani qanday tushunganini bosqichma-bosqich yozib boradi;
	// oxirida bazaga tushadi va dashboardda ko'rinadi.
	var tr trace

	// meter — shu suhbatga ketgan barcha AI so'rovlarining tokenlarini yig'adi
	// (Classify + Ask + kerak bo'lsa Summarize). Xarajat shundan hisoblanadi.
	meter := &ai.Meter{}
	ctx = ai.WithMeter(ctx, meter)

	// 1-qadam: butun tarix emas — faqat javob berilmagan YANGI xabarlar va
	// ulardan oldingi CONTEXT_BEFORE ta xabar olinadi (token tejash).
	prevID := a.track.LastID(ch.ID)
	window := support.Window(msgs, prevID, a.cfg.ContextBefore, a.cfg.HistoryLimit, a.cfg.MaxMessageAge)
	transcript := support.Transcript(window)
	tr.add("📚", "%d ta xabar o'qildi: yangilari + %d ta kontekst (jami tarix %d ta)",
		len(window), a.cfg.ContextBefore, len(msgs))

	// 2-qadam: mos kategoriyani aniqlash.
	var (
		catID   *uint
		catInfo string
	)
	if catalog == "" {
		tr.add("🏷", "Kategoriya bosqichi o'tkazib yuborildi (katalog bo'sh)")
	} else {
		id, err := a.ai.Classify(ctx, catalog, transcript)
		switch {
		case err != nil:
			tr.add("⚠️", "Kategoriya aniqlanmadi (xato: %v)", err)
		case id == 0:
			tr.add("🏷", "Mos kategoriya topilmadi — umumiy bilim bilan javob beriladi")
		default:
			cat, cerr := a.cats.Get(id)
			if cerr != nil || !cat.Active {
				tr.add("🏷", "Kategoriya %d tanlandi, lekin o'chirilgan/topilmadi", id)
			} else {
				catID, catInfo = &cat.ID, cat.Content
				tr.add("🏷", "Kategoriya: %s (id %d)", cat.Name, cat.ID)
			}
		}
	}

	// 3-qadam: rasmlar. Rasm AI'ga yuborilmaydi (token tejash) — faqat
	// borligi qayd etiladi, javobda esa mijozdan buyurtma raqami so'raladi.
	imgURLs := support.ImageURLs(window, maxImages)
	if len(imgURLs) > 0 {
		tr.add("📷", "Mijoz %d ta rasm yubordi — rasm tahlil qilinmaydi, buyurtma raqami so'raladi", len(imgURLs))
	}

	// 4-qadam: xabardagi track raqami yoki mijoz id'si bo'yicha buyurtma
	// holatini ikkinchi saytdan olish.
	orderInfo := a.lookupOrders(&tr, ch, lastText)

	// 5-qadam: kategoriya + buyurtma ma'lumoti bilan javob yozish.
	reply, err := a.ai.Ask(ctx, ai.Request{
		Transcript: transcript,
		Category:   catInfo,
		OrderInfo:  orderInfo,
		HasImage:   len(imgURLs) > 0,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "    AI xatosi:", err)
		return false // keyingi tsiklda qayta urinamiz
	}

	// Xabar tayyor — endi o'qilgan deb belgilaymiz.
	if ids := support.ClientMessageIDs(msgs); len(ids) > 0 {
		_ = support.MarkRead(c, ids)
	}

	// Eskalatsiya markeri bo'lsa xodimlarga.
	if strings.Contains(reply, a.cfg.EscalateMarker) {
		tr.add("🆘", "AI muammoni hal qila olmadi (%s) — xodimlar guruhiga yuborilmoqda", a.cfg.EscalateMarker)
		a.escalate(ctx, ch, lastText, transcript, orderInfo, imgURLs, meter, &tr)
		_ = a.track.Commit(ch.ID, lastID)
		return true
	}

	fmt.Printf("    🤖 %s\n", reply)
	sent, status := false, models.StatusAIDraft
	if a.set.Bool(settings.AutoReply, a.cfg.AutoReply) {
		if _, err := support.SendMessage(c, senderID, ch.ID, "agent", reply); err != nil {
			tr.add("⚠️", "Javob yuborilmadi (xato: %v)", err)
			status = models.StatusFailed
		} else {
			sent, status = true, models.StatusAISent
			tr.add("✅", "AI muammoni o'zi hal qildi va javobni mijozga yubordi")
		}
	} else {
		tr.add("👁", "AI javob yozdi — dashboardda tekshirib tasdiqlashingiz kutilmoqda")
	}
	in := &models.Interaction{
		ClientMessage: lastText, AIReply: reply, Sent: sent,
		Status: status, HandledBy: handledByAI(a.ai.Name()),
		CategoryID: catID,
		ImageCount: len(imgURLs), ImageURLs: strings.Join(imgURLs, "\n"),
	}
	a.fillUsage(in, meter, &tr)
	in.Steps = tr.String() // xarajat qadami ham tarixga tushsin
	a.record(ch, in)
	_ = a.track.Commit(ch.ID, lastID)
	return true
}

// handledByAI — "kim hal qildi" ustuni uchun (masalan "AI (openai gpt-4o-mini)").
func handledByAI(model string) string { return "AI (" + model + ")" }

// modelName — narx jadvalida qidiriladigan model nomi. Provayder nomi
// olib tashlanadi ("openai gpt-4o-mini" → "gpt-4o-mini"), zaxirali zanjirdan
// esa asosiy model olinadi ("ollama llama3.1:8b → openai gpt-4o-mini" →
// "llama3.1:8b").
func (a *app) modelName() string {
	name := a.ai.Name()
	if primary, _, ok := strings.Cut(name, " → "); ok {
		name = primary
	}
	if _, after, ok := strings.Cut(name, " "); ok {
		return after
	}
	return name
}

// fillUsage yozuvga sarflangan tokenlarni va ularning USD qiymatini qo'yadi.
// Tokenlar provayder javobidan olinadi (aniq son); narx noma'lum bo'lsa
// xarajat 0 bo'lib qoladi, lekin tokenlar baribir saqlanadi.
func (a *app) fillUsage(in *models.Interaction, meter *ai.Meter, tr *trace) {
	u := meter.Usage()
	if u.Calls == 0 {
		return
	}
	model := u.Model
	if model == "" {
		model = a.modelName()
	}
	cost, known := pricing.CostOf(model, u.PromptTokens, u.CachedTokens, u.CompletionTokens)

	in.Model = model
	in.PromptTokens, in.CachedTokens = u.PromptTokens, u.CachedTokens
	in.CompletionTokens, in.AICalls, in.CostUSD = u.CompletionTokens, u.Calls, cost

	switch {
	case known && cost == 0:
		tr.add("💰", "%s — lokal model, bepul", u)
	case known:
		tr.add("💰", "%s — $%.6f", u, cost)
	default:
		tr.add("💰", "%s — narx noma'lum (AI_PRICE_IN/AI_PRICE_OUT o'rnatilmagan)", u)
	}
	// Ogohlantirishlar (masalan kontekst to'lib qolgani) — javob berilgan,
	// lekin sifatga ta'sir qilishi mumkin.
	for _, w := range meter.Warnings() {
		tr.add("⚠️", "%s", w)
	}
}

// sendReply admin dashboardda tasdiqlagan javobni mijozga yuboradi.
// web paketi shu funksiya orqali ishlaydi — u Sahiy API'ni bilmaydi.
func (a *app) sendReply(conversationID int64, text string) error {
	c, senderID, err := a.apiClient()
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	if _, err := support.SendMessage(c, senderID, conversationID, "agent", text); err != nil {
		return err
	}
	fmt.Printf("    ✅ Admin tasdiqlagan javob mijozga yuborildi (#%d)\n", conversationID)
	return nil
}

// lookupOrders mijoz xabaridagi track raqami bo'yicha, u bo'lmasa mijoz
// id'si bo'yicha buyurtmalarni ikkinchi saytdan qidiradi.
func (a *app) lookupOrders(tr *trace, ch support.Conversation, lastText string) string {
	if !a.ord.Enabled() {
		tr.add("📦", "Buyurtma qidiruvi o'chiq (SERVICE_PHONE/PASSWORD yo'q)")
		return ""
	}

	// Avval xabardagi track raqamlari — eng aniq qidiruv.
	if tracks := orders.Tracks(lastText); len(tracks) > 0 {
		tr.add("🔎", "Xabardan track raqami topildi: %s", strings.Join(tracks, ", "))
		var all []orders.Order
		for _, t := range tracks {
			list, err := a.ord.ByTrack(t)
			if err != nil {
				tr.add("⚠️", "Track %s qidiruvida xato: %v", t, err)
				continue
			}
			if len(list) == 0 {
				tr.add("❌", "Track %s bo'yicha buyurtma topilmadi", t)
				continue
			}
			tr.add("📦", "Track %s: %d ta buyurtma topildi", t, len(list))
			all = append(all, list...)
		}
		if len(all) > 0 {
			return orders.Summary(all)
		}
	}

	// Track yo'q — mijozning barcha buyurtmalarini ko'ramiz.
	// support.chat.conversation dagi client_id delivery API'dagi user_id
	// bilan bir xil (tekshirilgan: client_id 7911997 → o'sha user_id'dagi
	// buyurtmalar). Yangi qo'shiladigan saytlarda ham shu id ishlatiladi.
	if ch.ClientID == nil || *ch.ClientID == 0 {
		tr.add("📦", "Xabarda track raqami yo'q va mijoz id'si noma'lum — buyurtma ko'rilmadi")
		return ""
	}
	list, err := a.ord.ByUser(*ch.ClientID)
	if err != nil {
		tr.add("⚠️", "Mijoz %d buyurtmalarini olishda xato: %v", *ch.ClientID, err)
		return ""
	}
	if len(list) == 0 {
		tr.add("❌", "Mijoz %d bo'yicha buyurtma topilmadi", *ch.ClientID)
		return ""
	}
	tr.add("📦", "Mijoz %d bo'yicha %d ta buyurtma topildi", *ch.ClientID, len(list))
	return orders.Summary(list)
}

// escalate muammoni xodimlar guruhiga yuboradi. Guruhga faqat oxirgi savol
// emas, butun suhbatdan chiqarilgan umumiy muammo tushuntirishi ketadi.
func (a *app) escalate(ctx context.Context, ch support.Conversation, question, transcript, orderInfo string,
	imgURLs []string, meter *ai.Meter, tr *trace) {
	if a.staff == nil {
		fmt.Fprintf(os.Stderr, "    ⚠️  Eskalatsiya kanali yo'q — xodimlarga yuborilmadi (#%d)\n", ch.ID)
		tr.add("⚠️", "Eskalatsiya kanali yo'q — xodimlarga yuborilmadi")
		fail := &models.Interaction{
			ClientMessage: question, AIReply: "[ESKALATSIYA — kanal yo'q, yuborilmadi]",
			Status:     models.StatusFailed,
			ImageCount: len(imgURLs), ImageURLs: strings.Join(imgURLs, "\n"),
		}
		a.fillUsage(fail, meter, tr)
		fail.Steps = tr.String()
		a.record(ch, fail)
		return
	}

	daraja, summary, err := a.ai.Summarize(ctx, transcript, orderInfo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    xulosa xatosi:", err)
		summary = "(xulosa tayyorlanmadi — suhbatni dashboard'dan ko'ring)"
	}
	tr.add(daraja.Belgi(), "Muammo darajasi: %s", daraja.Sarlavha())

	// Raqamlar backtick bilan belgilanadi — Telegram'da monospace bo'lib
	// chiqadi va bosilganda nusxalanadi.
	// Tizimdan olingan buyurtma va rasm ma'lumoti bo'lsa — xodim ham ko'rsin.
	orderBlock := ""
	if orderInfo != "" {
		orderBlock = "\n📦 Tizimdagi ma'lumot:" + tgtext.MarkNumbers(orderInfo)
	}
	// Rasm havolalari ochiq — xodim Telegram'dan bosib ko'ra oladi.
	if len(imgURLs) > 0 {
		orderBlock += "\n📷 Mijoz rasmlari:\n" + strings.Join(imgURLs, "\n") + "\n"
	}

	// Mijozning oxirgi xabari eng tepada — xodim birinchi shuni ko'radi.
	raw := fmt.Sprintf(
		"%s %s — yordam kerak\nSuhbat #`%d` | Mijoz: %s %s\n\n💬 Mijozning oxirgi xabari:\n%s\n\n📋 Umumiy holat:\n%s\n%s\n↩️ Javob berish uchun shu xabarga REPLY qiling.",
		daraja.Belgi(), daraja.Sarlavha(), ch.ID, ch.ClientName, clientIDLabel(ch.ClientID),
		tgtext.MarkNumbers(question), tgtext.MarkNumbers(summary), orderBlock)
	text, code := tgtext.Build(raw)

	msgID, err := a.staff.Send(text, code)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    telegram yuborish xatosi:", err)
		tr.add("⚠️", "Telegram'ga yuborishda xato: %v", err)
		fail := &models.Interaction{
			ClientMessage: question, AIReply: "[ESKALATSIYA — yuborilmadi: " + err.Error() + "]",
			Status:     models.StatusFailed,
			ImageCount: len(imgURLs), ImageURLs: strings.Join(imgURLs, "\n"),
		}
		a.fillUsage(fail, meter, tr)
		fail.Steps = tr.String()
		a.record(ch, fail)
		return
	}
	var clientID int64
	if ch.ClientID != nil {
		clientID = *ch.ClientID
	}
	// Muammo "jarayonda" holatida saqlanadi — xodim javob berguncha shunday qoladi.
	if err := a.esc.Add(&models.Escalation{
		TgMessageID:    msgID,
		ConversationID: ch.ID,
		ClientID:       clientID,
		ClientName:     ch.ClientName,
		Question:       question,
		Summary:        summary,
		Level:          string(daraja),
		Status:         models.StatusPending,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "    eskalatsiya saqlanmadi:", err)
	}
	tr.add("📨", "Xodimlar guruhiga yuborildi (tg xabar id %d)", msgID)
	tr.add("⏳", "Holat: %s", models.StatusLabel(models.StatusPending))
	in := &models.Interaction{
		ClientMessage: question,
		AIReply:       "[" + daraja.Sarlavha() + " — xodimlar guruhiga yuborildi]\n\n" + summary,
		Status:        models.StatusPending, EscalationID: &msgID,
		ImageCount: len(imgURLs), ImageURLs: strings.Join(imgURLs, "\n"),
	}
	a.fillUsage(in, meter, tr)
	in.Steps = tr.String()
	a.record(ch, in)
}

// clientIDLabel mijoz id'sini "ID `7235`" ko'rinishida qaytaradi
// (id bo'lmasa bo'sh satr). Backtick — nusxalanadigan bo'lak belgisi.
func clientIDLabel(id *int64) string {
	if id == nil || *id == 0 {
		return ""
	}
	return fmt.Sprintf("ID `%d`", *id)
}

// onStaffReply xodim guruhda REPLY qilganda chaqiriladi (userbot yoki bot API).
// Xodim javobi mijozga yuboriladi va muammo holati "jarayonda" dan
// "xodim hal qildi" ga o'tadi.
func (a *app) onStaffReply(replyToMsgID int64, text, from string) {
	item, ok := a.esc.Get(replyToMsgID)
	if !ok {
		return
	}
	if item.Resolved() {
		fmt.Printf("    ℹ️  #%d — bu muammoga allaqachon javob berilgan (%s)\n",
			item.ConversationID, item.AnsweredBy)
		return
	}
	c, senderID, err := a.apiClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "javob token xatosi:", err)
		return
	}
	if _, err := support.SendMessage(c, senderID, item.ConversationID, "agent", text); err != nil {
		fmt.Fprintln(os.Stderr, "javob yuborish xatosi:", err)
		if a.staff != nil {
			warn, code := tgtext.Build(fmt.Sprintf(
				"⚠️ #`%d` — javob mijozga yuborilmadi: %v\nMuammo hali ham jarayonda.",
				item.ConversationID, err))
			a.staff.Send(warn, code)
		}
		return
	}

	by := "Xodim: " + from
	if err := a.esc.Answer(item.TgMessageID, text, by); err != nil {
		fmt.Fprintln(os.Stderr, "eskalatsiya holati yangilanmadi:", err)
	}
	// Dashboarddagi "jarayonda" qatori shu yerda yopiladi (yangi qator emas).
	n, err := a.hist.ResolveEscalation(item.TgMessageID, text, by)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tarix holati yangilanmadi:", err)
	}
	if n == 0 {
		// Eski yozuvlar uchun (escalation_id yo'q edi) — alohida qator.
		_ = a.hist.Append(&models.Interaction{
			ConversationID: item.ConversationID,
			ClientID:       item.ClientID,
			ClientName:     item.ClientName,
			ClientMessage:  item.Question,
			AIReply:        text,
			Sent:           true,
			Status:         models.StatusStaffSent,
			HandledBy:      by,
			EscalationID:   &item.TgMessageID,
			Steps: fmt.Sprintf("1. AI hal qila olmadi — muammo xodimlar guruhiga yuborildi (jarayonda)\n"+
				"2. %s guruhda REPLY qildi\n3. Javob mijozga yuborildi — muammo hal qilindi", by),
		})
	}
	if a.staff != nil {
		done, code := tgtext.Build(fmt.Sprintf(
			"✅ #`%d` — javob mijozga yuborildi (%s)\nHolat: %s",
			item.ConversationID, from, models.StatusLabel(models.StatusStaffSent)))
		a.staff.Send(done, code)
	}
	fmt.Printf("    ✅ Xodim javobi mijozga yuborildi (#%d) — holat: %s\n",
		item.ConversationID, models.StatusLabel(models.StatusStaffSent))
}

// pollBotAPI Bot API rejimida xodim javoblarini poll qiladi.
func (a *app) pollBotAPI(ctx context.Context, bot *telegram.Bot) {
	var offset int64
	for ctx.Err() == nil {
		updates, err := bot.GetUpdates(offset, 50)
		if err != nil {
			fmt.Fprintln(os.Stderr, "telegram poll xatosi:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.ReplyTo == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			from := u.Message.From.FirstName
			if from == "" {
				from = u.Message.From.Username
			}
			a.onStaffReply(u.Message.ReplyTo.MessageID, u.Message.Text, from)
		}
	}
}

// apiClient token oladi va 401 da o'zini yangilaydigan client qaytaradi.
func (a *app) apiClient() (*client.Client, int64, error) {
	token, err := auth.GetToken(a.cfg, a.cachePath)
	if err != nil {
		return nil, 0, err
	}
	c := client.New(a.cfg.BaseURL, token)
	c.Refresh = func() (string, error) { return auth.Refresh(a.cfg, a.cachePath) }
	return c, a.senderID(token), nil
}

// fetchLimit serverdan olinadigan xabarlar soni — kontekst limitidan kam
// bo'lmasligi kerak, aks holda oxirgi N ta xabar to'liq yig'ilmaydi.
func (a *app) fetchLimit() int {
	if a.cfg.HistoryLimit > fetchLimit {
		return a.cfg.HistoryLimit
	}
	return fetchLimit
}

func (a *app) senderID(token string) int64 {
	if a.cfg.AgentSenderID != 0 {
		return a.cfg.AgentSenderID
	}
	if sub, err := auth.Subject(token); err == nil {
		return sub
	}
	return 0
}

// record bitta muammo yozuvini bazaga qo'shadi. Suhbatga oid maydonlarni
// (kim, qaysi chat) o'zi to'ldiradi — chaqiruvchi faqat natijani beradi.
func (a *app) record(ch support.Conversation, in *models.Interaction) {
	if ch.ClientID != nil {
		in.ClientID = *ch.ClientID
	}
	in.ConversationID, in.ClientName, in.Title = ch.ID, ch.ClientName, ch.Title
	if in.Status == "" {
		in.Status = models.StatusAISent
	}
	// Yakunlangan holatlar uchun hal qilingan vaqti yoziladi.
	if in.Status == models.StatusAISent || in.Status == models.StatusStaffSent {
		now := time.Now()
		in.ResolvedAt = &now
	}
	if err := a.hist.Append(in); err != nil {
		fmt.Fprintln(os.Stderr, "    tarixga yozilmadi:", err)
	}
}

// --- staffChannel adapterlari ---

type ubChannel struct {
	bot    *userbot.Bot
	target int64
	ctx    context.Context
	// disabled — sessiya yo'qligi aniqlangach yoqiladi; shundan keyin
	// har eskalatsiyada 15 soniya kutib o'tirilmaydi.
	disabled atomic.Bool
}

func (u *ubChannel) Send(text string, code []tgtext.Span) (int64, error) {
	if u.disabled.Load() {
		return 0, fmt.Errorf("telegram sessiyasi yo'q — `agent login` bajaring")
	}
	return u.bot.SendToGroup(u.ctx, u.target, text, code)
}

type botChannel struct {
	bot    *telegram.Bot
	chatID string
}

func (b *botChannel) Send(text string, code []tgtext.Span) (int64, error) {
	return b.bot.SendMessage(b.chatID, text, code)
}

// escalationTarget eskalatsiya boradigan guruhni aniqlaydi.
func escalationTarget(cfg *config.Config) int64 {
	if cfg.TelegramChatID != "" {
		if id, err := strconv.ParseInt(cfg.TelegramChatID, 10, 64); err == nil {
			return id
		}
	}
	if len(cfg.AllowedGroups) > 0 {
		return cfg.AllowedGroups[0]
	}
	return 0
}

// --- buyruqlar ---

// loginCmd bir martalik Telegram login (sessiyani DATA_DIR ga saqlaydi).
func loginCmd(ctx context.Context, cfg *config.Config) error {
	if cfg.TgAPIID == 0 || cfg.TgAPIHash == "" {
		return fmt.Errorf("API_ID va API_HASH .env da bo'lishi kerak")
	}
	phone := cfg.TgPhone
	if phone == "" {
		var err error
		if phone, err = stdinPrompt("Telefon raqamingiz (+998...): ")(ctx); err != nil {
			return err
		}
	}
	ub := userbot.New(
		cfg.TgAPIID, cfg.TgAPIHash, phone, cfg.TgSession, nil, nil,
		stdinPrompt("Telegram kod (SMS/app): "),
		stdinPrompt("2FA parol (bo'lsa): "),
		false,
	)
	return ub.Login(ctx)
}

// listGroups saqlangan sessiya orqali a'zo bo'lgan guruhlarni chop etadi.
func listGroups(ctx context.Context, cfg *config.Config) error {
	if cfg.TgAPIID == 0 || cfg.TgAPIHash == "" {
		return fmt.Errorf("API_ID va API_HASH .env da bo'lishi kerak")
	}
	// requireSession=true — bu buyruq hech qachon yangi login boshlamaydi.
	ub := userbot.New(
		cfg.TgAPIID, cfg.TgAPIHash, cfg.TgPhone, cfg.TgSession, nil, nil,
		noPrompt("Telegram kod"), noPrompt("2FA parol"), true,
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- ub.Run(ctx) }()

	groups, err := ub.ListGroups(ctx)
	if err != nil {
		select {
		case runErr := <-errCh:
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				return runErr
			}
		default:
		}
		return err
	}

	fmt.Println("\n=== Guruhlar (ALLOWED_GROUPS uchun id) ===")
	for _, g := range groups {
		fmt.Printf("  %-14d  %s  (%s)\n", g.BotAPIID, g.Title, g.Kind)
	}
	fmt.Println("\nKerakli guruh id'sini .env dagi ALLOWED_GROUPS ga yozing.")
	return nil
}

// stdinPrompt terminaldan qiymat o'qiydigan funksiya qaytaradi.
// Interaktiv terminal bo'lmasa (EOF) — xato qaytaradi, bo'sh qiymat emas.
func stdinPrompt(label string) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		fmt.Print(label)
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("interaktiv kirish yo'q (stdin yopiq)")
		}
		v := strings.TrimSpace(sc.Text())
		if v == "" {
			return "", fmt.Errorf("bo'sh qiymat kiritildi")
		}
		return v, nil
	}
}

// noPrompt — hech qachon so'ramaydi (fon rejimida takroriy login urinishlarini
// oldini olish uchun).
func noPrompt(what string) func(ctx context.Context) (string, error) {
	return func(context.Context) (string, error) {
		return "", fmt.Errorf("%s kerak, lekin fon rejimida so'ralmaydi — `agent login` bajaring", what)
	}
}
