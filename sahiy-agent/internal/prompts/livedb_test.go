//go:build promptcheck

// Haqiqiy Postgres'da: seed, versiya/tarix, kesh, rollback va baza
// yiqilganda eski kesh bilan ishlash.
//
//	PROMPTCHECK_DSN='postgres://...' go test -tags promptcheck -run TestPromptlar -v ./internal/prompts/
package prompts

import (
	"os"
	"testing"

	"sahiy-agent/internal/db"
	"sahiy-agent/internal/models"
)

func TestPromptlar(t *testing.T) {
	dsn := os.Getenv("PROMPTCHECK_DSN")
	if dsn == "" {
		t.Skip("PROMPTCHECK_DSN yo'q")
	}
	gdb, err := db.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}

	// prompt.txt zaxira sifatida.
	fallback := t.TempDir() + "/prompt.txt"
	if err := os.WriteFile(fallback, []byte("ZAXIRA PROMPT (prompt.txt)"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(gdb, fallback)

	// Kesh yangilanishini hook orqali ulaymiz (main.go da ham shunday).
	reloads := 0
	models.PromptChanged = func() {
		reloads++
		if err := s.Reload(); err != nil {
			t.Log("reload:", err)
		}
	}
	defer func() { models.PromptChanged = nil }()

	// --- seed ---
	if err := s.Seed(); err != nil {
		t.Fatal("Seed:", err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal("Reload:", err)
	}
	t.Logf("✓ seed: %d ta prompt, kalitlar: base=%d belgi, cat:*=%v",
		s.Len(), len(s.Get(models.PromptBase)), s.Keys("cat:"))
	if s.Get(models.PromptBase) == "" || s.Get(models.PromptClassify) == "" {
		t.Fatal("base/classify seed bo'lmadi")
	}
	if len(s.Keys("cat:")) == 0 {
		t.Error("kategoriya promptlari seed bo'lmadi")
	}

	// --- tahrirlash: versiya oshadi, tarix yoziladi, kesh darhol yangilanadi ---
	before := s.Get(models.PromptBase)
	if err := s.Set(models.PromptBase, "YANGI ASOSIY PROMPT v2"); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(models.PromptBase); got != "YANGI ASOSIY PROMPT v2" {
		t.Errorf("kesh darhol yangilanmadi: %q", got)
	}
	if reloads == 0 {
		t.Error("AfterSave hook kesh yangilashni chaqirmadi")
	}
	all, _ := s.All()
	var ver int
	for _, p := range all {
		if p.Key == models.PromptBase {
			ver = p.Version
		}
	}
	if ver != 2 {
		t.Errorf("versiya = %d, 2 kutilgan", ver)
	}
	hist, _ := s.History(models.PromptBase)
	if len(hist) == 0 || hist[0].Content != before {
		t.Errorf("tarixga eski matn yozilmadi: %+v", hist)
	}
	t.Logf("✓ tahrir: v%d, tarixda %d ta yozuv, kesh yangilandi (%d marta)", ver, len(hist), reloads)

	// Bir xil matnni qayta saqlash versiyani oshirmasligi kerak.
	if err := s.Set(models.PromptBase, "YANGI ASOSIY PROMPT v2"); err != nil {
		t.Fatal(err)
	}
	all, _ = s.All()
	for _, p := range all {
		if p.Key == models.PromptBase && p.Version != 2 {
			t.Errorf("matn o'zgarmagan, versiya %d ga oshdi", p.Version)
		}
	}

	// --- rollback ---
	if err := s.Rollback(models.PromptBase, 1); err != nil {
		t.Fatal("Rollback:", err)
	}
	if got := s.Get(models.PromptBase); got != before {
		t.Errorf("rollback ishlamadi: %q", got)
	}
	t.Log("✓ rollback: v1 ga qaytdi, kesh ham yangilandi")

	// --- o'chirilgan prompt keshga tushmaydi ---
	if err := s.SetEnabled(models.PromptSummarize, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if s.Get(models.PromptSummarize) != "" {
		t.Error("o'chirilgan prompt keshda qoldi")
	}
	if err := s.SetEnabled(models.PromptSummarize, true); err != nil {
		t.Fatal(err)
	}

	// --- baza yiqilganda eski kesh qoladi ---
	keep := s.Get(models.PromptBase)
	sql, _ := gdb.DB()
	sql.Close() // bazani "yiqitamiz"

	if err := s.Reload(); err == nil {
		t.Error("baza yopiq — Reload xato qaytarishi kerak")
	}
	if got := s.Get(models.PromptBase); got != keep {
		t.Errorf("baza yiqilganda kesh yo'qoldi: %q", got)
	}
	s.checkOnce() // fon tekshiruvi ham yiqilmasligi kerak
	if got := s.Get(models.PromptBase); got != keep {
		t.Errorf("checkOnce keshni buzdi: %q", got)
	}
	t.Log("✓ baza yiqildi — agent eski kesh bilan ishlashda davom etadi")
}
