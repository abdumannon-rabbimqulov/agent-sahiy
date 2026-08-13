package main

import (
	"context"
	"testing"

	"sahiy-agent/internal/ai"
)

// noPrompts — bo'sh prompt manbai (bu testda prompt kerak emas).
type noPrompts struct{}

func (noPrompts) Get(string) string    { return "" }
func (noPrompts) Keys(string) []string { return nil }

type nameOnly struct{ n string }

func (b nameOnly) Name() string { return b.n }
func (b nameOnly) Ready() bool  { return true }
func (b nameOnly) Generate(context.Context, string, string, ai.GenOptions) (string, ai.Usage, error) {
	return "", ai.Usage{}, nil
}

func TestModelName(t *testing.T) {
	cases := map[string]string{
		"openai gpt-4o-mini": "gpt-4o-mini",
		"ollama llama3.1:8b": "llama3.1:8b",
		// Zaxirali zanjirda narx asosiy model bo'yicha qidiriladi.
		"ollama llama3.1:8b → openai gpt-4o-mini": "llama3.1:8b",
		"yakkanom": "yakkanom",
	}
	for in, want := range cases {
		a := &app{ai: ai.New(nameOnly{in}, noPrompts{})}
		if got := a.modelName(); got != want {
			t.Errorf("modelName(%q) = %q, kutilgan %q", in, got, want)
		}
	}
}
