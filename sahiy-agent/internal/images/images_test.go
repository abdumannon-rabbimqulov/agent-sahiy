package images

import "testing"

func TestCheckURL(t *testing.T) {
	ok := []string{
		"https://storage.abusahiy.uz/client-chat-images/123-abc.jpg",
		"https://STORAGE.ABUSAHIY.UZ/x.png",
	}
	for _, u := range ok {
		if err := checkURL(u); err != nil {
			t.Errorf("checkURL(%q) = %v, xato kutilmagan", u, err)
		}
	}

	bad := []string{
		"https://evil.example.com/x.jpg",        // begona host
		"http://127.0.0.1:8080/admin",           // ichki manzil (SSRF)
		"https://storage.abusahiy.uz.evil.com/", // o'xshash host
		"file:///etc/passwd",                    // fayl sxemasi
		"://",                                   // buzuq URL
	}
	for _, u := range bad {
		if err := checkURL(u); err == nil {
			t.Errorf("checkURL(%q) xato qaytarishi kerak edi", u)
		}
	}
}

func TestExtByMime(t *testing.T) {
	if extByMime["image/jpeg"] != ".jpg" {
		t.Error("image/jpeg → .jpg kutilgan")
	}
	if _, ok := extByMime["text/html"]; ok {
		t.Error("text/html qabul qilinmasligi kerak")
	}
}
