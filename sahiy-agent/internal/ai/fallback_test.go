package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stub struct {
	name  string
	ready bool
	out   string
	usage Usage
	err   error
	calls int
}

func (s *stub) Name() string { return s.name }
func (s *stub) Ready() bool  { return s.ready }
func (s *stub) Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error) {
	s.calls++
	return s.out, s.usage, s.err
}

func TestFallbackAsosiyIshlaganda(t *testing.T) {
	p := &stub{name: "ollama", ready: true, out: "lokal javob", usage: Usage{Model: "llama3.1:8b", Calls: 1}}
	sec := &stub{name: "openai", ready: true, out: "zaxira javob"}
	f := &Fallback{Primary: p, Secondary: sec}

	out, u, err := f.Generate(context.Background(), "s", "u", GenOptions{})
	if err != nil || out != "lokal javob" || u.Model != "llama3.1:8b" {
		t.Errorf("out=%q usage=%+v err=%v", out, u, err)
	}
	if sec.calls != 0 {
		t.Error("zaxira chaqirilmasligi kerak")
	}
	if got := f.Name(); got != "ollama → openai" {
		t.Errorf("Name() = %q", got)
	}
}

func TestFallbackXatodaZaxiraga(t *testing.T) {
	p := &stub{name: "ollama", ready: true, err: errors.New("ulanib bo'lmadi")}
	sec := &stub{name: "openai", ready: true, out: "zaxira javob", usage: Usage{Model: "gpt-4o-mini", Calls: 1}}
	f := &Fallback{Primary: p, Secondary: sec}

	out, u, err := f.Generate(context.Background(), "s", "u", GenOptions{})
	if err != nil || out != "zaxira javob" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	// Xarajat javob bergan modelniki bo'lishi kerak.
	if u.Model != "gpt-4o-mini" {
		t.Errorf("usage.Model = %q, zaxira modeli kutilgan", u.Model)
	}
}

func TestFallbackKontekstBekorQilinganda(t *testing.T) {
	p := &stub{name: "ollama", ready: true, err: context.Canceled}
	sec := &stub{name: "openai", ready: true, out: "zaxira"}
	f := &Fallback{Primary: p, Secondary: sec}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := f.Generate(ctx, "s", "u", GenOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled kutilgan, keldi %v", err)
	}
	if sec.calls != 0 {
		t.Error("dastur to'xtayapti — zaxiraga o'tilmasligi kerak")
	}
}

func TestFallbackQismanMuvaffaqiyatZaxirasiz(t *testing.T) {
	// Javob ham, xato ham bor (kontekst to'lgan) — zaxira chaqirilmaydi.
	p := &stub{name: "ollama", ready: true, out: "yarim javob", err: errors.New("kontekst to'ldi")}
	sec := &stub{name: "openai", ready: true, out: "zaxira"}
	f := &Fallback{Primary: p, Secondary: sec}

	out, _, err := f.Generate(context.Background(), "s", "u", GenOptions{})
	if out != "yarim javob" || err == nil {
		t.Errorf("out=%q err=%v", out, err)
	}
	if sec.calls != 0 {
		t.Error("javob bor ekan — zaxira chaqirilmasligi kerak")
	}
}

func TestFallbackZaxiraTayyorEmas(t *testing.T) {
	p := &stub{name: "ollama", ready: true, err: errors.New("server o'chiq")}
	sec := &stub{name: "openai", ready: false} // kalit yo'q
	f := &Fallback{Primary: p, Secondary: sec}

	if _, _, err := f.Generate(context.Background(), "s", "u", GenOptions{}); err == nil ||
		!strings.Contains(err.Error(), "server o'chiq") {
		t.Errorf("asl xato qaytishi kerak, keldi: %v", err)
	}
	if got := f.Name(); got != "ollama" {
		t.Errorf("tayyor bo'lmagan zaxira nomda ko'rinmasligi kerak: %q", got)
	}
	if !f.Ready() {
		t.Error("asosiysi tayyor — Fallback ham tayyor")
	}
}

func TestFallbackIkkalasiXato(t *testing.T) {
	p := &stub{name: "ollama", ready: true, err: errors.New("lokal xato")}
	sec := &stub{name: "openai", ready: true, err: errors.New("kvota tugadi")}
	f := &Fallback{Primary: p, Secondary: sec}

	_, _, err := f.Generate(context.Background(), "s", "u", GenOptions{})
	if err == nil || !strings.Contains(err.Error(), "lokal xato") || !strings.Contains(err.Error(), "kvota tugadi") {
		t.Errorf("ikkala sabab ham ko'rinishi kerak: %v", err)
	}
}
