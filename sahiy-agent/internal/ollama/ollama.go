package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sahiy-agent/internal/ai"
)

const (
	// defaultBaseURL — Ollama serveri. Docker ichidan localhost EMAS,
	// host.docker.internal ishlatiladi (docker-compose.yml ga qarang).
	defaultBaseURL = "http://localhost:11434"
	defaultModel   = "llama3.1:8b"

	// defaultKeepAlive — model RAM'da qancha turishi. Qisqa muddat = kam RAM.
	defaultKeepAlive = "2m"
	// defaultNumCtx — kontekst oynasi. Har bir token KV-kesh uchun RAM oladi,
	// shuning uchun kichik tutiladi (promptlarimiz allaqachon qisqartirilgan).
	defaultNumCtx = 4096
	// defaultMaxTokens — javob uzunligi chegarasi (cheksiz generatsiyaga qarshi).
	defaultMaxTokens = 600
	// defaultTemperature — support javoblari barqaror bo'lsin.
	defaultTemperature = 0.2
	// defaultTimeout — 8B model sekin ishlaydi, 60s yetmaydi.
	defaultTimeout = 180 * time.Second

	// ctxWarnRatio — prompt kontekstning shuncha qismidan oshsa ogohlantiramiz.
	ctxWarnRatio = 0.9
)

// ErrContextFull — prompt kontekst oynasini to'ldirib qo'ydi. Javob baribir
// qaytadi (xato bilan birga), lekin uning boshi kesilgan bo'lishi mumkin.
var ErrContextFull = errors.New("prompt kontekst oynasiga sig'madi")

// Options — RAM va tezlikka ta'sir qiladigan sozlamalar. Nol qiymatlar
// default bilan to'ldiriladi.
type Options struct {
	KeepAlive   string        // "2m", "0" (darhol bo'shatish), "30m"
	NumCtx      int           // kontekst oynasi (token)
	MaxTokens   int           // javobning eng ko'p tokeni
	Temperature float64       // 0 — default (0.2) ishlatiladi
	Timeout     time.Duration // HTTP timeout
}

// Client lokal Ollama serveri bilan ishlaydi.
type Client struct {
	BaseURL     string
	Model       string
	KeepAlive   string
	NumCtx      int
	MaxTokens   int
	Temperature float64
	http        *http.Client
}

// New yangi Ollama backend yaratadi.
func New(baseURL, model string, opt Options) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if model == "" {
		model = defaultModel
	}
	if opt.KeepAlive == "" {
		opt.KeepAlive = defaultKeepAlive
	}
	if opt.NumCtx <= 0 {
		opt.NumCtx = defaultNumCtx
	}
	if opt.MaxTokens <= 0 {
		opt.MaxTokens = defaultMaxTokens
	}
	if opt.Temperature <= 0 {
		opt.Temperature = defaultTemperature
	}
	if opt.Timeout <= 0 {
		opt.Timeout = defaultTimeout
	}
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Model:       model,
		KeepAlive:   opt.KeepAlive,
		NumCtx:      opt.NumCtx,
		MaxTokens:   opt.MaxTokens,
		Temperature: opt.Temperature,
		http:        &http.Client{Timeout: opt.Timeout},
	}
}

// Name — loglarda ko'rinadigan nom.
func (c *Client) Name() string { return "ollama " + c.Model }

// Ready — lokal modelga API kalit kerak emas, manzil va model nomi yetarli.
func (c *Client) Ready() bool { return c.BaseURL != "" && c.Model != "" }

// --- so'rov/javob tuzilmalari ---

type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	Stream    bool      `json:"stream"`
	KeepAlive string    `json:"keep_alive"`
	// Format — "json" (ixtiyoriy JSON) yoki to'liq JSON Schema. Sxema
	// berilganda model undan tashqariga chiqa olmaydi.
	Format  json.RawMessage `json:"format,omitempty"`
	Options opts            `json:"options"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// opts — Ollama'ning model sozlamalari (RAM va javob uzunligi shu yerda).
type opts struct {
	NumCtx      int     `json:"num_ctx"`
	NumPredict  int     `json:"num_predict"`
	Temperature float64 `json:"temperature"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
	// Token hisobi — Ollama aniq sonlarni qaytaradi.
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	EvalDuration    int64  `json:"eval_duration"` // nanosekund
	Error           string `json:"error"`
}

// Generate — lokal modelga bitta matnli so'rov.
//
// Kontekst to'lib ketgan bo'lsa javob bilan birga ErrContextFull qaytadi —
// chaqiruvchi javobni ishlatishi mumkin, lekin ogohlantirishi kerak.
func (c *Client) Generate(ctx context.Context, system, user string, opt ai.GenOptions) (string, ai.Usage, error) {
	if !c.Ready() {
		return "", ai.Usage{}, fmt.Errorf("ollama sozlanmagan (OLLAMA_URL/OLLAMA_MODEL)")
	}

	msgs := make([]message, 0, 2)
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: user})

	// So'rov sozlamalari: chaqiruvchi bergani ustun turadi.
	numPredict, temp := c.MaxTokens, c.Temperature
	if opt.MaxTokens > 0 {
		numPredict = opt.MaxTokens
	}
	if t, ok := opt.Temp(); ok {
		temp = t
	}
	// Sxema bo'lsa — o'sha (qat'iy shakl), bo'lmasa oddiy JSON rejimi.
	var format json.RawMessage
	switch {
	case len(opt.Schema) > 0:
		format = opt.Schema
	case opt.JSON:
		format = json.RawMessage(`"json"`)
	}
	body, err := json.Marshal(chatRequest{
		Model:     c.Model,
		Messages:  msgs,
		Stream:    false, // butun javobni bir marta olamiz
		KeepAlive: c.KeepAlive,
		Format:    format,
		Options: opts{
			NumCtx:      c.NumCtx,
			NumPredict:  numPredict,
			Temperature: temp,
		},
	})
	if err != nil {
		return "", ai.Usage{}, err
	}

	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", ai.Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", ai.Usage{}, fmt.Errorf("ollama'ga ulanib bo'lmadi (%s): %w\n"+
			"Server ishlayaptimi? Tekshiring: ollama serve", c.BaseURL, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", ai.Usage{}, fmt.Errorf("ollama: %q modeli topilmadi — `ollama pull %s` bajaring", c.Model, c.Model)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ai.Usage{}, fmt.Errorf("ollama xatosi (status %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", ai.Usage{}, fmt.Errorf("ollama javobini o'qib bo'lmadi: %w\nXom: %s", err, string(raw))
	}
	if cr.Error != "" {
		return "", ai.Usage{}, fmt.Errorf("ollama xatosi: %s", cr.Error)
	}
	out := strings.TrimSpace(cr.Message.Content)
	if out == "" {
		return "", ai.Usage{}, fmt.Errorf("ollama bo'sh javob qaytardi\nXom: %s", string(raw))
	}

	u := ai.Usage{
		Model:            c.Model,
		PromptTokens:     cr.PromptEvalCount,
		CompletionTokens: cr.EvalCount,
		Calls:            1,
		DurationMS:       time.Since(started).Milliseconds(),
	}

	// Kontekst to'lgan bo'lsa — prompt boshi kesilgan bo'lishi mumkin.
	if c.ContextFull(u.PromptTokens) {
		return out, u, fmt.Errorf("%w (%d/%d token)", ErrContextFull, u.PromptTokens, c.NumCtx)
	}
	return out, u, nil
}

// ContextFull — prompt kontekst oynasini to'ldirib qo'yganmi.
func (c *Client) ContextFull(promptTokens int) bool {
	return c.NumCtx > 0 && float64(promptTokens) >= float64(c.NumCtx)*ctxWarnRatio
}

// tagsResponse — /api/tags javobi.
type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Check ishga tushganda bir marta chaqiriladi: server javob beradimi va
// kerakli model yuklanganmi. Xato bo'lsa nima qilish kerakligini aytadi.
func (c *Client) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama serveriga ulanib bo'lmadi (%s): %w\n"+
			"   `ollama serve` ishlayaptimi? Docker'da bo'lsa OLLAMA_URL=http://host.docker.internal:11434", c.BaseURL, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama /api/tags xatosi (status %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var tr tagsResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return fmt.Errorf("ollama model ro'yxatini o'qib bo'lmadi: %w", err)
	}
	for _, m := range tr.Models {
		// Ollama nomni "llama3.1:8b" ko'rinishida qaytaradi; tegsiz
		// yozilgan bo'lsa ":latest" bilan solishtiriladi.
		if m.Name == c.Model || m.Name == c.Model+":latest" {
			return nil
		}
	}
	names := make([]string, 0, len(tr.Models))
	for _, m := range tr.Models {
		names = append(names, m.Name)
	}
	return fmt.Errorf("ollama: %q modeli yuklanmagan — `ollama pull %s` bajaring\n   Mavjud modellar: %s",
		c.Model, c.Model, strings.Join(names, ", "))
}
