package support

import "testing"

func TestReorderAllowed(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{StatusPaid, true},   // 3 — sotib olingan, to'langan
		{StatusBanned, true}, // 10 — taqiqlangan tovar
		{StatusWaiting, false},
		{StatusFinished, false},
		{0, false},
		{7, false},
	}
	for _, c := range cases {
		if got := ReorderAllowed(c.status); got != c.want {
			t.Errorf("ReorderAllowed(%d) = %v, kerak %v", c.status, got, c.want)
		}
	}
}

func TestMentionsReorder(t *testing.T) {
	yes := []string{
		"boshqa tovar tanlasin",
		"shu summaga boshqa mahsulot tanlang",
		"boshqa narsa tanlab bersin",
		"boshqasini tanlasin",
		"tovar taqiqlangan, jo'natilmaydi",
		"бошқа товар танланг",
		"тақиқланган товар",
		"пусть выберет другой товар",
		"товар запрещен",
	}
	for _, s := range yes {
		if !MentionsReorder(s) {
			t.Errorf("MentionsReorder(%q) = false, kerak true", s)
		}
	}

	no := []string{
		"",
		"ertaga jo'natamiz",
		"omborda qoldi, kelgusi haftada chiqadi",
		"mijozdan buyurtma raqamini so'rang",
		"посылка уже в пути",
	}
	for _, s := range no {
		if MentionsReorder(s) {
			t.Errorf("MentionsReorder(%q) = true, kerak false", s)
		}
	}
}

func TestStatusBannedLabel(t *testing.T) {
	if got := StatusLabel(StatusBanned); got != "taqiqlangan tovar" {
		t.Errorf("StatusLabel(10) = %q", got)
	}
	if StatusMeaning(StatusBanned) == "" {
		t.Error("StatusMeaning(10) bo'sh — model holatni tushunmaydi")
	}
}
