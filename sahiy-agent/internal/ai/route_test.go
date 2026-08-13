package ai

import (
	"context"
	"strings"
	"testing"
)

// capture — yuborilgan system/user va sozlamalarni ushlab qoladi.
type capture struct {
	system, user string
	opt          GenOptions
	out          string
}

func (c *capture) Name() string { return "capture" }
func (c *capture) Ready() bool  { return true }
func (c *capture) Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error) {
	c.system, c.user, c.opt = system, user, opt
	return c.out, Usage{Calls: 1}, nil
}

func testStore() testPrompts {
	return testPrompts{
		PromptBase:            "ASOSIY PROMPT",
		PromptClassify:        "Router.\n{{CATEGORIES}}\nJSON yoz.",
		"cat:yetkazib-berish": "Toshkentga bepul.",
		"cat:tolov":           "Karta orqali.",
	}
}

func TestClassifyJSONVaSozlamalar(t *testing.T) {
	be := &capture{out: `{"category":"yetkazib-berish","escalate":false}`}
	r, err := New(be, testStore()).Classify(context.Background(), "client: qachon keladi?")
	if err != nil {
		t.Fatal(err)
	}
	if r.Category != "yetkazib-berish" || r.Escalate {
		t.Errorf("route = %+v", r)
	}
	// Router qisqa va deterministik bo'lishi kerak.
	if be.opt.MaxTokens != 20 || !be.opt.TempZero || !be.opt.JSON {
		t.Errorf("router sozlamalari = %+v", be.opt)
	}
	// Kategoriyalar ro'yxati dinamik qo'yilgan.
	if strings.Contains(be.system, "{{CATEGORIES}}") {
		t.Error("{{CATEGORIES}} almashtirilmadi")
	}
	for _, want := range []string{"- yetkazib-berish", "- tolov"} {
		if !strings.Contains(be.system, want) {
			t.Errorf("ro'yxatda %q yo'q:\n%s", want, be.system)
		}
	}
	// Mijoz matni o'zgarishsiz user qismida.
	if be.user != "client: qachon keladi?" {
		t.Errorf("user = %q", be.user)
	}
}

func TestClassifyOylabTopilganKategoriyaRadEtiladi(t *testing.T) {
	be := &capture{out: `{"category":"mavjud-emas","escalate":false}`}
	r, _ := New(be, testStore()).Classify(context.Background(), "salom")
	if r.Category != "" {
		t.Errorf("ro'yxatda yo'q kategoriya qabul qilindi: %q", r.Category)
	}
}

func TestClassifyIflosJSON(t *testing.T) {
	// Model JSON atrofiga matn qo'shsa ham ishlashi kerak.
	cases := []string{
		"```json\n{\"category\":\"tolov\",\"escalate\":true}\n```",
		"Mana javob: {\"category\":\"TOLOV\",\"escalate\":true} — tayyor.",
	}
	for _, out := range cases {
		be := &capture{out: out}
		r, _ := New(be, testStore()).Classify(context.Background(), "pulimni qaytaring")
		if r.Category != "tolov" || !r.Escalate {
			t.Errorf("%q → %+v", out, r)
		}
	}
	// Buzuq javob — xato emas, shunchaki kategoriyasiz.
	be := &capture{out: "hech qanday JSON yo'q"}
	if r, err := New(be, testStore()).Classify(context.Background(), "salom"); err != nil || r.Category != "" {
		t.Errorf("buzuq javob: %+v err=%v", r, err)
	}
}

func TestClassifyKategoriyasizChaqirilmaydi(t *testing.T) {
	be := &capture{out: `{"category":"x"}`}
	// "cat:" kalitlari yo'q — router umuman chaqirilmasligi kerak.
	r, err := New(be, testPrompts{PromptClassify: "router"}).Classify(context.Background(), "salom")
	if err != nil || r.Category != "" {
		t.Errorf("route = %+v err=%v", r, err)
	}
	if be.system != "" {
		t.Error("kategoriya yo'q ekan, router chaqirilmasligi kerak edi")
	}
}

func TestAskTartibi(t *testing.T) {
	be := &capture{out: "javob"}
	_, err := New(be, testStore()).Ask(context.Background(), Request{
		Transcript:  "client: DG60582375 qayerda?",
		CategoryKey: "yetkazib-berish",
		OrderInfo:   `[{"track":"DG60582375"}]`,
		HasImage:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// KV-kesh uchun o'zgarmas qism boshda: base → sana → kategoriya →
	// buyurtma → rasm qoidasi.
	order := []string{"ASOSIY PROMPT", "Bugungi sana", "Toshkentga bepul", "DG60582375", "rasm"}
	pos := -1
	for _, part := range order {
		i := strings.Index(be.system, part)
		if i < 0 {
			t.Fatalf("system'da %q yo'q:\n%s", part, be.system)
		}
		if i < pos {
			t.Errorf("%q noto'g'ri tartibda", part)
		}
		pos = i
	}
	if !strings.HasPrefix(be.system, "ASOSIY PROMPT") {
		t.Error("base prompt eng boshda turishi kerak (KV-kesh)")
	}

	// MUHIM: mijoz matni o'zgarishsiz, buyurtma raqami aynan turadi.
	if be.user != "client: DG60582375 qayerda?" {
		t.Errorf("user qismi o'zgargan: %q", be.user)
	}
}

func TestAskKategoriyasiz(t *testing.T) {
	be := &capture{out: "javob"}
	New(be, testStore()).Ask(context.Background(), Request{Transcript: "salom"})
	if strings.Contains(be.system, "Shu savolga oid ma'lumot") {
		t.Error("kategoriya yo'q ekan, bo'lim qo'shilmasligi kerak")
	}
	if !strings.HasPrefix(be.system, "ASOSIY PROMPT") {
		t.Error("base prompt yo'q")
	}
}

func TestPromptlarHarChaqiruvdaQaytaOqiladi(t *testing.T) {
	p := testPrompts{PromptBase: "ESKI"}
	be := &capture{out: "javob"}
	c := New(be, p)

	c.Ask(context.Background(), Request{Transcript: "salom"})
	if !strings.HasPrefix(be.system, "ESKI") {
		t.Fatalf("system = %q", be.system)
	}
	// Dashboarddan tahrirlash — restart'siz keyingi javobda ko'rinishi kerak.
	p[PromptBase] = "YANGI"
	c.Ask(context.Background(), Request{Transcript: "salom"})
	if !strings.HasPrefix(be.system, "YANGI") {
		t.Errorf("yangilangan prompt ishlatilmadi: %q", be.system)
	}
}
