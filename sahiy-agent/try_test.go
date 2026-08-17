package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"sahiy-agent/internal/ai"
	"sahiy-agent/internal/ollama"
	"sahiy-agent/internal/prompts"
)

// stubPrompts — bazasiz prompt manbai (sinov uchun).
type stubPrompts map[string]string

func (s stubPrompts) Get(key string) string { return s[key] }
func (s stubPrompts) Keys(prefix string) []string {
	var out []string
	for k := range s {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out
}

// TestTryPromptLive — "Sinab ko'rish" yo'li haqiqiy model bilan ishlaydimi
// va JSON sxemasi javob shaklini kafolatlaydimi.
//
// Ollama kerak, shuning uchun ixtiyoriy: OLLAMA_TEST_URL berilmasa
// o'tkazib yuboriladi (`OLLAMA_TEST_URL=http://localhost:11434 go test .`).
func TestTryPromptLive(t *testing.T) {
	url := os.Getenv("OLLAMA_TEST_URL")
	if url == "" {
		t.Skip("OLLAMA_TEST_URL berilmagan — jonli sinov o'tkazib yuborildi")
	}
	model := os.Getenv("OLLAMA_TEST_MODEL")
	if model == "" {
		model = "llama3.1:8b"
	}

	a := &app{ai: ai.New(ollama.New(url, model, ollama.Options{}), stubPrompts{})}

	// Prompt ATAYLAB "JSON yozma" deb turibdi: sxema baribir to'g'ri
	// shaklni majburlashi kerak.
	out, err := a.tryPrompt(context.Background(), prompts.TryRequest{
		Key:        ai.PromptBase,
		Content:    "Sen yo'naltiruvchisan. JSON YOZMA, oddiy gap bilan javob ber.",
		Transcript: "Mijoz: buyurtmam qachon keladi? SN12345",
	})
	if err != nil {
		t.Fatalf("sinov o'tmadi: %v", err)
	}
	res, ok := out.(tryResult)
	if !ok {
		t.Fatalf("kutilmagan natija turi: %T", out)
	}
	if res.ParseError != "" {
		t.Errorf("sxemaga qaramay JSON o'qilmadi: %s\nxom javob: %s", res.ParseError, res.Raw)
	}
	if res.Parsed == nil {
		t.Errorf("qaror o'qilmadi, xom javob: %s", res.Raw)
	}
	if res.Tokens.PromptTokens == 0 {
		t.Error("token hisobi bo'sh")
	}
	t.Logf("yo'l=%s kind=%s token=%d+%d %dms\nxom: %s",
		res.Path, res.Kind, res.Tokens.PromptTokens, res.Tokens.CompletionTokens,
		res.Tokens.DurationMS, res.Raw)
}
