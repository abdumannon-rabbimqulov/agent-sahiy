package support

import "testing"

func TestWithOrderSN(t *testing.T) {
	cases := []struct {
		name string
		text string
		sns  []string
		want string
	}{
		{"raqam yo'q — qo'shiladi", "Ertaga jo'natiladi.", []string{"DG1"},
			"DG1 — Ertaga jo'natiladi."},
		{"raqam bor — takrorlanmaydi", "DG1 buyurtmangiz yo'lda.", []string{"DG1"},
			"DG1 buyurtmangiz yo'lda."},
		{"ikkitadan biri yozilgan", "DG1 tayyor.", []string{"DG1", "DG2"},
			"DG2 — DG1 tayyor."},
		{"ikkalasi ham yo'q", "Tayyor.", []string{"DG1", "DG2"},
			"DG1, DG2 — Tayyor."},
		{"raqam berilmagan", "Tayyor.", nil, "Tayyor."},
		{"bo'sh matn", "  ", []string{"DG1"}, ""},
	}
	for _, c := range cases {
		if got := WithOrderSN(c.text, c.sns); got != c.want {
			t.Errorf("%s: %q, kutilgan %q", c.name, got, c.want)
		}
	}
}

func TestIssueNumbers(t *testing.T) {
	got := issueNumbers([]OrderIssue{{OrderSN: "DG1"}, {OrderSN: " "}, {OrderSN: "DG2"}})
	if len(got) != 2 || got[0] != "DG1" || got[1] != "DG2" {
		t.Errorf("issueNumbers = %v", got)
	}
}
