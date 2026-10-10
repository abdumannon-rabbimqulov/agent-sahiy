package support

import "testing"

func TestFlagsBlock(t *testing.T) {
	if got := flagsBlock(nil); got != "" {
		t.Errorf("bo'sh map = %q, kerak bo'sh satr", got)
	}

	got := flagsBlock(map[string]any{
		FlagCancelAsk: true,
		FlagSalom:     true,
		FlagPicked:    []string{"https://a.aliexpress.com/_mPrCbCv"},
	})
	want := flagsHeader +
		`{"bekor_qilish_sorovi":true,"salom":true,` +
		`"tanlangan_tovar":["https://a.aliexpress.com/_mPrCbCv"]}`
	if got != want {
		t.Errorf("flagsBlock =\n%s\nkerak\n%s", got, want)
	}
}

// Kalitlar tartibi barqaror bo'lishi kerak: bir xil murojaat har safar
// bir xil matn bersin, aks holda LLM keshi behuda buziladi.
func TestMapJSONStable(t *testing.T) {
	m := map[string]any{"b": 1, "a": 2, "c": 3}
	first := mapJSON(m)
	for i := 0; i < 20; i++ {
		if got := mapJSON(m); got != first {
			t.Fatalf("mapJSON tartibi o'zgardi: %s != %s", got, first)
		}
	}
	if first != `{"a":2,"b":1,"c":3}` {
		t.Errorf("mapJSON = %s", first)
	}
}
