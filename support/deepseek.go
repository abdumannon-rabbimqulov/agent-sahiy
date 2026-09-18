// DeepSeek (LLM) mijozi: Groq'ga muqobil provayder, xuddi shunday
// OpenAI-mos /chat/completions API.
package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultDeepseekBaseURL - DeepSeek'ning OpenAI-mos endpointi.
const DefaultDeepseekBaseURL = "https://api.deepseek.com"

// DefaultDeepseekModel - DEEPSEEK_MODEL bo'sh bo'lsa shu ishlatiladi.
const DefaultDeepseekModel = "deepseek-chat"

// ErrNoDeepseekKey - kalit berilmagan.
var ErrNoDeepseekKey = errors.New("DEEPSEEK_API_KEY berilmagan")

// Deepseek - AI provayder klienti (Groq bilan bir xil LLM interfeysini
// bajaradi, qarang: llm.go).
type Deepseek struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration
}

// DeepseekFromEnv .env dagi DEEPSEEK_* qiymatlaridan klient yasaydi.
func DeepseekFromEnv() Deepseek {
	d := Deepseek{
		BaseURL:     os.Getenv("DEEPSEEK_BASE_URL"),
		APIKey:      os.Getenv("DEEPSEEK_API_KEY"),
		Model:       os.Getenv("DEEPSEEK_MODEL"),
		MaxTokens:   envInt("DEEPSEEK_MAX_TOKENS", 800),
		Temperature: envFloat("DEEPSEEK_TEMPERATURE"),
		Timeout:     time.Duration(envInt("DEEPSEEK_TIMEOUT_SEC", 60)) * time.Second,
	}
	if d.BaseURL == "" {
		d.BaseURL = DefaultDeepseekBaseURL
	}
	if d.Model == "" {
		d.Model = DefaultDeepseekModel
	}
	return d
}

// Ready - so'rov yuborish mumkinmi.
func (d Deepseek) Ready() bool { return d.APIKey != "" }

// deepseekResponse - /chat/completions javobidan kerakli maydonlar
// (groqResponse bilan bir xil OpenAI shakli).
type deepseekResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptCacheHit   int `json:"prompt_cache_hit_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Generate modelga so'rov yuboradi va JSON matn qaytaradi.
func (d Deepseek) Generate(ctx context.Context, system, user string) (string, Usage, error) {
	if !d.Ready() {
		return "", Usage{}, ErrNoDeepseekKey
	}

	// DeepSeek ham json_object rejimida xabarlar ichida "json" so'zini
	// talab qiladi (Groq bilan bir xil cheklov).
	if !strings.Contains(strings.ToLower(system+user), "json") {
		system = strings.TrimRight(system, "\n") +
			"\n\nJavobni faqat JSON obyekt ko'rinishida qaytar."
	}

	reqBody := groqRequest{
		Model: d.Model,
		Messages: []groqMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:      d.MaxTokens,
		Temperature:    d.Temperature,
		ResponseFormat: &groqFormat{Type: "json_object"},
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, fmt.Errorf("so'rov yasash: %w", err)
	}

	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(d.BaseURL, "/") + "/chat/completions"
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
		return req, nil
	}

	start := time.Now()
	status, respBody, err := doWithRetry(&http.Client{Timeout: timeout}, newReq, Retries())
	if err != nil {
		return "", Usage{}, fmt.Errorf("deepseek so'rovi: %w", err)
	}
	ms := time.Since(start).Milliseconds()

	var out deepseekResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", Usage{DurationMS: ms},
			fmt.Errorf("deepseek javobi JSON emas (status %d): %s", status, snippet(respBody))
	}

	u := Usage{
		Provider:         ProviderDeepSeek,
		Model:            out.Model,
		PromptTokens:     out.Usage.PromptTokens,
		CachedTokens:     out.Usage.PromptCacheHit,
		CompletionTokens: out.Usage.CompletionTokens,
		Calls:            1,
		DurationMS:       ms,
	}
	if u.Model == "" {
		u.Model = d.Model
	}

	if out.Error != nil {
		return "", u, fmt.Errorf("deepseek: %s", out.Error.Message)
	}
	if status < 200 || status >= 300 {
		return "", u, fmt.Errorf("deepseek status %d: %s", status, snippet(respBody))
	}
	if len(out.Choices) == 0 {
		return "", u, errors.New("deepseek javobi bo'sh")
	}
	return out.Choices[0].Message.Content, u, nil
}
