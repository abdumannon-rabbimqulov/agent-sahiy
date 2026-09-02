package support

import (
	"strings"
	"testing"
)

func TestExtractNumbers(t *testing.T) {
	msgs := []Message{
		{SenderType: "client", Message: "78975877791396 shu buyurtmani ham"},
		{SenderType: "client", Message: "JT3147778954467 va DG60645244"},
		{SenderType: "agent", Message: "DG99999999 bo'yicha tekshiryapmiz"}, // bizniki — olinmaydi
		{SenderType: "client", Message: "дг60619846 qayerda?"},              // kirillcha
		{SenderType: "client", Message: "998901234567 raqamimga qo'ng'iroq qiling"},
	}

	sn, ex := ExtractNumbers(msgs)

	wantSN := map[string]bool{"DG60645244": true, "DG60619846": true}
	if len(sn) != 2 {
		t.Fatalf("2 ta buyurtma raqami kutilgan: %v", sn)
	}
	for _, v := range sn {
		if !wantSN[strings.ToUpper(v)] {
			t.Errorf("kutilmagan buyurtma raqami: %s", v)
		}
	}

	joined := strings.Join(ex, ",")
	for _, want := range []string{"78975877791396", "JT3147778954467"} {
		if !strings.Contains(joined, want) {
			t.Errorf("trek raqami topilmadi: %s (%v)", want, ex)
		}
	}
	// Bizning javobimizdagi raqam olinmasin.
	if strings.Contains(strings.Join(sn, ","), "99999999") {
		t.Error("agent xabaridagi raqam olindi")
	}
}

func TestMergeNumbers(t *testing.T) {
	got := mergeNumbers([]string{"DG1", "DG2"}, []string{"dg2", "DG3", ""}, 10)
	if strings.Join(got, ",") != "DG1,DG2,DG3" {
		t.Errorf("birlashtirish: %v", got)
	}
	if len(mergeNumbers([]string{"a", "b", "c"}, []string{"d"}, 2)) != 2 {
		t.Error("chegara ishlamadi")
	}
}
