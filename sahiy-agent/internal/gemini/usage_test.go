package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Gemini'ning haqiqiy javob shakli (kerakli qismi).
const sampleResponse = `{
  "candidates": [{"content": {"parts": [{"text": "Salom!"}], "role": "model"}}],
  "usageMetadata": {
    "promptTokenCount": 900,
    "candidatesTokenCount": 40,
    "thoughtsTokenCount": 12,
    "cachedContentTokenCount": 100,
    "totalTokenCount": 952
  }
}`

func TestGenerateUsageniOqiydi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(sampleResponse))
	}))
	defer srv.Close()

	c := New("test-key", "gemini-2.5-flash-lite")
	c.BaseURL = srv.URL // testda soxta serverga yo'naltiramiz

	out, u, err := c.Generate(context.Background(), "system", "salom")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Salom!" {
		t.Errorf("javob = %q", out)
	}
	// "o'ylash" tokenlari ham chiqish tokeni sifatida hisoblanadi: 40 + 12.
	if u.PromptTokens != 900 || u.CompletionTokens != 52 || u.CachedTokens != 100 {
		t.Errorf("usage = %+v", u)
	}
	if u.Calls != 1 || u.Model != "gemini-2.5-flash-lite" {
		t.Errorf("calls/model = %d / %q", u.Calls, u.Model)
	}
}
