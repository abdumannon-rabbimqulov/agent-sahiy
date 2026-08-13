//go:build livellm

// Haqiqiy Ollama serveriga qarshi tirik sinov.
package ollama

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTirikModel(t *testing.T) {
	c := New("", "llama3.1:8b", Options{KeepAlive: "0", NumCtx: 4096})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	if err := c.Check(ctx); err != nil {
		t.Fatal("Check:", err)
	}
	t.Log("✓ Check: server va model joyida")

	// 1) Kategoriya tanlash — kod faqat raqam kutadi.
	out, u, err := c.Generate(ctx,
		"Sen matnni tasniflaysan.\n1. Yetkazib berish — muddat, punkt, narx\n2. To'lov — karta, qaytarish\n"+
			"Mijozning savoli qaysi kategoriyaga tegishli? FAQAT bitta raqam yoz.",
		"client: buyurtmam qachon yetib keladi?")
	if err != nil {
		t.Fatal("Classify:", err)
	}
	t.Logf("Classify javobi: %q | %s", strings.TrimSpace(out), u)

	// 2) Oddiy support javobi — o'zbek tilida yozilyaptimi?
	out2, u2, err := c.Generate(ctx,
		"Sen Sahiy support agentisan. Mijozga O'ZBEK TILIDA qisqa, xushmuomala javob yoz.",
		"client: salom, buyurtmam qayerda?")
	if err != nil {
		t.Fatal("Ask:", err)
	}
	t.Logf("Javob: %q", strings.TrimSpace(out2))
	t.Logf("Hisob: %s", u2)
}
