// Package openai — ai.Backend'ning OpenAI (Chat Completions) uchun amalga
// oshirilishi. Promptlar bu yerda emas, internal/ai paketida.
package openai

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

// defaultBaseURL — OpenAI-mos API manzili (OPENAI_BASE_URL bilan almashtiriladi).
const defaultBaseURL = "https://api.openai.com/v1"

// defaultModel — arzon va tez model (faqat matn kerak).
const defaultModel = "gpt-4o-mini"

// maxRetries — 429/5xx uchun qayta urinishlar soni.
const maxRetries = 3

// Client OpenAI API bilan ishlaydi.
type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	http    *http.Client
}

// New yangi OpenAI backend yaratadi.
func New(apiKey, model, baseURL string) *Client {
	if model == "" {
		model = defaultModel
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Name — loglarda ko'rinadigan nom.
func (c *Client) Name() string { return "openai " + c.Model }

// Ready — API kaliti bormi.
func (c *Client) Ready() bool { return c.APIKey != "" }

// --- so'rov/javob tuzilmalari ---

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// responseFormat — {"type":"json_object"} qat'iy JSON javob uchun.
type responseFormat struct {
	Type string `json:"type"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	// Usage — bilinadigan aniq token soni (xarajat shundan hisoblanadi).
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate — matnli so'rov.
func (c *Client) Generate(ctx context.Context, system, user string, opt ai.GenOptions) (string, ai.Usage, error) {
	return c.send(ctx, system, user, opt)
}

// send — OpenAI'ga so'rov yuboradi (429/5xx da qayta urinish bilan).
func (c *Client) send(ctx context.Context, systemPrompt, userContent string, opt ai.GenOptions) (string, ai.Usage, error) {
	if c.APIKey == "" {
		return "", ai.Usage{}, fmt.Errorf("OPENAI_API_KEY bo'sh")
	}

	msgs := make([]message, 0, 2)
	if systemPrompt != "" {
		msgs = append(msgs, message{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, message{Role: "user", Content: userContent})

	req0 := chatRequest{Model: c.Model, Messages: msgs, MaxTokens: opt.MaxTokens}
	if t, ok := opt.Temp(); ok {
		req0.Temperature = &t
	}
	if opt.JSON {
		req0.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	data, err := json.Marshal(req0)
	if err != nil {
		return "", ai.Usage{}, err
	}
	url := c.BaseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second // 2s, 4s
			select {
			case <-ctx.Done():
				return "", ai.Usage{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", ai.Usage{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.APIKey)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("openai so'rov: %w", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Vaqtinchalik xatolar — qayta urinamiz.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("openai xatosi (status %d): %s", resp.StatusCode, string(body))
			continue
		}

		var cr chatResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return "", ai.Usage{}, fmt.Errorf("openai javobini o'qib bo'lmadi: %w\nXom: %s", err, string(body))
		}
		if cr.Error != nil {
			return "", ai.Usage{}, fmt.Errorf("openai xatosi: %s", cr.Error.Message)
		}
		if len(cr.Choices) == 0 || strings.TrimSpace(cr.Choices[0].Message.Content) == "" {
			return "", ai.Usage{}, fmt.Errorf("openai javob qaytarmadi\nXom: %s", string(body))
		}
		return cr.Choices[0].Message.Content, ai.Usage{
			Model:            c.Model,
			PromptTokens:     cr.Usage.PromptTokens,
			CachedTokens:     cr.Usage.PromptDetails.CachedTokens,
			CompletionTokens: cr.Usage.CompletionTokens,
			Calls:            1,
		}, nil
	}
	return "", ai.Usage{}, lastErr
}
