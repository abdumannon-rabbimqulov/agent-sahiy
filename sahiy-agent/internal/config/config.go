package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultHistoryLimit — HISTORY_LIMIT berilmasa ishlatiladi.
	defaultHistoryLimit = 20
	// minHistoryLimit — talab: agent kamida 10 ta xabarni o'rganadi
	// (support.MinHistory bilan bir xil bo'lishi kerak).
	minHistoryLimit = 10
	// defaultContextBefore — yangi xabarlardan oldin AI ko'radigan xabarlar
	// soni. Butun tarix o'rniga shuncha — token sezilarli tejaladi.
	defaultContextBefore = 4
	// defaultMaxMessageAge — shundan eski mijoz xabari AI'ga umuman
	// yuborilmaydi (eski, dolzarbligini yo'qotgan murojaatlar).
	defaultMaxMessageAge = 24 * time.Hour
)

// Config dasturga kerakli sozlamalar.
type Config struct {
	// API kirish
	Login        string
	Password     string
	LoginField   string // login body'dagi maydon nomi (login/username/phone)
	BaseURL      string
	UserBaseURL  string // api.sahiy.uz (adminka daigou-orders uchun)
	AdminkaToken string // ADMINKA_TOKEN_BEARER — daigou-orders uchun Bearer token

	// AI provayderi: "openai" | "gemini" | "" (auto — OpenAI kaliti bo'lsa OpenAI)
	AIProvider string

	// Gemini
	GeminiAPIKey string
	GeminiModel  string

	// AI xarajati (0 bo'lsa kod jadvalidagi narx ishlatiladi)
	PriceIn       float64 // USD / 1M kirish tokeni
	PriceCachedIn float64 // USD / 1M kesh'dan olingan kirish tokeni
	PriceOut      float64 // USD / 1M chiqish tokeni
	BudgetUSD     float64 // oylik chegara; oshsa Telegram'ga ogohlantirish (0 — o'chiq)

	// Ollama (lokal model)
	OllamaURL         string        // http://localhost:11434
	OllamaModel       string        // llama3.1:8b
	OllamaKeepAlive   string        // model RAM'da qancha turadi ("0" — darhol bo'shatish)
	OllamaNumCtx      int           // kontekst oynasi (RAM'ga bevosita ta'sir qiladi)
	OllamaMaxTokens   int           // javobning eng ko'p tokeni
	OllamaTemperature float64       // 0 — paketdagi default
	OllamaTimeout     time.Duration // lokal model sekin — uzun timeout

	// OpenAI
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string // OpenAI-mos boshqa API uchun (odatda bo'sh)

	// Agent xatti-harakati
	AgentSenderID int64 // 0 bo'lsa token'dagi "sub" ishlatiladi
	AutoReply     bool  // true bo'lsa Gemini javobini avtomatik yuboradi
	HistoryLimit  int   // AI'ga beriladigan xabarlarning eng ko'p soni (yuqori chegara)

	// Token tejash: AI'ga butun tarix emas, faqat yangi xabarlar beriladi.
	ContextBefore int           // yangi xabarlardan oldin qo'shiladigan kontekst xabarlari soni
	MaxMessageAge time.Duration // shundan eski mijoz xabariga javob yozilmaydi (0 — cheklovsiz)

	// Telegram eskalatsiya (xodimlar guruhi)
	TelegramToken  string // bot token (Bot API rejimi, ixtiyoriy)
	TelegramChatID string // eskalatsiya boradigan guruh id (bo'sh bo'lsa ALLOWED_GROUPS[0])
	EscalateMarker string // Gemini javobida shu belgi bo'lsa eskalatsiya

	// Telegram userbot (MTProto)
	TgAPIID       int     // API_ID
	TgAPIHash     string  // API_HASH
	TgPhone       string  // telefon raqami (birinchi kirishda kod so'raladi)
	TgSession     string  // sessiya fayli yo'li
	AllowedGroups []int64 // userbot ishlaydigan guruh id'lari

	// Web dashboard
	WebAddr   string // masalan ":8080"
	AdminUser string // dashboard uchun Basic Auth login (bo'sh bo'lsa himoya yo'q)
	AdminPass string // dashboard uchun Basic Auth parol
	WebDev    bool   // true bo'lsa frontend fayllar diskdan o'qiladi (embed emas)

	// Ikkinchi sayt (api.sahiy.uz service API) — buyurtma holati
	ServiceBaseURL    string // default https://api.sahiy.uz
	ServicePhone      string
	ServicePassword   string
	ServiceDeviceID   string
	ServiceDeviceName string
	ServiceDeviceType string
	ServiceAPKType    int
	ServiceFcmToken   string

	// Birinchi ishga tushishda mavjud suhbatlarga javob berilsinmi
	Backfill bool

	// Ma'lumotlar bazasi va fayllar
	DatabaseURL string // Postgres DSN
	DataDir     string // sessiya/token fayllar uchun katalog
}

// Load .env faylni o'qiydi va Config qaytaradi.
func Load(envPath string) (*Config, error) {
	_ = loadDotEnv(envPath) // .env ixtiyoriy

	cfg := &Config{
		Login:       os.Getenv("LOGIN"),
		LoginField:  os.Getenv("LOGIN_FIELD"),
		Password:    os.Getenv("PASSWORD"),
		BaseURL:     os.Getenv("BASE_URL"),
		UserBaseURL: os.Getenv("USER_BASE_URL"),
		// Eski .env'larda kalit ADMINKA_TOKEN_BAARER deb yozilgan.
		AdminkaToken: firstNonEmpty(os.Getenv("ADMINKA_TOKEN_BEARER"), os.Getenv("ADMINKA_TOKEN_BAARER")),
		GeminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		GeminiModel:  os.Getenv("GEMINI_MODEL"),

		AIProvider:    strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER"))),
		OpenAIModel:   os.Getenv("OPENAI_MODEL"),
		OpenAIBaseURL: os.Getenv("OPENAI_BASE_URL"),

		TelegramToken:  os.Getenv("TELEGRAM_TOKEN"),
		TelegramChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		EscalateMarker: os.Getenv("ESCALATE_MARKER"),
		WebAddr:        os.Getenv("WEB_ADDR"),
		AdminUser:      os.Getenv("ADMIN_USER"),
		AdminPass:      os.Getenv("ADMIN_PASS"),
	}
	// OPENAI_API_KEY asosiy nom; OPEN_API_KEY — eski .env'lar uchun.
	cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	if cfg.OpenAIAPIKey == "" {
		cfg.OpenAIAPIKey = os.Getenv("OPEN_API_KEY")
	}
	cfg.WebDev = strings.EqualFold(os.Getenv("WEB_DEV"), "true")
	cfg.Backfill = strings.EqualFold(os.Getenv("BACKFILL"), "true")
	if cfg.LoginField == "" {
		cfg.LoginField = "login"
	}
	if cfg.EscalateMarker == "" {
		cfg.EscalateMarker = "#ESCALATE"
	}
	if cfg.WebAddr == "" {
		cfg.WebAddr = ":8080"
	}

	// Userbot (MTProto)
	if v := os.Getenv("API_ID"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			cfg.TgAPIID = id
		}
	}
	cfg.TgAPIHash = os.Getenv("API_HASH")
	cfg.TgPhone = os.Getenv("TG_PHONE")
	cfg.AllowedGroups = parseIDs(os.Getenv("ALLOWED_GROUPS"))

	// Ma'lumotlar bazasi va fayllar katalogi.
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.DataDir = os.Getenv("DATA_DIR")
	if cfg.DataDir == "" {
		cfg.DataDir = "."
	}
	cfg.TgSession = os.Getenv("TG_SESSION")
	if cfg.TgSession == "" {
		cfg.TgSession = filepath.Join(cfg.DataDir, "tg-session.json")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.market.sahiy.uz"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if v := os.Getenv("AGENT_SENDER_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.AgentSenderID = id
		}
	}
	cfg.AutoReply = strings.EqualFold(os.Getenv("AUTO_REPLY"), "true")

	// Kontekst chuqurligi: kamida support.MinHistory (10) ta xabar.
	cfg.HistoryLimit = defaultHistoryLimit
	if v := os.Getenv("HISTORY_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.HistoryLimit = n
		}
	}
	if cfg.HistoryLimit < minHistoryLimit {
		cfg.HistoryLimit = minHistoryLimit
	}

	// Ollama (lokal model). Bo'sh qiymatlar paket ichidagi default bilan
	// to'ldiriladi — bu yerda faqat .env dagi qiymat o'qiladi.
	cfg.OllamaURL = os.Getenv("OLLAMA_URL")
	cfg.OllamaModel = os.Getenv("OLLAMA_MODEL")
	cfg.OllamaKeepAlive = os.Getenv("OLLAMA_KEEP_ALIVE")
	cfg.OllamaNumCtx = envInt("OLLAMA_NUM_CTX")
	cfg.OllamaMaxTokens = envInt("OLLAMA_MAX_TOKENS")
	cfg.OllamaTemperature = envFloat("OLLAMA_TEMPERATURE")
	if sec := envInt("OLLAMA_TIMEOUT_SEC"); sec > 0 {
		cfg.OllamaTimeout = time.Duration(sec) * time.Second
	}

	// Narx va byudjet (USD, 1M token uchun).
	cfg.PriceIn = envFloat("AI_PRICE_IN")
	cfg.PriceCachedIn = envFloat("AI_PRICE_CACHED_IN")
	cfg.PriceOut = envFloat("AI_PRICE_OUT")
	cfg.BudgetUSD = envFloat("AI_BUDGET_USD")

	// Yangi xabarlardan oldin nechta xabar kontekstga qo'shiladi.
	cfg.ContextBefore = defaultContextBefore
	if v := os.Getenv("CONTEXT_BEFORE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.ContextBefore = n
		}
	}

	// Eski xabarlarga javob yozilmaydi (token behuda ketmasin).
	cfg.MaxMessageAge = defaultMaxMessageAge
	if v := os.Getenv("MAX_MESSAGE_AGE_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxMessageAge = time.Duration(n) * time.Hour
		}
	}

	// --- Ikkinchi sayt (service API) ---
	cfg.ServiceBaseURL = os.Getenv("SERVICE_BASE_URL")
	if cfg.ServiceBaseURL == "" {
		cfg.ServiceBaseURL = "https://api.sahiy.uz"
	}
	cfg.ServicePhone = os.Getenv("SERVICE_PHONE")
	cfg.ServicePassword = os.Getenv("SERVICE_PASSWORD")
	cfg.ServiceDeviceID = os.Getenv("SERVICE_DEVICE_ID")
	cfg.ServiceDeviceName = os.Getenv("SERVICE_DEVICE_NAME")
	if cfg.ServiceDeviceName == "" {
		cfg.ServiceDeviceName = "phone"
	}
	cfg.ServiceDeviceType = os.Getenv("SERVICE_DEVICE_TYPE")
	if cfg.ServiceDeviceType == "" {
		cfg.ServiceDeviceType = "mobile"
	}
	cfg.ServiceAPKType = 2
	if v := os.Getenv("SERVICE_APK_TYPE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.ServiceAPKType = n
		}
	}
	cfg.ServiceFcmToken = os.Getenv("SERVICE_FCM_TOKEN")

	if cfg.Login == "" || cfg.Password == "" {
		return nil, fmt.Errorf("LOGIN va PASSWORD .env faylda yoki muhit o'zgaruvchilarida bo'lishi kerak")
	}
	return cfg, nil
}

// envInt — muhit o'zgaruvchisidan butun son o'qiydi (bo'lmasa yoki noto'g'ri
// bo'lsa 0).
func envInt(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// envFloat — muhit o'zgaruvchisidan son o'qiydi (bo'lmasa yoki noto'g'ri
// bo'lsa 0).
func envFloat(key string) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// loadDotEnv oddiy KEY=VALUE parser (tashqi kutubxonasiz).
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// parseIDs "-100123,456" kabi vergul bilan ajratilgan id'larni o'qiydi.
func parseIDs(s string) []int64 {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// firstNonEmpty — birinchi bo'sh bo'lmagan qiymat.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
