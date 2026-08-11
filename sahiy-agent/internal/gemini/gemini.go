package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://generativelanguage.googleapis.com/v1beta/models"

// Client Gemini API bilan ishlaydi.
type Client struct {
	APIKey string
	Model  string // masalan "gemini-2.5-flash"
	Prompt string // tizim (system) prompt — .env'dan keladi
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
func (c *Client) Ask(transcript string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bo'sh")
	}

	reqBody := genRequest{
		Contents: []content{
			{Role: "user", Parts: []part{{Text: transcript}}},
		},
	}
	if c.Prompt != "" {
		reqBody.SystemInstruction = &content{Parts: []part{{Text: c.Prompt}}}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBase, c.Model, c.APIKey)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini so'rov: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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
