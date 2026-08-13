package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"sahiy-agent/internal/ai"
)

// OpenAI'ning haqiqiy javob shakli (kerakli qismi).
const sampleResponse = `{
  "id": "chatcmpl-1",
  "model": "gpt-4o-mini-2024-07-18",
  "choices": [{"message": {"role": "assistant", "content": "Salom!"}}],
  "usage": {
    "prompt_tokens": 1240,
    "completion_tokens": 86,
    "total_tokens": 1326,
    "prompt_tokens_details": {"cached_tokens": 512}
  }
}`

func TestGenerateUsageniOqiydi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleResponse))
	}))
	defer srv.Close()

	c := New("test-key", "gpt-4o-mini", srv.URL)
	out, u, err := c.Generate(context.Background(), "system", "salom", ai.GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Salom!" {
		t.Errorf("javob = %q", out)
	}
	if u.PromptTokens != 1240 || u.CompletionTokens != 86 || u.CachedTokens != 512 {
		t.Errorf("usage = %+v", u)
	}
	if u.Calls != 1 || u.Model != "gpt-4o-mini" {
		t.Errorf("calls/model = %d / %q", u.Calls, u.Model)
	}
}

func TestKalitsizSorovYuborilmaydi(t *testing.T) {
	c := New("", "gpt-4o-mini", "")
	if _, _, err := c.Generate(context.Background(), "s", "u", ai.GenOptions{}); err == nil {
		t.Error("kalit bo'lmasa xato kutilgan")
	}
}
