// sahiy/chat — yordam xizmati (support) suhbatidan agent QANDAY ma'lumot
// olayotganini ko'rsatadi.
//
// Faqat O'QIYDI: mijozga hech narsa yuborilmaydi, "o'qildi" qo'yilmaydi,
// bazaga yozilmaydi. Agent yo'lining aynan o'zi takrorlanadi (main.go
// dagi handleChat: FetchMessages → Window → TranscriptAfter).
//
// Ishlatish:
//
//	go run ./sahiy/chat -conv 56825
//	go run ./sahiy/chat -client 8407941
//	go run ./sahiy/chat -conv 56825 -prev 981234   # "yangi xabar" chegarasi
//	go run ./sahiy/chat -conv 56825 -raw           # to'liq xom JSON
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sahiy-agent/internal/auth"
	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
	"sahiy-agent/internal/envfile"
	"sahiy-agent/internal/support"
)

// maxImages — agentdagi bilan bir xil (main.go:51): suhbatdan eslab
// qolinadigan rasm havolalari soni.
const maxImages = 3

func main() {
	conv := flag.Int64("conv", 0, "suhbat id (support.chat.conversation)")
	clientID := flag.Int64("client", 0, "mijoz id — uning suhbatlari ro'yxati")
	prev := flag.Int64("prev", 0, "oxirgi javob berilgan xabar id (yangi xabar chegarasi)")
	limit := flag.Int("limit", 0, "nechta xabar o'qilsin (0 — agentdagidek)")
	raw := flag.Bool("raw", false, "xom JSON javobni to'liq chiqarish")
	envPath := flag.String("env", ".env", ".env fayl yo'li")
	flag.Parse()

	if *conv == 0 && *clientID == 0 {
		fmt.Fprintln(os.Stderr, "xato: -conv yoki -client bering")
		flag.Usage()
		os.Exit(1)
	}

	cfg := loadConfig(*envPath)
	c, err := apiClient(cfg)
	if err != nil {
		fail(err)
	}

	// Mijoz id berilgan bo'lsa — avval uning suhbatlarini ko'rsatamiz.
	if *clientID != 0 {
		chats, err := support.SearchByClient(c, *clientID, 1, 20, nil)
		if err != nil {
			fail(err)
		}
		printConversations(*clientID, chats)
		if *conv == 0 {
			if len(chats) == 0 {
				return
			}
			*conv = chats[0].ID
			fmt.Printf("\nEng yangi suhbat tanlandi: %d\n", *conv)
		}
	}

	n := *limit
	if n <= 0 {
		n = fetchLimit(cfg)
	}
	msgs, err := support.FetchMessages(c, *conv, 1, n)
	if err != nil {
		fail(err)
	}

	printRaw(msgs, n, *raw)
	window := printWindow(cfg, msgs, *prev)
	printForAI(window, *prev)
}

// --- 1-bo'lim: xom javob ---

func printRaw(msgs []support.Message, limit int, full bool) {
	head(fmt.Sprintf("1. XOM JAVOB — serverdan kelgani (so'ralgani: %d ta)", limit))
	fmt.Printf("Kelgan xabarlar: %d ta\n", len(msgs))
	if len(msgs) == limit {
		fmt.Printf("⚠️  Chegaraga tegdi — suhbatda bundan ko'p xabar bor, faqat 1-sahifa olinadi\n")
	}
	if len(msgs) == 0 {
		return
	}

	if full {
		out, err := json.MarshalIndent(msgs, "", "  ")
		if err != nil {
			fail(err)
		}
		fmt.Println(string(out))
		return
	}

	fmt.Printf("\n%-10s %-8s %-9s %-20s %s\n", "id", "kim", "turi", "vaqti", "matn")
	for _, m := range msgs {
		kind := "matn"
		if m.IsImage() {
			kind = "rasm"
		} else if strings.TrimSpace(m.Message) == "" {
			kind = "BO'SH"
		}
		seen := ""
		if m.SeenAt == nil {
			seen = " (o'qilmagan)"
		}
		fmt.Printf("%-10d %-8s %-9s %-20s %s%s\n",
			m.ID, role(m), kind, short(m.CreatedAt, 19), short(oneLine(m.Message), 60), seen)
	}
	fmt.Println("\n(to'liq JSON uchun: -raw)")
}

// --- 2-bo'lim: tanlangan oyna ---

func printWindow(cfg *config.Config, msgs []support.Message, prev int64) []support.Message {
	head("2. TANLANGAN OYNA — shulardan qaysilari modelga boradi")

	window := support.Window(msgs, prev, cfg.ContextBefore, cfg.HistoryLimit)
	fmt.Printf("Sozlama: CONTEXT_BEFORE=%d, HISTORY_LIMIT=%d, oxirgi javob berilgan xabar (-prev)=%d\n",
		cfg.ContextBefore, cfg.HistoryLimit, prev)
	fmt.Printf("Natija: %d tadan %d tasi tanlandi\n", len(msgs), len(window))

	if len(window) > 0 {
		fmt.Printf("Tanlangan id'lar: %d … %d\n", window[0].ID, window[len(window)-1].ID)
	}
	if dropped := len(msgs) - len(window); dropped > 0 {
		fmt.Printf("Tashlab ketilgan (eski): %d ta\n", dropped)
	}

	lastID, lastText := support.LastClientMessage(msgs)
	fmt.Printf("\nMijozning oxirgi xabari: id=%d — %s\n", lastID, short(oneLine(lastText), 100))
	if stale, age := support.Stale(msgs, cfg.MaxMessageAge); stale {
		fmt.Printf("⚠️  ESKI: %s oldin yozilgan (chegara %s) — agent bunday suhbatga javob bermaydi\n",
			age.Round(time.Minute), cfg.MaxMessageAge)
	}
	if imgs := support.ImageURLs(window, maxImages); len(imgs) > 0 {
		fmt.Printf("Rasmlar: %d ta (model rasmni ko'rmaydi, faqat \"rasm bor\" belgisi ketadi)\n", len(imgs))
	}
	return window
}

// --- 3-bo'lim: modelga ketadigan matn ---

func printForAI(window []support.Message, prev int64) {
	head("3. AI KO'RADIGAN MATN — modelga user xabari sifatida ketadi")
	text := support.TranscriptAfter(window, prev)
	fmt.Print(text)
	fmt.Printf("\n(%d belgi, ≈%d token)\n", len(text), len(text)/4)
	if prev == 0 {
		fmt.Printf("\nEslatma: -prev berilmagani uchun %q ajratgichi qo'yilmadi.\n"+
			"Agent uni bazadagi oxirgi javob berilgan xabar id'sidan oladi.\n", support.NewMarker)
	}
	fmt.Println("\nSystem prompt (base va bloklar) bazadan olinadi — uni /prompts\n" +
		"sahifasidagi \"Sinab ko'rish\" tugmasi bilan ko'rish mumkin.")
}

func printConversations(clientID int64, chats []support.Conversation) {
	head(fmt.Sprintf("MIJOZ %d SUHBATLARI", clientID))
	if len(chats) == 0 {
		fmt.Println("Suhbat topilmadi.")
		return
	}
	fmt.Printf("%-8s %-6s %-8s %-20s %s\n", "id", "holat", "o'qilm.", "yaratilgan", "oxirgi xabar")
	for _, ch := range chats {
		fmt.Printf("%-8d %-6d %-8d %-20s %s\n",
			ch.ID, ch.State, ch.OperatorUnseenCount,
			short(ch.CreatedAt, 19), short(oneLine(ch.Message), 50))
	}
}

// --- yordamchilar ---

// loadConfig — .env ni topib config.Load ga beradi (agentdagi bilan bir xil
// default'lar shu yerdan keladi).
func loadConfig(envPath string) *config.Config {
	if _, path := envfile.Find(envPath); path != "" {
		envPath = path
		fmt.Fprintln(os.Stderr, ".env o'qildi:", path)
	}
	cfg, err := config.Load(envPath)
	if err != nil {
		fail(err)
	}
	return cfg
}

// apiClient — main.go:1146 dagi bilan bir xil: token olinadi va 401 da
// o'zini yangilaydigan client qaytadi.
func apiClient(cfg *config.Config) (*client.Client, error) {
	cachePath := filepath.Join(cfg.DataDir, "token.json")
	token, err := auth.GetToken(cfg, cachePath)
	if err != nil {
		return nil, err
	}
	c := client.New(cfg.BaseURL, token)
	c.Refresh = func() (string, error) { return auth.Refresh(cfg, cachePath) }
	return c, nil
}

// fetchLimit — agentdagi qoida (main.go:1159): HISTORY_LIMIT 50 dan katta
// bo'lsa o'sha, aks holda 50.
func fetchLimit(cfg *config.Config) int {
	if cfg.HistoryLimit > 50 {
		return cfg.HistoryLimit
	}
	return 50
}

func role(m support.Message) string {
	if m.SenderType == "" {
		return "unknown"
	}
	return m.SenderType
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func head(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("─", len([]rune(title))))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "xato:", err)
	os.Exit(1)
}
