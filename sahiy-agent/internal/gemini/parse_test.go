package gemini

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseImageAnswer(t *testing.T) {
	in := "**Raqamlar:** DG60582375, 79019342930966\n**Tavsif:** Buyurtma tafsilotlari skrinshoti."
	nums, desc := ParseImageAnswer(in)
	if want := []string{"DG60582375", "79019342930966"}; !reflect.DeepEqual(nums, want) {
		t.Errorf("raqamlar = %v, kutilgan %v", nums, want)
	}
	if desc != "Buyurtma tafsilotlari skrinshoti." {
		t.Errorf("tavsif = %q", desc)
	}
}

func TestParseImageAnswerRaqamsiz(t *testing.T) {
	nums, desc := ParseImageAnswer("Raqamlar: yo'q\nTavsif: Ko'ylak surati.")
	if len(nums) != 0 {
		t.Errorf("raqamlar bo'sh bo'lishi kerak, keldi: %v", nums)
	}
	if desc != "Ko'ylak surati." {
		t.Errorf("tavsif = %q", desc)
	}
}

func TestParseImageAnswerFormatsiz(t *testing.T) {
	// Model formatga rioya qilmasa — butun matn tavsif bo'lib qoladi.
	_, desc := ParseImageAnswer("Rasmda quti ko'rinadi")
	if desc != "Rasmda quti ko'rinadi" {
		t.Errorf("tavsif = %q", desc)
	}
}

func TestSplitDaraja(t *testing.T) {
	cases := []struct {
		in   string
		want Daraja
	}{
		{"Daraja: yuqori\nMuammo: buyurtma yo'qolgan", Yuqori},
		{"**Daraja:** past\nMuammo: oddiy savol", Past},
		{"Daraja: o'rta\nMuammo: tekshirish kerak", Orta},
		{"Muammo: daraja qatori yo'q", Orta}, // default
	}
	for _, c := range cases {
		got, body := splitDaraja(c.in)
		if got != c.want {
			t.Errorf("splitDaraja(%q) = %q, kutilgan %q", c.in, got, c.want)
		}
		if strings.Contains(strings.ToLower(body), "daraja:") {
			t.Errorf("Daraja qatori xulosadan olib tashlanishi kerak: %q", body)
		}
	}
}

func TestDarajaBelgi(t *testing.T) {
	if Yuqori.Belgi() != "🔴" || Past.Belgi() != "🟢" || Orta.Belgi() != "🟡" {
		t.Error("belgilar mos emas")
	}
	if Yuqori.Sarlavha() != "YUQORI" {
		t.Error("sarlavha mos emas")
	}
}
