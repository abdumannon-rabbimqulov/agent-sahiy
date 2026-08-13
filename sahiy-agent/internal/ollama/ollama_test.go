package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ollama /api/chat javobining haqiqiy shakli (kerakli qismi).
const sampleChat = `{
  "model": "llama3.1:8b",
  "message": {"role": "assistant", "content": "  Salom! Qanday yordam bera olaman?  "},
  "done": true,
  "total_duration": 12400000000,
  "prompt_eval_count": 980,
  "eval_count": 120,
  "eval_duration": 6000000000
}`

func TestGenerateJavobVaTokenlar(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("yo'l = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(sampleChat))
	}))
	defer srv.Close()

	c := New(srv.URL, "llama3.1:8b", Options{KeepAlive: "0", NumCtx: 4096, MaxTokens: 600})
	out, u, err := c.Generate(context.Background(), "sen agentsan", "salom")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Salom! Qanday yordam bera olaman?" {
		t.Errorf("javob = %q (bo'shliqlar kesilishi kerak)", out)
	}
	if u.PromptTokens != 980 || u.CompletionTokens != 120 || u.Calls != 1 {
		t.Errorf("usage = %+v", u)
	}
	if u.Model != "llama3.1:8b" {
		t.Errorf("model = %q", u.Model)
	}
	if u.DurationMS < 0 {
		t.Errorf("davomiylik o'lchanmadi: %d", u.DurationMS)
	}

	// So'rov tanasi — RAM sozlamalari yuborilishi shart.
	if got.Stream {
		t.Error("stream=false bo'lishi kerak")
	}
	if got.KeepAlive != "0" {
		t.Errorf("keep_alive = %q", got.KeepAlive)
	}
	if got.Options.NumCtx != 4096 || got.Options.NumPredict != 600 {
		t.Errorf("options = %+v", got.Options)
	}
	if got.Options.Temperature != defaultTemperature {
		t.Errorf("temperature = %v, default kutilgan", got.Options.Temperature)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Errorf("xabarlar = %+v", got.Messages)
	}
}

func TestGenerateKontekstTolgan(t *testing.T) {
	// prompt_eval_count = 980, num_ctx = 1024 → 95% → ogohlantirish.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleChat))
	}))
	defer srv.Close()

	c := New(srv.URL, "llama3.1:8b", Options{NumCtx: 1024})
	out, u, err := c.Generate(context.Background(), "s", "u")
	if !errors.Is(err, ErrContextFull) {
		t.Fatalf("ErrContextFull kutilgan, keldi: %v", err)
	}
	// Javob baribir qaytishi kerak — chaqiruvchi uni ishlatadi.
	if out == "" || u.PromptTokens != 980 {
		t.Errorf("qisman muvaffaqiyat kutilgan: out=%q usage=%+v", out, u)
	}
}

func TestGenerateXatolar(t *testing.T) {
	// 404 — model yo'q.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "yo-q-model", Options{})
	if _, _, err := c.Generate(context.Background(), "s", "u"); err == nil {
		t.Error("404 uchun xato kutilgan")
	}

	// Server o'chiq — ulanish xatosi.
	dead := New("http://127.0.0.1:1", "llama3.1:8b", Options{})
	if _, _, err := dead.Generate(context.Background(), "s", "u"); err == nil {
		t.Error("ulanish xatosi kutilgan")
	}

	// Bo'sh javob.
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":"   "},"done":true}`))
	}))
	defer empty.Close()
	if _, _, err := New(empty.URL, "m", Options{}).Generate(context.Background(), "s", "u"); err == nil {
		t.Error("bo'sh javob uchun xato kutilgan")
	}
}

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("yo'l = %q", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"name":"llama3.1:8b"},{"name":"qwen2.5:7b"}]}`))
	}))
	defer srv.Close()

	if err := New(srv.URL, "llama3.1:8b", Options{}).Check(context.Background()); err != nil {
		t.Errorf("model bor, xato bo'lmasligi kerak: %v", err)
	}
	// Tegsiz nom ":latest" bilan ham topilishi kerak.
	tagless := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"llama3.2:latest"}]}`))
	}))
	defer tagless.Close()
	if err := New(tagless.URL, "llama3.2", Options{}).Check(context.Background()); err != nil {
		t.Errorf("llama3.2 → llama3.2:latest topilishi kerak: %v", err)
	}
	// Model yo'q.
	if err := New(srv.URL, "yo-q:8b", Options{}).Check(context.Background()); err == nil {
		t.Error("model yo'q — xato kutilgan")
	}
	// Server o'chiq.
	if err := New("http://127.0.0.1:1", "m", Options{}).Check(context.Background()); err == nil {
		t.Error("server o'chiq — xato kutilgan")
	}
}

func TestDefaultlar(t *testing.T) {
	c := New("", "", Options{})
	if c.BaseURL != defaultBaseURL || c.Model != defaultModel {
		t.Errorf("default manzil/model: %q %q", c.BaseURL, c.Model)
	}
	if c.KeepAlive != defaultKeepAlive || c.NumCtx != defaultNumCtx || c.MaxTokens != defaultMaxTokens {
		t.Errorf("default RAM sozlamalari: %+v", c)
	}
	// "0" (darhol bo'shatish) default bilan almashib ketmasligi kerak.
	if got := New("", "", Options{KeepAlive: "0"}); got.KeepAlive != "0" {
		t.Errorf("keep_alive=0 saqlanmadi: %q", got.KeepAlive)
	}
}
