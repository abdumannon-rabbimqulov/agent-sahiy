package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sahiy-agent/internal/auth"
	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
	"sahiy-agent/internal/db"
	"sahiy-agent/internal/escalation"
	"sahiy-agent/internal/gemini"
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
	staff     staffChannel // nil bo'lishi mumkin
	cachePath string       // token cache fayli (DataDir ostida)
}

func main() {
	cfg, err := config.Load(envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config xatosi:", err)
		os.Exit(1)
	}

	// `go run . groups` — guruhlar ro'yxatini id bilan chop etadi.
	if len(os.Args) > 1 && os.Args[1] == "groups" {
		listGroups(cfg)
		return
	}

	// Postgres.
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL .env da bo'lishi kerak")
		os.Exit(1)
	}
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "postgres xatosi:", err)
		os.Exit(1)
	}
	defer database.Close()
	fmt.Println("✓ Postgres ulandi")

	a := &app{
		cfg:       cfg,
		ai:        gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.AgentPrompt),
		track:     support.LoadTracker(database),
		hist:      store.New(database),
		esc:       escalation.New(database),
		cachePath: filepath.Join(cfg.DataDir, "token.json"),
	}

	// Web dashboard.
	srv := web.New(a.hist, cfg.WebAddr)
	go func() {
		fmt.Printf("🌐 Dashboard: http://localhost%s\n", cfg.WebAddr)
		if err := srv.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "web server xatosi:", err)
		}
	}()

	// Telegram: userbot (afzal) yoki Bot API.
	a.setupTelegram(context.Background())

	if st, err := a.hist.Stats(); err == nil {
		fmt.Printf("📊 Tarix: %s\n", st)
	}
	fmt.Printf("🚀 Agent ishga tushdi. Har %s da tekshiradi.\n", pollInterval)

	a.runCycle()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		a.runCycle()
	}
}

// setupTelegram userbot yoki Bot API kanalini sozlaydi.
func (a *app) setupTelegram(ctx context.Context) {
	cfg := a.cfg

	// 1) Userbot (MTProto) — API_ID/API_HASH va ALLOWED_GROUPS bo'lsa.
	if cfg.TgAPIID != 0 && cfg.TgAPIHash != "" && len(cfg.AllowedGroups) > 0 {
		target := escalationTarget(cfg)
		ub := userbot.New(
			cfg.TgAPIID, cfg.TgAPIHash, cfg.TgPhone, cfg.TgSession,
			cfg.AllowedGroups,
			a.onStaffReply, // reply -> mijozga
			stdinPrompt("Telegram kod (SMS/app): "),
			stdinPrompt("2FA parol (bo'lsa): "),
		)
		go func() {
			if err := ub.Run(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "userbot xatosi:", err)
			}
		}()
		a.staff = &ubChannel{bot: ub, target: target, ctx: ctx}
		fmt.Printf("📨 Userbot eskalatsiya yoqilgan (guruh %d)\n", target)
		return
	}

	// 2) Bot API — TELEGRAM_TOKEN bo'lsa.
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		bot := telegram.New(cfg.TelegramToken)
		a.staff = &botChannel{bot: bot, chatID: cfg.TelegramChatID}
		go a.pollBotAPI(bot)
		fmt.Println("📨 Bot API eskalatsiya yoqilgan")
	}
}

// runCycle bitta tekshirish tsikli.
func (a *app) runCycle() {
	ts := time.Now().Format("15:04:05")

	token, err := auth.GetToken(a.cfg, a.cachePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] token xatosi: %v\n", ts, err)
		return
	}
	senderID := a.senderID(token)
	c := client.New(a.cfg.BaseURL, token)

	chats, err := support.FetchConversations(c, 1, 20, support.FilterRequest{
		Type: "client", State: []int{1, 2, 3},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] chatlar xatosi: %v\n", ts, err)
		return
	}

	fresh := a.track.New(chats)
	if len(fresh) == 0 {
		fmt.Printf("[%s] yangi chat yo'q (jami %d)\n", ts, len(chats))
		return
	}
	fmt.Printf("[%s] 🔔 %d ta yangi chat\n", ts, len(fresh))

	for _, ch := range fresh {
		a.handleChat(c, senderID, ch)
	}
	if err := a.track.Commit(chats); err != nil {
		fmt.Fprintln(os.Stderr, "    tracker saqlanmadi:", err)
	}
	if st, err := a.hist.Stats(); err == nil {
		fmt.Printf("[%s] 📊 %s\n", ts, st)
	}
}

func (a *app) handleChat(c *client.Client, senderID int64, ch support.Conversation) {
	fmt.Printf("  === #%d | %s | %q ===\n", ch.ID, ch.ClientName, ch.Title)

	msgs, err := support.FetchMessages(c, ch.ID, 1, 50)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    xabarlar xatosi:", err)
		return
	}
	if ids := support.ClientMessageIDs(msgs); len(ids) > 0 {
		_ = support.MarkRead(c, ids)
	}

	if a.cfg.GeminiAPIKey == "" {
		fmt.Println("    (GEMINI_API_KEY yo'q)")
		return
	}
	reply, err := a.ai.Ask(support.Transcript(msgs))
	if err != nil {
		fmt.Fprintln(os.Stderr, "    gemini xatosi:", err)
		return
	}

	// Eskalatsiya markeri bo'lsa xodimlarga.
	if a.staff != nil && strings.Contains(reply, a.cfg.EscalateMarker) {
		a.escalate(ch, lastClientMessage(msgs))
		return
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
	a.record(ch, lastClientMessage(msgs), reply, sent)
}

// escalate muammoni xodimlar guruhiga yuboradi.
func (a *app) escalate(ch support.Conversation, question string) {
	text := fmt.Sprintf("🆘 Yordam kerak\nSuhbat #%d\nMijoz: %s\nSavol: %s\n\n↩️ Javob berish uchun shu xabarga REPLY qiling.",
		ch.ID, ch.ClientName, question)
	msgID, err := a.staff.Send(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "    telegram yuborish xatosi:", err)
		return
	}
	_ = a.esc.Add(escalation.Item{
		TgMessageID:    msgID,
		ConversationID: ch.ID,
		ClientName:     ch.ClientName,
		Question:       question,
	})
	a.record(ch, question, "[ESKALATSIYA → xodimlar guruhi]", false)
	fmt.Printf("    📨 Xodimlar guruhiga yuborildi (#%d, tg=%d)\n", ch.ID, msgID)
}

// onStaffReply xodim guruhda REPLY qilganda chaqiriladi (userbot yoki bot API).
func (a *app) onStaffReply(replyToMsgID int64, text, from string) {
	item, ok := a.esc.Get(replyToMsgID)
	if !ok || item.Resolved {
		return
	}
	token, err := auth.GetToken(a.cfg, a.cachePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "javob token xatosi:", err)
		return
	}
	c := client.New(a.cfg.BaseURL, token)
	if _, err := support.SendMessage(c, a.senderID(token), item.ConversationID, "agent", text); err != nil {
		fmt.Fprintln(os.Stderr, "javob yuborish xatosi:", err)
		return
	}
	_ = a.esc.Resolve(item.TgMessageID, text)
	if a.staff != nil {
		a.staff.Send(fmt.Sprintf("✅ #%d — javob mijozga yuborildi (%s)", item.ConversationID, from))
	}
	_ = a.hist.Append(store.Interaction{
		ConversationID: item.ConversationID,
		ClientName:     item.ClientName,
		ClientMessage:  item.Question,
		AIReply:        fmt.Sprintf("[Xodim %s] %s", from, text),
		Sent:           true,
	})
	fmt.Printf("    ✅ Xodim javobi mijozga yuborildi (#%d)\n", item.ConversationID)
}

// pollBotAPI Bot API rejimida xodim javoblarini poll qiladi.
func (a *app) pollBotAPI(bot *telegram.Bot) {
	var offset int64
	for {
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

func (a *app) senderID(token string) int64 {
	if a.cfg.AgentSenderID != 0 {
		return a.cfg.AgentSenderID
	}
	if sub, err := auth.Subject(token); err == nil {
		return sub
	}
	return 0
}

func (a *app) record(ch support.Conversation, clientMsg, reply string, sent bool) {
	var clientID int64
	if ch.ClientID != nil {
		clientID = *ch.ClientID
	}
	_ = a.hist.Append(store.Interaction{
		ConversationID: ch.ID, ClientID: clientID, ClientName: ch.ClientName,
		Title: ch.Title, ClientMessage: clientMsg, AIReply: reply, Sent: sent,
	})
}

func lastClientMessage(msgs []support.Message) string {
	var last string
	var maxID int64 = -1
	for _, m := range msgs {
		if m.SenderType == "client" && m.ID > maxID {
			maxID, last = m.ID, m.Message
		}
	}
	return last
}

// --- staffChannel adapterlari ---

type ubChannel struct {
	bot    *userbot.Bot
	target int64
	ctx    context.Context
}

func (u *ubChannel) Send(text string) (int64, error) {
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

// stdinPrompt terminaldan qiymat o'qiydigan funksiya qaytaradi.
func stdinPrompt(label string) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		fmt.Print(label)
		sc := bufio.NewScanner(os.Stdin)
		if sc.Scan() {
			return strings.TrimSpace(sc.Text()), nil
		}
		return "", sc.Err()
	}
}

// listGroups userbotga ulanib, a'zo bo'lgan guruhlarni id bilan chop etadi.
func listGroups(cfg *config.Config) {
	if cfg.TgAPIID == 0 || cfg.TgAPIHash == "" {
		fmt.Fprintln(os.Stderr, "API_ID va API_HASH .env da bo'lishi kerak")
		os.Exit(1)
	}
	ctx := context.Background()
	phone := cfg.TgPhone
	if phone == "" {
		phone, _ = stdinPrompt("Telefon raqamingiz (+998...): ")(ctx)
	}
	ub := userbot.New(
		cfg.TgAPIID, cfg.TgAPIHash, phone, cfg.TgSession,
		nil, nil,
		stdinPrompt("Telegram kod (SMS/app): "),
		stdinPrompt("2FA parol (bo'lsa): "),
	)
	go func() {
		if err := ub.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "userbot xatosi:", err)
		}
	}()

	groups, err := ub.ListGroups(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "guruhlarni olish xatosi:", err)
		os.Exit(1)
	}
	fmt.Println("\n=== Guruhlar (ALLOWED_GROUPS uchun id) ===")
	for _, g := range groups {
		fmt.Printf("  %-14d  %s  (%s)\n", g.BotAPIID, g.Title, g.Kind)
	}
	fmt.Println("\nKerakli guruh id'sini .env dagi ALLOWED_GROUPS ga yozing.")
}
