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
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/store"
	"sahiy-agent/internal/support"
	"sahiy-agent/internal/telegram"
	"sahiy-agent/internal/userbot"
	"sahiy-agent/internal/web"
)

const (
	envPath      = ".env"
	pollInterval = 30 * time.Second
)

// staffChannel — xodimlar guruhiga xabar yuboradigan kanal
// (userbot yoki Bot API).
type staffChannel interface {
	Send(text string) (int64, error)
}

type app struct {
	cfg       *config.Config
	ai        *gemini.Client
	track     *support.Tracker
	hist      *store.Store
	esc       *escalation.Store
	cats      *category.Store
	staff     staffChannel // nil bo'lishi mumkin
	cachePath string       // token cache fayli (DataDir ostida)
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
	}

	// Web dashboard.
	srv := web.New(a.hist, a.cats, web.Options{
		Addr:      cfg.WebAddr,
		AdminUser: cfg.AdminUser,
		AdminPass: cfg.AdminPass,
		Dev:       cfg.WebDev,
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
		msgs, err := support.FetchMessages(c, ch.ID, 1, 50)
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

	msgs, err := support.FetchMessages(c, ch.ID, 1, 50)
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

	transcript := support.Transcript(msgs)

	// 1-qadam: mos kategoriyani aniqlash.
	var (
		catID   *uint
		catInfo string
	)
	if catalog != "" {
		id, err := a.ai.Classify(ctx, catalog, transcript)
		if err != nil {
			fmt.Fprintln(os.Stderr, "    klassifikatsiya xatosi:", err)
		} else if id > 0 {
			if cat, err := a.cats.Get(id); err == nil && cat.Active {
				catID, catInfo = &cat.ID, cat.Content
				fmt.Printf("    🏷  kategoriya: %d (%s)\n", cat.ID, cat.Name)
			}
		}
	}

	// 2-qadam: kategoriya ma'lumoti bilan javob yozish.
	reply, err := a.ai.Ask(ctx, transcript, catInfo)
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
		a.escalate(ch, lastText)
		_ = a.track.Commit(ch.ID, lastID)
		return true
	}

	fmt.Printf("    🤖 %s\n", reply)
	sent := false
	if a.cfg.AutoReply {
		if _, err := support.SendMessage(c, senderID, ch.ID, "agent", reply); err != nil {
			fmt.Fprintln(os.Stderr, "    yuborish xatosi:", err)
		} else {
			sent = true
			fmt.Println("    ✓ Javob yuborildi")
		}
	}
	a.record(ch, lastText, reply, sent, catID)
	_ = a.track.Commit(ch.ID, lastID)
	return true
}

// escalate muammoni xodimlar guruhiga yuboradi.
func (a *app) escalate(ch support.Conversation, question string) {
	text := fmt.Sprintf("🆘 Yordam kerak\nSuhbat #%d\nMijoz: %s\nSavol: %s\n\n↩️ Javob berish uchun shu xabarga REPLY qiling.",
		ch.ID, ch.ClientName, question)

	if a.staff == nil {
		fmt.Fprintf(os.Stderr, "    ⚠️  Eskalatsiya kanali yo'q — xodimlarga yuborilmadi (#%d)\n", ch.ID)
		a.record(ch, question, "[ESKALATSIYA — kanal yo'q, yuborilmadi]", false, nil)
		return
	}

	msgID, err := a.staff.Send(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    telegram yuborish xatosi:", err)
		a.record(ch, question, "[ESKALATSIYA — yuborilmadi: "+err.Error()+"]", false, nil)
		return
	}
	_ = a.esc.Add(&models.Escalation{
		TgMessageID:    msgID,
		ConversationID: ch.ID,
		ClientName:     ch.ClientName,
		Question:       question,
	})
	a.record(ch, question, "[ESKALATSIYA → xodimlar guruhi]", false, nil)
	fmt.Printf("    📨 Xodimlar guruhiga yuborildi (#%d, tg=%d)\n", ch.ID, msgID)
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
		a.staff.Send(fmt.Sprintf("✅ #%d — javob mijozga yuborildi (%s)", item.ConversationID, from))
	}
	_ = a.hist.Append(&models.Interaction{
		ConversationID: item.ConversationID,
		ClientName:     item.ClientName,
		ClientMessage:  item.Question,
		AIReply:        fmt.Sprintf("[Xodim %s] %s", from, text),
		Sent:           true,
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

func (a *app) senderID(token string) int64 {
	if a.cfg.AgentSenderID != 0 {
		return a.cfg.AgentSenderID
	}
	if sub, err := auth.Subject(token); err == nil {
		return sub
	}
	return 0
}

func (a *app) record(ch support.Conversation, clientMsg, reply string, sent bool, catID *uint) {
	var clientID int64
	if ch.ClientID != nil {
		clientID = *ch.ClientID
	}
	_ = a.hist.Append(&models.Interaction{
		ConversationID: ch.ID, ClientID: clientID, ClientName: ch.ClientName,
		Title: ch.Title, ClientMessage: clientMsg, AIReply: reply,
		Sent: sent, CategoryID: catID,
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

func (u *ubChannel) Send(text string) (int64, error) {
	if u.disabled.Load() {
		return 0, fmt.Errorf("telegram sessiyasi yo'q — `agent login` bajaring")
	}
	return u.bot.SendToGroup(u.ctx, u.target, text)
}

type botChannel struct {
	bot    *telegram.Bot
	chatID string
}

func (b *botChannel) Send(text string) (int64, error) {
	return b.bot.SendMessage(b.chatID, text)
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
