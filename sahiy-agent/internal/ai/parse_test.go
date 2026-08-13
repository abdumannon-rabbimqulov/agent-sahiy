package ai

import (
	"strings"
	"testing"
)

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
