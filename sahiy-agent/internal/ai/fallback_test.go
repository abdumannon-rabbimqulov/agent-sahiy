package ai

import (
	"context"
	"errors"
	"testing"
)

// stubBackend — tarmoqsiz soxta backend.
type stubBackend struct {
	name  string
	ready bool
	out   string
	usage Usage
	err   error
	calls int
}

func (s *stubBackend) Name() string { return s.name }
func (s *stubBackend) Ready() bool  { return s.ready }
func (s *stubBackend) Generate(context.Context, string, string, GenOptions) (string, Usage, error) {
	s.calls++
	return s.out, s.usage, s.err
}

func TestFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("zaxira tayyor bo'lmasa qatlam qo'shilmaydi", func(t *testing.T) {
		primary := &stubBackend{name: "ollama", ready: true}
		if got := NewFallback(primary, &stubBackend{ready: false}); got != Backend(primary) {
			t.Errorf("asosiy backendning o'zi kutilgandi, olindi: %T", got)
		}
		if got := NewFallback(primary, nil); got != Backend(primary) {
			t.Errorf("nil zaxira bilan asosiy backend kutilgandi, olindi: %T", got)
		}
	})

	t.Run("asosiysi ishlasa zaxira chaqirilmaydi", func(t *testing.T) {
		primary := &stubBackend{name: "ollama", ready: true, out: "javob"}
		secondary := &stubBackend{name: "groq", ready: true, out: "zaxira javobi"}
		out, _, err := NewFallback(primary, secondary).Generate(ctx, "s", "u", GenOptions{})
		if err != nil || out != "javob" {
			t.Fatalf("javob=%q xato=%v", out, err)
		}
		if secondary.calls != 0 {
			t.Error("zaxira bekorga chaqirildi")
		}
	})

	t.Run("asosiysi yiqilsa zaxira ishlaydi", func(t *testing.T) {
		primary := &stubBackend{name: "ollama", ready: true, err: errors.New("ulanib bo'lmadi")}
		secondary := &stubBackend{name: "groq", ready: true, out: "zaxira javobi"}
		m := &Meter{}
		out, _, err := NewFallback(primary, secondary).
			Generate(WithMeter(ctx, m), "s", "u", GenOptions{})
		if err != nil || out != "zaxira javobi" {
			t.Fatalf("javob=%q xato=%v", out, err)
		}
		// Lokal modelning yiqilgani yo'qolmasligi kerak.
		if w := m.Warnings(); len(w) != 1 {
			t.Errorf("bitta ogohlantirish kutilgandi: %v", w)
		}
	})

	t.Run("qisman javob zaxiraga o'tkazmaydi", func(t *testing.T) {
		// Kontekst to'lganda Ollama javob VA xato qaytaradi — javob bor,
		// ya'ni bekorga bulutga pul to'lash shart emas.
		primary := &stubBackend{name: "ollama", ready: true,
			out: "kesilgan javob", err: errors.New("kontekst to'ldi")}
		secondary := &stubBackend{name: "groq", ready: true, out: "zaxira javobi"}
		out, _, err := NewFallback(primary, secondary).Generate(ctx, "s", "u", GenOptions{})
		if out != "kesilgan javob" || err == nil {
			t.Fatalf("qisman javob kutilgandi: %q / %v", out, err)
		}
		if secondary.calls != 0 {
			t.Error("qisman javobda zaxira chaqirilgan")
		}
	})

	t.Run("asosiysi tayyor bo'lmasa to'g'ridan-to'g'ri zaxira", func(t *testing.T) {
		primary := &stubBackend{name: "ollama", ready: false}
		secondary := &stubBackend{name: "groq", ready: true, out: "zaxira javobi"}
		f := NewFallback(primary, secondary)
		if !f.Ready() {
			t.Fatal("zaxira tayyor bo'lsa Ready true bo'lishi kerak")
		}
		if out, _, err := f.Generate(ctx, "s", "u", GenOptions{}); err != nil || out != "zaxira javobi" {
			t.Fatalf("javob=%q xato=%v", out, err)
		}
		if primary.calls != 0 {
			t.Error("tayyor bo'lmagan backend chaqirildi")
		}
	})

	t.Run("nom zanjiri", func(t *testing.T) {
		f := NewFallback(&stubBackend{name: "ollama llama3.1:8b", ready: true},
			&stubBackend{name: "groq openai/gpt-oss-120b", ready: true})
		want := "ollama llama3.1:8b → groq openai/gpt-oss-120b"
		if f.Name() != want {
			t.Errorf("nom: %q, kutilgan: %q", f.Name(), want)
		}
	})
}
