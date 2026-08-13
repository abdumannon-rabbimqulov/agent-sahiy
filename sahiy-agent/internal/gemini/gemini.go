// Package gemini — ai.Backend'ning Google Gemini uchun amalga oshirilishi.
// Promptlar bu yerda emas, internal/ai paketida.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sahiy-agent/internal/ai"
)

// defaultBaseURL — Gemini API manzili.
const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// maxRetries — 429/5xx uchun qayta urinishlar soni.
const maxRetries = 3

// Client Gemini API bilan ishlaydi.
type Client struct {
	APIKey  string
	Model   string // masalan "gemini-2.5-flash"
	BaseURL string // odatda defaultBaseURL (testda almashtiriladi)
	http    *http.Client
}

// New yangi Gemini backend yaratadi.
func New(apiKey, model string) *Client {
	if model == "" {
		model = "gemini-2.5-flash-lite"
	}
	return &Client{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Name — loglarda ko'rinadigan nom.
func (c *Client) Name() string { return "gemini " + c.Model }

// Ready — API kaliti bormi.
func (c *Client) Ready() bool { return c.APIKey != "" }

// --- so'rov/javob tuzilmalari ---

type genRequest struct {
	SystemInstruction *content   `json:"systemInstruction,omitempty"`
	Contents          []content  `json:"contents"`
	GenerationConfig  *genConfig `json:"generationConfig,omitempty"`
}

// genConfig — javob uzunligi, temperature va qat'iy JSON.
type genConfig struct {
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text,omitempty"`
}

type genResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	// UsageMetadata — bilinadigan aniq token soni. thoughtsTokenCount
	// ("o'ylash" tokenlari) ham chiqish tokeni sifatida hisoblanadi.
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate — Gemini'ga bitta matnli so'rov (429/5xx da qayta urinish bilan).
func (c *Client) Generate(ctx context.Context, system, user string, opt ai.GenOptions) (string, ai.Usage, error) {
	return c.send(ctx, system, []part{{Text: user}}, opt)
}

// genCfg — GenOptions'ni Gemini formatiga o'giradi (kerak bo'lmasa nil).
func genCfg(opt ai.GenOptions) *genConfig {
	cfg := &genConfig{MaxOutputTokens: opt.MaxTokens}
	if t, ok := opt.Temp(); ok {
		cfg.Temperature = &t
	}
	if opt.JSON {
		cfg.ResponseMimeType = "application/json"
	}
	if cfg.MaxOutputTokens == 0 && cfg.Temperature == nil && cfg.ResponseMimeType == "" {
		return nil
	}
	return cfg
}

// send — Gemini'ga so'rov yuboradi (429/5xx da qayta urinish bilan).
func (c *Client) send(ctx context.Context, systemPrompt string, parts []part, opt ai.GenOptions) (string, ai.Usage, error) {
	if c.APIKey == "" {
		return "", ai.Usage{}, fmt.Errorf("GEMINI_API_KEY bo'sh")
	}

	reqBody := genRequest{
		Contents: []content{{Role: "user", Parts: parts}},
	}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &content{Parts: []part{{Text: systemPrompt}}}
	}
	if cfg := genCfg(opt); cfg != nil {
		reqBody.GenerationConfig = cfg
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", ai.Usage{}, err
	}
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", base, c.Model, c.APIKey)

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

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("gemini so'rov: %w", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Vaqtinchalik xatolar — qayta urinamiz.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("gemini xatosi (status %d): %s", resp.StatusCode, string(body))
			continue
		}

		var gr genResponse
		if err := json.Unmarshal(body, &gr); err != nil {
			return "", ai.Usage{}, fmt.Errorf("gemini javobini o'qib bo'lmadi: %w\nXom: %s", err, string(body))
		}
		if gr.Error != nil {
			return "", ai.Usage{}, fmt.Errorf("gemini xatosi: %s", gr.Error.Message)
		}
		if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
			return "", ai.Usage{}, fmt.Errorf("gemini javob qaytarmadi\nXom: %s", string(body))
		}
		m := gr.UsageMetadata
		return gr.Candidates[0].Content.Parts[0].Text, ai.Usage{
			Model:            c.Model,
			PromptTokens:     m.PromptTokenCount,
			CachedTokens:     m.CachedContentTokenCount,
			CompletionTokens: m.CandidatesTokenCount + m.ThoughtsTokenCount,
			Calls:            1,
		}, nil
	}
	return "", ai.Usage{}, lastErr
}
