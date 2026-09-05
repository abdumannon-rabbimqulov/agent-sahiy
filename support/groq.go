// Groq (LLM) mijozi: /chat/completions ga so'rov, model sozlamalari va
// javobni o'qish.
//
// .env qiymatlarini o'qiydigan kichik yordamchilar (envStr, envInt) va
// xato matnini qisqartiruvchi snippet ham shu yerda.
package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultGroqBaseURL - Groq'ning OpenAI-mos endpointi.
const DefaultGroqBaseURL = "https://api.groq.com/openai/v1"

// DefaultGroqModel - GROQ_MODEL bo'sh bo'lsa shu ishlatiladi.
const DefaultGroqModel = "openai/gpt-oss-120b"

// ErrNoGroqKey - kalit berilmagan.
var ErrNoGroqKey = errors.New("GROQ_API_KEY berilmagan")

// Groq - AI provayder klienti.
type Groq struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration
}

// GroqFromEnv .env dagi GROQ_* qiymatlaridan klient yasaydi.
func GroqFromEnv() Groq {
	g := Groq{
		BaseURL:     os.Getenv("GROQ_BASE_URL"),
		APIKey:      os.Getenv("GROQ_API_KEY"),
		Model:       os.Getenv("GROQ_MODEL"),
		MaxTokens:   envInt("GROQ_MAX_TOKENS", 800),
		Temperature: envFloat("GROQ_TEMPERATURE"),
		Timeout:     time.Duration(envInt("GROQ_TIMEOUT_SEC", 60)) * time.Second,
	}
	if g.BaseURL == "" {
		g.BaseURL = DefaultGroqBaseURL
	}
	if g.Model == "" {
		g.Model = DefaultGroqModel
	}
	return g
}

// envInt .env dagi butun son (bo'sh yoki xato bo'lsa def).
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// Ready - so'rov yuborish mumkinmi.
func (g Groq) Ready() bool { return g.APIKey != "" }

// groqRequest - /chat/completions body.
type groqRequest struct {
	Model          string        `json:"model"`
	Messages       []groqMessage `json:"messages"`
	MaxTokens      int           `json:"max_completion_tokens,omitempty"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *groqFormat   `json:"response_format,omitempty"`
	Reasoning      string        `json:"reasoning_effort,omitempty"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqFormat struct {
	Type string `json:"type"` // "json_object"
}

// groqResponse - kerakli maydonlar.
type groqResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Generate modelga so'rov yuboradi va JSON matn qaytaradi.
// Javob bilan birga sarflangan tokenlar ham qaytadi (xato bo'lsa ham,
// agar server usage bergan bo'lsa).
func (g Groq) Generate(ctx context.Context, system, user string) (string, Usage, error) {
	if !g.Ready() {
		return "", Usage{}, ErrNoGroqKey
	}

	// Groq json_object rejimida xabarlar ichida "json" so'zi bo'lishini
	// TALAB qiladi (aks holda 400). Promt matnini admin yozadi va u so'z
	// unda bo'lmasligi mumkin — shuning uchun kod o'zi kafolatlaydi.
	if !strings.Contains(strings.ToLower(system+user), "json") {
		system = strings.TrimRight(system, "\n") +
			"\n\nJavobni faqat JSON obyekt ko'rinishida qaytar."
	}

	reqBody := groqRequest{
		Model: g.Model,
		Messages: []groqMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:      g.MaxTokens,
		Temperature:    g.Temperature,
		ResponseFormat: &groqFormat{Type: "json_object"},
	}
	// gpt-oss oilasi javobdan oldin "reasoning" tokenlarini sarflaydi —
	// ularni eng pastga tushiramiz (aks holda max_tokens javobga yetmaydi).
	if strings.Contains(g.Model, "gpt-oss") {
		reqBody.Reasoning = envStr("GROQ_REASONING_EFFORT", "low")
	}

	return g.send(ctx, reqBody)
}

// send - /chat/completions ga so'rov yuborib javob matnini va sarflangan
// tokenlarni qaytaradi.
func (g Groq) send(ctx context.Context, body any) (string, Usage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", Usage{}, fmt.Errorf("so'rov yasash: %w", err)
	}

	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(g.BaseURL, "/") + "/chat/completions"
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
		return req, nil
	}

	// Groq bepul tarifda tez-tez 429 (tezlik chegarasi) qaytaradi —
	// bir necha soniya kutib qayta uriniladi.
	start := time.Now()
	status, respBody, err := doWithRetry(&http.Client{Timeout: timeout}, newReq, Retries())
	if err != nil {
		return "", Usage{}, fmt.Errorf("groq so'rovi: %w", err)
	}
	ms := time.Since(start).Milliseconds()

	var out groqResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", Usage{DurationMS: ms},
			fmt.Errorf("groq javobi JSON emas (status %d): %s", status, snippet(respBody))
	}

	u := Usage{
		Model:            out.Model,
		PromptTokens:     out.Usage.PromptTokens,
		CachedTokens:     out.Usage.PromptDetails.CachedTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		Calls:            1,
		DurationMS:       ms,
	}
	if u.Model == "" {
		u.Model = g.Model
	}

	if out.Error != nil {
		return "", u, fmt.Errorf("groq: %s", out.Error.Message)
	}
	if status < 200 || status >= 300 {
		return "", u, fmt.Errorf("groq status %d: %s", status, snippet(respBody))
	}
	if len(out.Choices) == 0 {
		return "", u, errors.New("groq javobi bo'sh")
	}
	return out.Choices[0].Message.Content, u, nil
}

// envStr .env qiymati (bo'sh bo'lsa def).
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// snippet - xato matnini qisqartiradi.
func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
