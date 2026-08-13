package models

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Yetkazib berish":     "yetkazib-berish",
		"To'lov va qaytarish": "tolov-va-qaytarish",
		"  Bo'sh   joylar  ":  "bosh-joylar",
		"Narx/Muddat":         "narx-muddat",
		"":                    "",
		"123":                 "123",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, kutilgan %q", in, got, want)
		}
	}
}

func TestCatKey(t *testing.T) {
	if got := CatKey("yetkazib-berish"); got != "cat:yetkazib-berish" {
		t.Errorf("CatKey = %q", got)
	}
}
