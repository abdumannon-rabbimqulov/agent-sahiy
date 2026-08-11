package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const apiBase = "https://generativelanguage.googleapis.com/v1beta/models"

// maxRetries — 429/5xx uchun qayta urinishlar soni.
const maxRetries = 3

// Client Gemini API bilan ishlaydi.
type Client struct {
	APIKey string
	Model  string // masalan "gemini-2.5-flash"
	Prompt string // tizim (system) prompt — prompt.txt yoki .env'dan
	http   *http.Client
}

// New yangi Gemini client yaratadi.
func New(apiKey, model, systemPrompt string) *Client {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Client{
		APIKey: apiKey,
		Model:  model,
		Prompt: systemPrompt,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// --- so'rov/javob tuzilmalari ---

type genRequest struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type genResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Ask transkriptni (chat tarixi) yuborib, agent javobini qaytaradi.
// extra — tanlangan kategoriya ma'lumoti (bo'sh bo'lishi mumkin).
func (c *Client) Ask(ctx context.Context, transcript, extra string) (string, error) {
	system := c.Prompt
	if extra != "" {
		system += "\n\n--- Shu savolga oid ma'lumot ---\n" + extra +
			"\n\nJavobingni faqat shu ma'lumotga tayanib yoz. Bu yerda yo'q narsani o'ylab topma."
	}
	return c.generate(ctx, system, transcript)
}

// Classify mijoz savoliga mos kategoriya id'sini tanlaydi.
// Mos kategoriya topilmasa 0 qaytaradi (xato emas).
func (c *Client) Classify(ctx context.Context, catalog, transcript string) (uint, error) {
	if catalog == "" {
		return 0, nil
	}
	system := "Sen matnni tasniflaysan. Quyida kategoriyalar ro'yxati bor.\n\n" +
		catalog +
		"\nMijozning oxirgi savoli qaysi kategoriyaga tegishli ekanini aniqla.\n" +
		"Javob sifatida FAQAT bitta raqam (kategoriya id) yoz. Hech qanday izoh yozma.\n" +
		"Agar hech qaysi kategoriyaga to'g'ri kelmasa 0 yoz."

	out, err := c.generate(ctx, system, transcript)
	if err != nil {
		return 0, err
	}
	m := regexp.MustCompile(`\d+`).FindString(out)
	if m == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(m, 10, 64)
	if err != nil {
		return 0, nil
	}
	return uint(id), nil
}

// generate — Gemini'ga bitta so'rov (429/5xx da qayta urinish bilan).
func (c *Client) generate(ctx context.Context, systemPrompt, userText string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bo'sh")
	}

	reqBody := genRequest{
		Contents: []content{{Role: "user", Parts: []part{{Text: userText}}}},
	}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &content{Parts: []part{{Text: systemPrompt}}}
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBase, c.Model, c.APIKey)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second // 2s, 4s
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", err
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
			return "", fmt.Errorf("gemini javobini o'qib bo'lmadi: %w\nXom: %s", err, string(body))
		}
		if gr.Error != nil {
			return "", fmt.Errorf("gemini xatosi: %s", gr.Error.Message)
		}
		if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("gemini javob qaytarmadi\nXom: %s", string(body))
		}
		return gr.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", lastErr
}
