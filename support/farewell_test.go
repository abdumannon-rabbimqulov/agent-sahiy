package support

import "testing"

// TestIsClosingMessage - qaysi xabar suhbatni yakunlaydi.
func TestIsClosingMessage(t *testing.T) {
	closing := map[string]closingLang{
		"rahmat":             langUzLat,
		"Raxmat!":            langUzLat,
		"katta rahmat":       langUzLat,
		"hop":                langUzLat,
		"xo'p, rahmat":       langUzLat,
		"xo’p":               langUzLat,
		"mayli rahmat sizga": langUzLat,
		"раҳмат":             langUzCyr,
		"катта рахмат":       langUzCyr,
		"хоп":                langUzCyr,
		"спасибо":            langRU,
		"Спасибо большое!":   langRU,
		"хорошо, спасибо":    langRU,
		"понял":              langRU,
		"ок":                 langRU, // kirill, lekin tili noaniq → rus
		// Tili bilinmaydi — FarewellText o'zbekcha lotinga tushadi.
		"ok": langUnknown,
		"👍":  langUnknown,
	}
	for text, want := range closing {
		got, ok := IsClosingMessage(text)
		if !ok {
			t.Errorf("%q yakunlovchi deb topilmadi", text)
			continue
		}
		if got != want {
			t.Errorf("%q: til %d, kutilgani %d", text, got, want)
		}
	}
}

// TestIsClosingMessageNegative - savol yoki oddiy xabar yakunlash EMAS.
func TestIsClosingMessageNegative(t *testing.T) {
	notClosing := []string{
		"",
		"rahmat, lekin qachon keladi",
		"rahmat qachon keladi?",
		"спасибо а когда придет",
		"DG60607041",
		"rahmat DG60607041",
		"buyurtmam qani",
		"hop, lekin men kutyapman",
		"rahmat rahmat rahmat rahmat rahmat rahmat", // 5 tadan ko'p
		"assalomu alaykum",
	}
	for _, text := range notClosing {
		if _, ok := IsClosingMessage(text); ok {
			t.Errorf("%q noto'g'ri yakunlovchi deb topildi", text)
		}
	}
}

// TestFarewell - xayrlashish matni mijozning tilida bo'ladi.
func TestFarewell(t *testing.T) {
	cases := []struct {
		last string
		want string
	}{
		{"rahmat", FarewellUzLat},
		{"раҳмат", FarewellUzCyr},
		{"спасибо", FarewellRU},
	}
	if FarewellText(langUnknown) != FarewellUzLat {
		t.Error("tili noma'lum bo'lsa o'zbekcha lotin bo'lishi kerak")
	}
	if FarewellText(langUnknown) != FarewellUzLat {
		t.Error("tili noma'lum bo'lsa o'zbekcha lotin bo'lishi kerak")
	}
	for _, c := range cases {
		msgs := []Message{
			{ID: 1, SenderType: "client", Message: "buyurtmam qachon keladi"},
			{ID: 2, SenderType: "agent", Message: "Ertaga jo'natiladi"},
			{ID: 3, SenderType: "client", Message: c.last},
		}
		got, ok := Farewell(msgs)
		if !ok {
			t.Errorf("%q: xayrlashish topilmadi", c.last)
			continue
		}
		if got != c.want {
			t.Errorf("%q: %q, kutilgani %q", c.last, got, c.want)
		}
	}
}

// TestFarewellSkips - xodim javobi, rasm va savol xayrlashuv emas.
func TestFarewellSkips(t *testing.T) {
	cases := map[string][]Message{
		"oxirgi so'z xodimniki": {
			{ID: 1, SenderType: "client", Message: "rahmat"},
			{ID: 2, SenderType: "agent", Message: "Rahmat sizga ham"},
		},
		"rasm": {
			{ID: 1, SenderType: "client", Message: "https://cdn.example.com/chat-images/a.jpg"},
		},
		"savol": {
			{ID: 1, SenderType: "client", Message: "qachon keladi"},
		},
		"bo'sh": {},
	}
	for name, msgs := range cases {
		if _, ok := Farewell(msgs); ok {
			t.Errorf("%s: xayrlashish qaytdi", name)
		}
	}
}
