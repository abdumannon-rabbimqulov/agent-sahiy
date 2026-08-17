package main

import (
	"context"
	"os"
	"testing"

	"sahiy-agent/internal/ai"
	"sahiy-agent/internal/groq"
	"sahiy-agent/internal/ollama"
	"sahiy-agent/internal/prompts"
)

// TestFallbackLive — LOKAL MODEL ISHLAMAGANDA butun zanjir zaxiraga o'tib
// ishlaydimi. Ollama manzili ataylab yopiq portga qaratiladi, ya'ni bu
// test lokal modelni o'chirmasdan ham haqiqiy "ollama yiqildi" holatini
// tekshiradi.
//
// GROQ_TEST_KEY berilmasa o'tkazib yuboriladi.
func TestFallbackLive(t *testing.T) {
	key := os.Getenv("GROQ_TEST_KEY")
	if key == "" {
		t.Skip("GROQ_TEST_KEY berilmagan — zaxira sinovi o'tkazib yuborildi")
	}

	// Yopiq port — Ollama "ulanib bo'lmadi" xatosini qaytaradi.
	dead := ollama.New("http://127.0.0.1:1", "llama3.1:8b", ollama.Options{})
	cloud := groq.New(key, groq.Options{Model: os.Getenv("GROQ_TEST_MODEL")})

	a := &app{ai: ai.New(ai.NewFallback(dead, cloud), stubPrompts{})}

	out, err := a.tryPrompt(context.Background(), prompts.TryRequest{
		Key:        ai.PromptBase,
		Content:    "Sen yo'naltiruvchisan. Murojaat buyurtma holati haqida bo'lsa dashboard va adminka true.",
		Transcript: "Mijoz: buyurtmam qachon keladi? SN998877",
	})
	if err != nil {
		t.Fatalf("zaxira ham ishlamadi: %v", err)
	}
	res := out.(tryResult)
	if res.ParseError != "" || res.Parsed == nil {
		t.Errorf("zaxira javobi o'qilmadi: %s\nxom: %s", res.ParseError, res.Raw)
	}
	// Lokal modelning yiqilgani ogohlantirish bo'lib qolishi kerak.
	if len(res.Warnings) == 0 {
		t.Error("lokal modelning yiqilgani qayd etilmadi")
	}
	if res.Tokens.Model == "" {
		t.Error("zaxira model nomi yozilmadi")
	}
	t.Logf("model=%s token=%d+%d %dms\nogohlantirish: %v\nxom: %s",
		res.Tokens.Model, res.Tokens.PromptTokens, res.Tokens.CompletionTokens,
		res.Tokens.DurationMS, res.Warnings, res.Raw)
}
