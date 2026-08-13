package pricing

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestLookupAniqVaPrefiks(t *testing.T) {
	defer Set(0, 0, 0)
	if p, ok := Lookup("gpt-4o-mini"); !ok || p.In != 0.15 {
		t.Errorf("aniq nom topilmadi: %+v ok=%v", p, ok)
	}
	// Sana qo'shilgan nom ham topilishi kerak.
	if p, ok := Lookup("gpt-4o-mini-2024-07-18"); !ok || p.In != 0.15 {
		t.Errorf("prefiks bo'yicha topilmadi: %+v ok=%v", p, ok)
	}
	// Eng uzun mos prefiks tanlanadi: gpt-4.1 emas, gpt-4.1-mini.
	if p, _ := Lookup("gpt-4.1-mini-2025-04-14"); p.In != 0.40 {
		t.Errorf("uzun prefiks tanlanmadi: %+v", p)
	}
	if _, ok := Lookup("qandaydir-model"); ok {
		t.Error("noma'lum model topilmasligi kerak")
	}
}

func TestCost(t *testing.T) {
	p := Price{In: 0.15, CachedIn: 0.075, Out: 0.60}
	// 1M kirish (keshsiz) + 1M chiqish = 0.15 + 0.60
	if got := p.Cost(1_000_000, 0, 1_000_000); !near(got, 0.75) {
		t.Errorf("cost = %v, kutilgan 0.75", got)
	}
	// Yarmi kesh'dan: 500k*0.15 + 500k*0.075 = 0.075 + 0.0375
	if got := p.Cost(1_000_000, 500_000, 0); !near(got, 0.1125) {
		t.Errorf("kesh hisobi = %v, kutilgan 0.1125", got)
	}
	// cached > prompt bo'lib qolsa ham manfiy chiqmaydi.
	if got := p.Cost(100, 500, 0); got < 0 {
		t.Errorf("manfiy narx: %v", got)
	}
}

func TestCachedInBoshBolsaKirishNarxi(t *testing.T) {
	p := Price{In: 0.10, Out: 0.40} // Gemini: alohida kesh narxi yo'q
	if got := p.Cost(1_000_000, 1_000_000, 0); !near(got, 0.10) {
		t.Errorf("cost = %v, kutilgan 0.10", got)
	}
}

func TestEnvOverride(t *testing.T) {
	defer Set(0, 0, 0)
	Set(1, 0, 2)
	p, ok := Lookup("gpt-4o-mini") // jadvalda bor, lekin override ustun
	if !ok || p.In != 1 || p.Out != 2 {
		t.Errorf("override ishlamadi: %+v", p)
	}
	if _, ok := Lookup("mutlaqo-notanish"); !ok {
		t.Error("override bo'lsa noma'lum model ham hisoblanishi kerak")
	}
	Set(0, 0, 0)
	if _, ok := Lookup("mutlaqo-notanish"); ok {
		t.Error("override o'chirilgach noma'lum model topilmasligi kerak")
	}
}

func TestCostOfNomalumModel(t *testing.T) {
	c, ok := CostOf("yo-q-model", 1000, 0, 1000)
	if ok || c != 0 {
		t.Errorf("noma'lum model uchun (0,false) kutilgan, keldi (%v,%v)", c, ok)
	}
}
