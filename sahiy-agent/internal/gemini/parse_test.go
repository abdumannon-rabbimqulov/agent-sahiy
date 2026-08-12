package gemini

import (
	"reflect"
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
