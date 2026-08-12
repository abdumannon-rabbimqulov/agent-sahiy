package tgtext

import "testing"

func TestBuildUTF16Offsets(t *testing.T) {
	// 🆘 = 2 UTF-16 birlik, ✓ = 1 birlik.
	text, spans := Build("🆘 ID `7235` va `4417`")
	if text != "🆘 ID 7235 va 4417" {
		t.Fatalf("matn: %q", text)
	}
	if len(spans) != 2 {
		t.Fatalf("spanlar: %+v", spans)
	}
	// "🆘"(2) + " "(1) + "ID"(2) + " "(1) = 6
	if spans[0] != (Span{Offset: 6, Length: 4}) {
		t.Errorf("1-span: %+v", spans[0])
	}
	// 6 + 4 + " va "(4) = 14
	if spans[1] != (Span{Offset: 14, Length: 4}) {
		t.Errorf("2-span: %+v", spans[1])
	}
}

func TestMarkNumbers(t *testing.T) {
	got := MarkNumbers("Buyurtma №4417, tel 998901234567, 3 dona, id 12")
	want := "Buyurtma `№4417`, tel `998901234567`, 3 dona, id 12"
	if got != want {
		t.Errorf("got %q", got)
	}
}
