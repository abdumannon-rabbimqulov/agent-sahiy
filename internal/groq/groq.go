// Package groq — Groq bulutidagi model (ai.Backend). Lokal Ollama ishlamay
// qolganda zaxira sifatida ishlatiladi: internal/ai/fallback.go ga qarang.
//
// API OpenAI bilan mos (/openai/v1/chat/completions), shuning uchun bu yerda
// SDK yo'q — oddiy net/http yetadi.
package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sahiy-agent/internal/ai"
)

const (
	// defaultBaseURL — OpenAI-mos endpoint.
	defaultBaseURL = "https://api.groq.com/openai/v1"
	// defaultModel — JSON Schema'ni qo'llab-quvvatlaydigan model. MUHIM:
	// Groq'dagi hamma model buni bilmaydi (llama-3.3-70b, llama-3.1-8b,
	// groq/compound "does not support response format json_schema" deb
	// xato qaytaradi), shuning uchun modelni almashtirishdan oldin
	// tekshiring: console.groq.com/docs/structured-outputs
	defaultModel = "openai/gpt-oss-120b"
	// defaultMaxTokens — javob uzunligi chegarasi.
	defaultMaxTokens = 600
	// defaultTemperature — support javoblari barqaror bo'lsin.
	defaultTemperature = 0.2
	// defaultTimeout — bulut modeli tez, lekin tarmoq sekin bo'lishi mumkin.
	defaultTimeout = 60 * time.Second

	// defaultReasoningEffort — gpt-oss oilasi javobdan OLDIN "reasoning"
	// tokenlarini sarflaydi va ular ham max_tokens hisobidan ketadi.
	// "low" bo'lmasa qaror JSON'i 300 tokenga sig'may qoladi va Groq
	// "max completion tokens reached before generating a valid document"
	// deb butun so'rovni rad etadi. Ruxsat etilgan: low, medium, high.
	defaultReasoningEffort = "low"

	// reasoningHeadroom — reasoning yoqilganda so'raladigan eng kam token.
	// Bizning chegaralar (Decide 300, OrderAnswer 600) lokal, "o'ylamaydigan"
	// model uchun tanlangan — bulutda ular javobgacha yetib bormaydi.
	reasoningHeadroom = 1024
)

// Options — sozlamalar. Nol qiymatlar default bilan to'ldiriladi.
type Options struct {
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration
	// ReasoningEffort — "low" | "medium" | "high". Bo'sh bo'lsa
	// reasoning'li modellar uchun "low" ishlatiladi, boshqalarga
	// umuman yuborilmaydi.
	ReasoningEffort string
}

// Client — Groq API bilan ishlaydi.
type Client struct {
	BaseURL         string
	Model           string
	APIKey          string
	MaxTokens       int
	Temperature     float64
	ReasoningEffort string
	http            *http.Client
}

// New yangi Groq backend. apiKey bo'sh bo'lsa client tayyor emas (Ready
// false qaytaradi) — zaxira o'chirilgan holat.
func New(apiKey string, opt Options) *Client {
	if opt.BaseURL == "" {
		opt.BaseURL = defaultBaseURL
	}
	if opt.Model == "" {
		opt.Model = defaultModel
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
	if opt.ReasoningEffort == "" && reasons(opt.Model) {
		opt.ReasoningEffort = defaultReasoningEffort
	}
	return &Client{
		BaseURL:         strings.TrimRight(opt.BaseURL, "/"),
		Model:           opt.Model,
		APIKey:          strings.TrimSpace(apiKey),
		MaxTokens:       opt.MaxTokens,
		Temperature:     opt.Temperature,
		ReasoningEffort: opt.ReasoningEffort,
		http:            &http.Client{Timeout: opt.Timeout},
	}
}

// reasons — model javobdan oldin "o'ylaydimi". Hozircha gpt-oss oilasi;
// boshqa modelga o'tilsa GROQ_REASONING_EFFORT bilan qo'lda beriladi.
func reasons(model string) bool {
	return strings.Contains(strings.ToLower(model), "gpt-oss")
}

// Name — loglarda ko'rinadigan nom.
func (c *Client) Name() string { return "groq " + c.Model }

// Ready — kalit bor.
func (c *Client) Ready() bool { return c != nil && c.APIKey != "" && c.Model != "" }

// --- so'rov/javob tuzilmalari ---

type chatRequest struct {
	Model           string          `json:"model"`
	Messages        []message       `json:"messages"`
	Temperature     float64         `json:"temperature"`
	MaxTokens       int             `json:"max_tokens"`
	Format          *responseFormat `json:"response_format,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat — qat'iy JSON javob. Faqat json_schema ishlatiladi:
// oddiy "json_object" rejimi Groq'da xabar matnida "json" so'zi bo'lishini
// TALAB qiladi, bizning promptlarimizda esa u yo'q (shakl kod tomonidan
// beriladi) — ya'ni json_object 400 qaytarardi.
type responseFormat struct {
	Type   string      `json:"type"`
	Schema *jsonSchema `json:"json_schema,omitempty"`
}

type jsonSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate — bitta so'rov. ai.Backend shartnomasi bo'yicha javob bilan
// birga sarflangan tokenlarni qaytaradi.
func (c *Client) Generate(ctx context.Context, system, user string, opt ai.GenOptions) (string, ai.Usage, error) {
	if !c.Ready() {
		return "", ai.Usage{}, fmt.Errorf("groq sozlanmagan (GROQ_API_KEY)")
	}

	msgs := make([]message, 0, 2)
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: user})

	maxTokens, temp := c.MaxTokens, c.Temperature
	if opt.MaxTokens > 0 {
		maxTokens = opt.MaxTokens
	}
	if t, ok := opt.Temp(); ok {
		temp = t
	}
	// Reasoning tokenlari ham shu byudjetdan ketadi — javobga joy qoldiramiz.
	if c.ReasoningEffort != "" && maxTokens < reasoningHeadroom {
		maxTokens = reasoningHeadroom
	}
	req := chatRequest{
		Model: c.Model, Messages: msgs, Temperature: temp, MaxTokens: maxTokens,
		ReasoningEffort: c.ReasoningEffort,
	}
	if len(opt.Schema) > 0 {
		req.Format = &responseFormat{
			Type:   "json_schema",
			Schema: &jsonSchema{Name: "javob", Schema: opt.Schema},
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", ai.Usage{}, err
	}

	started := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", ai.Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", ai.Usage{}, fmt.Errorf("groq'ga ulanib bo'lmadi: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var cr chatResponse
	// Xato javobi ham JSON bo'ladi — avval o'qib ko'ramiz, sababi
	// aniqroq chiqsin ("model does not support response format" kabi).
	_ = json.Unmarshal(raw, &cr)
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ai.Usage{}, fmt.Errorf("groq: kalit qabul qilinmadi — GROQ_API_KEY ni tekshiring")
	}
	if cr.Error != nil && cr.Error.Message != "" {
		return "", ai.Usage{}, fmt.Errorf("groq xatosi: %s", cr.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ai.Usage{}, fmt.Errorf("groq xatosi (status %d): %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(cr.Choices) == 0 {
		return "", ai.Usage{}, fmt.Errorf("groq bo'sh javob qaytardi\nXom: %s", string(raw))
	}

	out := strings.TrimSpace(cr.Choices[0].Message.Content)
	if out == "" {
		return "", ai.Usage{}, fmt.Errorf("groq bo'sh matn qaytardi")
	}
	return out, ai.Usage{
		Model:            c.Model,
		PromptTokens:     cr.Usage.PromptTokens,
		CompletionTokens: cr.Usage.CompletionTokens,
		Calls:            1,
		DurationMS:       time.Since(started).Milliseconds(),
	}, nil
}

// Check — ishga tushganda bir marta: kalit ishlaydimi va model bormi.
func (c *Client) Check(ctx context.Context) error {
	if !c.Ready() {
		return fmt.Errorf("groq sozlanmagan (GROQ_API_KEY)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("groq'ga ulanib bo'lmadi: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("groq: kalit qabul qilinmadi — GROQ_API_KEY ni tekshiring")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("groq /models xatosi (status %d): %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("groq javobini o'qib bo'lmadi: %w", err)
	}
	for _, m := range list.Data {
		if m.ID == c.Model {
			return nil
		}
	}
	return fmt.Errorf("groq: %q modeli ro'yxatda yo'q — GROQ_MODEL ni tekshiring", c.Model)
}
