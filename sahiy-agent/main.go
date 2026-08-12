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

	"sahiy-agent/internal/auth"
	"sahiy-agent/internal/category"
	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
	"sahiy-agent/internal/db"
	"sahiy-agent/internal/escalation"
	"sahiy-agent/internal/gemini"
	"sahiy-agent/internal/images"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/orders"
	"sahiy-agent/internal/service"
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
	// maxImages — bitta tsiklda tahlil qilinadigan rasmlar soni.
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
	ai        *gemini.Client
	track     *support.Tracker
	hist      *store.Store
	esc       *escalation.Store
	cats      *category.Store
	ord       *orders.Lookup // buyurtma holati (ikkinchi sayt)
	staff     staffChannel   // nil bo'lishi mumkin
	cachePath string         // token cache fayli (DataDir ostida)
	imageDir  string         // chat rasmlari (DataDir/images)
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
		ai:        gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.AgentPrompt),
		track:     support.NewTracker(database),
		hist:      store.New(database),
		esc:       escalation.New(database),
		cats:      category.New(database),
		cachePath: filepath.Join(cfg.DataDir, "token.json"),
		imageDir:  filepath.Join(cfg.DataDir, "images"),
	}

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

	// Web dashboard.
	srv := web.New(a.hist, a.cats, web.Options{
		Addr:      cfg.WebAddr,
		AdminUser: cfg.AdminUser,
		AdminPass: cfg.AdminPass,
		Dev:       cfg.WebDev,
		MediaDir:  a.imageDir,
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

	fmt.Printf("  === #%d | %s | %q ===\n", ch.ID, ch.ClientName, ch.Title)

	if a.cfg.GeminiAPIKey == "" {
		fmt.Println("    (GEMINI_API_KEY yo'q)")
		return false
	}

	// tr — agent nimani qanday tushunganini bosqichma-bosqich yozib boradi;
	// oxirida bazaga tushadi va dashboardda ko'rinadi.
	var tr trace

	// 1-qadam: suhbatning oxirgi HISTORY_LIMIT ta xabari (kamida
	// support.MinHistory) o'qiladi.
	transcript, studied := support.TranscriptTail(msgs, a.cfg.HistoryLimit)
	tr.add("📚", "Suhbatning oxirgi %d ta xabari o'qildi (limit %d)", studied, a.cfg.HistoryLimit)

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

	// 3-qadam: mijoz yuborgan rasmlarni saqlash va tahlil qilish.
	imgInfo, imgNumbers := a.handleImages(ctx, &tr, ch, msgs)

	// 4-qadam: xabardagi (va rasmdagi) track raqami yoki mijoz id'si
	// bo'yicha buyurtma holatini ikkinchi saytdan olish.
	orderInfo := a.lookupOrders(&tr, ch, lastText, imgNumbers)

	// 5-qadam: kategoriya + rasm + buyurtma ma'lumoti bilan javob yozish.
	reply, err := a.ai.Ask(ctx, transcript, catInfo, orderInfo+imgInfo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    gemini xatosi:", err)
		return false // keyingi tsiklda qayta urinamiz
	}

	// Xabar tayyor — endi o'qilgan deb belgilaymiz.
	if ids := support.ClientMessageIDs(msgs); len(ids) > 0 {
		_ = support.MarkRead(c, ids)
	}

	// Eskalatsiya markeri bo'lsa xodimlarga.
	if strings.Contains(reply, a.cfg.EscalateMarker) {
		tr.add("🆘", "AI muammoni hal qila olmadi (%s) — xodimlar guruhiga yuborilmoqda", a.cfg.EscalateMarker)
		a.escalate(ctx, ch, lastText, transcript, orderInfo+imgInfo, &tr)
		_ = a.track.Commit(ch.ID, lastID)
		return true
	}

	fmt.Printf("    🤖 %s\n", reply)
	sent := false
	if a.cfg.AutoReply {
		if _, err := support.SendMessage(c, senderID, ch.ID, "agent", reply); err != nil {
			tr.add("⚠️", "Javob yuborilmadi (xato: %v)", err)
		} else {
			sent = true
			tr.add("✅", "AI javob yozdi va mijozga yuborildi")
		}
	} else {
		tr.add("👁", "AI javob yozdi, lekin yuborilmadi (AUTO_REPLY=false)")
	}
	a.record(ch, lastText, reply, sent, catID, tr.String())
	_ = a.track.Commit(ch.ID, lastID)
	return true
}

// lookupOrders mijoz xabaridagi track raqami bo'yicha, u bo'lmasa mijoz
// id'si bo'yicha buyurtmalarni ikkinchi saytdan qidiradi.
func (a *app) lookupOrders(tr *trace, ch support.Conversation, lastText string, extra []string) string {
	if !a.ord.Enabled() {
		tr.add("📦", "Buyurtma qidiruvi o'chiq (SERVICE_PHONE/PASSWORD yo'q)")
		return ""
	}

	// Avval xabardagi (va rasmdan olingan) track raqamlari — eng aniq qidiruv.
	if tracks := orders.Tracks(lastText + " " + strings.Join(extra, " ")); len(tracks) > 0 {
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
func (a *app) escalate(ctx context.Context, ch support.Conversation, question, transcript, orderInfo string, tr *trace) {
	if a.staff == nil {
		fmt.Fprintf(os.Stderr, "    ⚠️  Eskalatsiya kanali yo'q — xodimlarga yuborilmadi (#%d)\n", ch.ID)
		tr.add("⚠️", "Eskalatsiya kanali yo'q — xodimlarga yuborilmadi")
		a.record(ch, question, "[ESKALATSIYA — kanal yo'q, yuborilmadi]", false, nil, tr.String())
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
	if urls := a.imageURLs(ch.ID); len(urls) > 0 {
		orderBlock += "\n📷 Mijoz rasmlari:\n" + strings.Join(urls, "\n") + "\n"
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
		a.record(ch, question, "[ESKALATSIYA — yuborilmadi: "+err.Error()+"]", false, nil, tr.String())
		return
	}
	_ = a.esc.Add(&models.Escalation{
		TgMessageID:    msgID,
		ConversationID: ch.ID,
		ClientName:     ch.ClientName,
		Question:       question,
	})
	tr.add("📨", "Xodimlar guruhiga yuborildi (tg xabar id %d)", msgID)
	a.record(ch, question, "[ESKALATSIYA "+daraja.Sarlavha()+" → xodimlar guruhi]", false, nil, tr.String())
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
func (a *app) onStaffReply(replyToMsgID int64, text, from string) {
	item, ok := a.esc.Get(replyToMsgID)
	if !ok || item.Resolved {
		return
	}
	c, senderID, err := a.apiClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "javob token xatosi:", err)
		return
	}
	if _, err := support.SendMessage(c, senderID, item.ConversationID, "agent", text); err != nil {
		fmt.Fprintln(os.Stderr, "javob yuborish xatosi:", err)
		return
	}
	_ = a.esc.Resolve(item.TgMessageID, text)
	if a.staff != nil {
		done, code := tgtext.Build(fmt.Sprintf("✅ #`%d` — javob mijozga yuborildi (%s)", item.ConversationID, from))
		a.staff.Send(done, code)
	}
	_ = a.hist.Append(&models.Interaction{
		ConversationID: item.ConversationID,
		ClientName:     item.ClientName,
		ClientMessage:  item.Question,
		AIReply:        fmt.Sprintf("[Xodim %s] %s", from, text),
		Sent:           true,
		Steps:          fmt.Sprintf("1. Eskalatsiya guruhga yuborilgan edi\n2. Xodim %s guruhda REPLY qildi\n3. Javob mijozga yuborildi", from),
	})
	fmt.Printf("    ✅ Xodim javobi mijozga yuborildi (#%d)\n", item.ConversationID)
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

func (a *app) record(ch support.Conversation, clientMsg, reply string, sent bool, catID *uint, steps string) {
	var clientID int64
	if ch.ClientID != nil {
		clientID = *ch.ClientID
	}
	_ = a.hist.Append(&models.Interaction{
		ConversationID: ch.ID, ClientID: clientID, ClientName: ch.ClientName,
		Title: ch.Title, ClientMessage: clientMsg, AIReply: reply,
		Sent: sent, CategoryID: catID, Steps: steps,
	})
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

// handleImages mijoz yuborgan rasmlarni yuklab oladi, Gemini bilan tahlil
// qiladi va bazaga saqlaydi. Qaytaradi: Gemini promptiga qo'shiladigan matn
// va rasmlardan ajratilgan raqamlar.
//
// Bir marta ishlangan rasm (message_id) qayta yuklanmaydi va qayta tahlil
// qilinmaydi — natija bazadan olinadi.
func (a *app) handleImages(ctx context.Context, tr *trace, ch support.Conversation,
	msgs []support.Message) (string, []string) {

	imgs := support.ImageMessages(msgs, maxImages)
	if len(imgs) == 0 {
		return "", nil
	}
	tr.add("📷", "Suhbatda %d ta mijoz rasmi bor (%d tasi ko'riladi)",
		len(support.ImageMessages(msgs, 0)), len(imgs))

	var (
		b       strings.Builder
		numbers []string
	)
	for _, m := range imgs {
		rec := a.processImage(ctx, tr, ch, m)
		if rec == nil {
			continue
		}
		fmt.Fprintf(&b, "\n### Rasm (xabar %d)\n%s\n", rec.MessageID, rec.Analysis)
		if rec.Numbers != "" {
			fmt.Fprintf(&b, "Rasmdagi raqamlar: %s\n", rec.Numbers)
			numbers = append(numbers, strings.Split(rec.Numbers, ",")...)
		}
	}
	if b.Len() == 0 {
		return "", nil
	}
	return "\n\n--- Mijoz yuborgan rasmlar (tahlil qilingan) ---" + b.String(), numbers
}

// processImage bitta rasmni keshdan oladi yoki yuklab olib tahlil qiladi.
// Xatolik bo'lsa nil qaytaradi — javob yozish baribir davom etadi.
func (a *app) processImage(ctx context.Context, tr *trace, ch support.Conversation,
	m support.Message) *models.ChatImage {

	if rec, ok := a.hist.GetImage(m.ID); ok {
		tr.add("💾", "Rasm %d keshdan olindi (qayta yuklanmadi)", m.ID)
		return rec
	}

	res, err := images.Download(m.Message, a.imageDir, ch.ID, m.ID)
	if err != nil {
		tr.add("⚠️", "Rasm %d yuklanmadi: %v", m.ID, err)
		return nil
	}
	tr.add("⬇️", "Rasm %d yuklab olindi (%d KB)", m.ID, res.Size/1024)

	out, err := a.ai.DescribeImage(ctx, res.Mime, res.Data)
	if err != nil {
		tr.add("⚠️", "Rasm %d tahlil qilinmadi: %v", m.ID, err)
		return nil
	}
	nums, desc := gemini.ParseImageAnswer(out)
	if len(nums) > 0 {
		tr.add("🔎", "Rasmda topildi: %s", strings.Join(nums, ", "))
	} else {
		tr.add("🖼", "Rasmda raqam yo'q — %s", desc)
	}

	var clientID int64
	if ch.ClientID != nil {
		clientID = *ch.ClientID
	}
	rec := &models.ChatImage{
		MessageID:      m.ID,
		ConversationID: ch.ID,
		ClientID:       clientID,
		URL:            m.Message,
		Path:           res.Path,
		MimeType:       res.Mime,
		SizeBytes:      res.Size,
		Analysis:       desc,
		Numbers:        strings.Join(nums, ","),
	}
	if err := a.hist.SaveImage(rec); err != nil {
		tr.add("⚠️", "Rasm %d bazaga yozilmadi: %v", m.ID, err)
	}
	return rec
}

// imageURLs suhbatdagi saqlangan rasmlarning asl havolalarini qaytaradi
// (eskalatsiya xabarida xodimga ko'rsatish uchun).
func (a *app) imageURLs(conversationID int64) []string {
	all, err := a.hist.RecentImages(200)
	if err != nil {
		return nil
	}
	var out []string
	for _, im := range all {
		if im.ConversationID == conversationID {
			out = append(out, im.URL)
			if len(out) >= maxImages {
				break
			}
		}
	}
	return out
}
