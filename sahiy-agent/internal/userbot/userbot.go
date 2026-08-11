// Package userbot — haqiqiy MTProto userbot (foydalanuvchi akkaunti) gotd/td orqali.
package userbot

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// ErrNoSession — saqlangan Telegram sessiyasi yo'q.
// Bu holda `agent login` buyrug'i bir marta bajarilishi kerak.
var ErrNoSession = errors.New("telegram sessiyasi yo'q — avval `agent login` bajaring")

// ReplyHandler — guruhda kimdir bizning xabarimizga REPLY qilganda chaqiriladi.
// replyToMsgID — biz yuborgan (eskalatsiya) xabar id'si.
type ReplyHandler func(replyToMsgID int64, text, fromName string)

// CodePrompt — birinchi kirishda Telegram kodini so'rash uchun (odatda stdin).
type CodePrompt func(ctx context.Context) (string, error)

// Bot — userbot.
type Bot struct {
	apiID       int
	apiHash     string
	phone       string
	sessionPath string
	allowed     map[int64]bool // normallashtirilgan (musbat) guruh id'lari
	onReply     ReplyHandler
	codePrompt  CodePrompt
	passwordFn  func(ctx context.Context) (string, error)

	dispatcher tg.UpdateDispatcher
	client     *telegram.Client

	ready     chan struct{}
	readyOnce sync.Once
	// requireSession — true bo'lsa saqlangan sessiya bo'lmasa kod so'ramaydi
	// (Telegram'ga takroriy login urinishlari yubormaslik uchun).
	requireSession bool
	mu             sync.Mutex
	api            *tg.Client
	peers          map[int64]tg.InputPeerClass // guruh id (musbat) -> input peer
}

// New yangi userbot yaratadi.
func New(apiID int, apiHash, phone, sessionPath string, allowedGroups []int64,
	onReply ReplyHandler, code CodePrompt, password func(ctx context.Context) (string, error),
	requireSession bool) *Bot {

	allowed := map[int64]bool{}
	for _, g := range allowedGroups {
		allowed[normalizeID(g)] = true
	}
	return &Bot{
		apiID:          apiID,
		apiHash:        apiHash,
		phone:          phone,
		sessionPath:    sessionPath,
		allowed:        allowed,
		onReply:        onReply,
		codePrompt:     code,
		passwordFn:     password,
		requireSession: requireSession,
		dispatcher:     tg.NewUpdateDispatcher(),
		ready:          make(chan struct{}),
		peers:          map[int64]tg.InputPeerClass{},
	}
}

// Run userbotni ishga tushiradi va ulanib turadi (bloklaydi).
// Odatda goroutine ichida chaqiriladi.
func (b *Bot) Run(ctx context.Context) error {
	b.client = telegram.NewClient(b.apiID, b.apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: b.sessionPath},
		UpdateHandler:  b.dispatcher,
	})

	b.dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		b.handleMessage(u.Message)
		return nil
	})
	b.dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		b.handleMessage(u.Message)
		return nil
	})

	return b.client.Run(ctx, func(ctx context.Context) error {
		// Saqlangan sessiya bo'lmasa — kod so'ramaymiz. Aks holda har
		// restartda Telegram'ga yangi login urinishi ketadi (FLOOD_WAIT xavfi).
		if b.requireSession {
			st, err := b.client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("userbot auth holati: %w", err)
			}
			if !st.Authorized {
				return ErrNoSession
			}
		}

		flow := auth.NewFlow(
			termAuth{phone: b.phone, code: b.codePrompt, password: b.passwordFn},
			auth.SendCodeOptions{},
		)
		if err := b.client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("userbot auth: %w", err)
		}

		b.mu.Lock()
		b.api = b.client.API()
		b.mu.Unlock()

		if err := b.loadDialogs(ctx); err != nil {
			fmt.Println("userbot: dialoglarni yuklashda ogohlantirish:", err)
		}
		b.readyOnce.Do(func() { close(b.ready) })
		fmt.Println("✓ Userbot ulandi")

		<-ctx.Done()
		return ctx.Err()
	})
}

// SendToGroup guruhga matn yuboradi va xabar id'sini qaytaradi.
func (b *Bot) SendToGroup(ctx context.Context, groupID int64, text string) (int64, error) {
	select {
	case <-b.ready:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(15 * time.Second):
		return 0, fmt.Errorf("userbot tayyor emas (timeout)")
	}

	peer := b.peer(normalizeID(groupID))
	if peer == nil {
		// Qayta dialog yuklab ko'rish.
		_ = b.loadDialogs(ctx)
		peer = b.peer(normalizeID(groupID))
	}
	if peer == nil {
		return 0, fmt.Errorf("guruh topilmadi (id=%d) — userbot shu guruhda a'zomi?", groupID)
	}

	b.mu.Lock()
	api := b.api
	b.mu.Unlock()

	updates, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: randomID(),
	})
	if err != nil {
		return 0, fmt.Errorf("xabar yuborish: %w", err)
	}
	return extractMessageID(updates), nil
}

// handleMessage kiruvchi xabarni tekshiradi: bizning xabarimizga reply bo'lsa
// onReply chaqiradi.
func (b *Bot) handleMessage(msg tg.MessageClass) {
	m, ok := msg.(*tg.Message)
	if !ok {
		return
	}
	gid := peerID(m.PeerID)
	if gid == 0 || !b.allowed[gid] {
		return
	}
	rh, ok := m.GetReplyTo()
	if !ok {
		return
	}
	rhh, ok := rh.(*tg.MessageReplyHeader)
	if !ok || rhh.ReplyToMsgID == 0 {
		return
	}
	text := m.Message
	if text == "" {
		return
	}
	from := senderName(m.FromID)
	if b.onReply != nil {
		b.onReply(int64(rhh.ReplyToMsgID), text, from)
	}
}

// loadDialogs guruh peer'larini (access hash bilan) keshga yuklaydi.
func (b *Bot) loadDialogs(ctx context.Context) error {
	b.mu.Lock()
	api := b.api
	b.mu.Unlock()
	if api == nil {
		return fmt.Errorf("api tayyor emas")
	}

	res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      200,
	})
	if err != nil {
		return err
	}

	var chats []tg.ChatClass
	switch d := res.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range chats {
		switch c := ch.(type) {
		case *tg.Channel:
			b.peers[c.ID] = &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}
		case *tg.Chat:
			b.peers[c.ID] = &tg.InputPeerChat{ChatID: c.ID}
		}
	}
	return nil
}

func (b *Bot) peer(id int64) tg.InputPeerClass {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peers[id]
}

// --- yordamchilar ---

// termAuth — auth.UserAuthenticator.
type termAuth struct {
	phone    string
	code     CodePrompt
	password func(ctx context.Context) (string, error)
}

func (a termAuth) Phone(_ context.Context) (string, error) { return a.phone, nil }
func (a termAuth) Password(ctx context.Context) (string, error) {
	if a.password == nil {
		return "", fmt.Errorf("2FA parol kerak, lekin berilmagan")
	}
	return a.password(ctx)
}
func (a termAuth) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	return a.code(ctx)
}
func (a termAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error { return nil }
func (a termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("ro'yxatdan o'tish qo'llab-quvvatlanmaydi")
}

// normalizeID -100xxxxxxxxxx yoki -xxxx ni musbat "raw" id ga aylantiradi.
func normalizeID(id int64) int64 {
	if id < 0 {
		s := fmt.Sprintf("%d", -id)
		if len(s) > 3 && s[:3] == "100" {
			var raw int64
			fmt.Sscanf(s[3:], "%d", &raw)
			return raw
		}
		return -id
	}
	return id
}

func peerID(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerChannel:
		return v.ChannelID
	case *tg.PeerChat:
		return v.ChatID
	}
	return 0
}

func senderName(p tg.PeerClass) string {
	switch v := p.(type) {
	case *tg.PeerUser:
		return fmt.Sprintf("user:%d", v.UserID)
	}
	return "xodim"
}

func randomID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]))
}

// extractMessageID yuborilgan xabarning id'sini Updates ichidan oladi.
func extractMessageID(u tg.UpdatesClass) int64 {
	var ups []tg.UpdateClass
	switch v := u.(type) {
	case *tg.Updates:
		ups = v.Updates
	case *tg.UpdatesCombined:
		ups = v.Updates
	}
	for _, up := range ups {
		if mid, ok := up.(*tg.UpdateMessageID); ok {
			return int64(mid.ID)
		}
	}
	return 0
}

// Group — dialoglardagi guruh (ro'yxat uchun).
type Group struct {
	BotAPIID int64  // -100... ko'rinishidagi id (ALLOWED_GROUPS ga yoziladi)
	RawID    int64  // ichki (musbat) id
	Title    string // guruh nomi
	Kind     string // "supergroup/channel" yoki "group"
}

// ListGroups akkaunt a'zo bo'lgan guruhlarni qaytaradi.
// Ready bo'lguncha kutadi (Run goroutine'da ishlab turishi kerak).
func (b *Bot) ListGroups(ctx context.Context) ([]Group, error) {
	select {
	case <-b.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("userbot tayyor emas (timeout)")
	}

	b.mu.Lock()
	api := b.api
	b.mu.Unlock()

	res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      200,
	})
	if err != nil {
		return nil, err
	}
	var chats []tg.ChatClass
	switch d := res.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}

	var out []Group
	for _, ch := range chats {
		switch c := ch.(type) {
		case *tg.Channel:
			out = append(out, Group{
				BotAPIID: -(1000000000000 + c.ID),
				RawID:    c.ID,
				Title:    c.Title,
				Kind:     "supergroup/channel",
			})
		case *tg.Chat:
			out = append(out, Group{
				BotAPIID: -c.ID,
				RawID:    c.ID,
				Title:    c.Title,
				Kind:     "group",
			})
		}
	}
	return out, nil
}

// Login faqat bir martalik interaktiv kirish uchun: kod (va kerak bo'lsa 2FA)
// so'raydi, sessiyani sessionPath ga saqlaydi va darhol chiqadi.
// Shundan keyin agent hech qachon kod so'ramaydi.
func (b *Bot) Login(ctx context.Context) error {
	client := telegram.NewClient(b.apiID, b.apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: b.sessionPath},
	})

	return client.Run(ctx, func(ctx context.Context) error {
		st, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth holati: %w", err)
		}
		if st.Authorized {
			fmt.Println("✓ Sessiya allaqachon mavjud — qayta login shart emas")
			return nil
		}

		flow := auth.NewFlow(
			termAuth{phone: b.phone, code: b.codePrompt, password: b.passwordFn},
			auth.SendCodeOptions{},
		)
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("login: %w", err)
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("self: %w", err)
		}
		fmt.Printf("✓ Kirildi: %s (id=%d)\n✓ Sessiya saqlandi: %s\n",
			self.FirstName, self.ID, b.sessionPath)
		return nil
	})
}
