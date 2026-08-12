package orders

import (
	"reflect"
	"testing"
)

func TestTracks(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"buyurtmam 79017498359954 qayerda?", []string{"79017498359954"}},
		{"track YT7594703873671 va LP00123456789CN", []string{"YT7594703873671", "LP00123456789CN"}},
		// Oddiy so'zlar va telefon raqami track deb olinmasligi kerak.
		{"BUYURTMAM QAYERDA", nil},
		{"tel 998903047334", nil},
		{"3 dona, id 4417", nil}, // 8 xonadan qisqa
		{"79017498359954 va yana 79017498359954", []string{"79017498359954"}},
	}
	for _, c := range cases {
		if got := Tracks(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Tracks(%q) = %v, kutilgan %v", c.in, got, c.want)
		}
	}
}

func TestSummaryBoshHolat(t *testing.T) {
	if Summary(nil) != "" {
		t.Error("bo'sh ro'yxatdan bo'sh satr kutilgan")
	}
}
