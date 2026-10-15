package support

import (
	"strings"
	"testing"
	"time"
)

// Toshkent shahridagi mijoz, lekin jo'natma turi "Auto cargo-Pickup":
// posilkani KURYER olib bormaydi, mijoz punktdan o'zi oladi. Ilgari
// yetkazish turi faqat viloyatga qarab aniqlanar va 29 kun turgan
// posilka uchun "kuryer xizmati bilan bog'lanamiz" deb javob ketardi.
func TestBriefDeliveryExpressPickupTashkent(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -29).Format("2006-01-02 15:04:05")
	b, _ := BriefDelivery([]DeliveryOrder{{
		ExpressNum:     "773438280945555",
		City:           "Toshkent shahri",
		BranchName:     "TASHKENT CITY",
		LocationNumber: "SHOTA-28",
		BranchAddress:  "Toshkent shahri Olmazor tumani Furqat ko'chasi 2A",
		CreatedAt:      arrived,
		ExpressLine:    "Auto cargo-Pickup",
	}})

	if len(b.Pending) != 1 {
		t.Fatalf("mijozga_topshirilmagan: %d ta qator, 1 kutilgan", len(b.Pending))
	}
	izoh := b.Pending[0].Izoh
	if izoh != expressPickupNote {
		t.Errorf("izoh o'zi-olib-ketish matni emas:\n%s", izoh)
	}
	if strings.Contains(b.Kind, "kuryer mijozning manziliga") {
		t.Errorf("yetkazish_turi hali ham kuryer deb turibdi: %s", b.Kind)
	}
	if !strings.Contains(b.Kind, "punktdan o'zi olib ketadi") {
		t.Errorf("yetkazish_turi o'zi-olib-ketish emas: %s", b.Kind)
	}
	if len(b.Alerts) != 1 {
		t.Errorf("muddat o'tgani uchun 1 ta ogohlantirish kutilgan, %d ta", len(b.Alerts))
	}
}

// Toshkentdagi "Delivery" turidagi jo'natma eskicha qoladi: kuryer
// olib boradi.
func TestBriefDeliveryExpressDeliveryTashkentUnchanged(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")
	b, _ := BriefDelivery([]DeliveryOrder{{
		ExpressNum:     "773438280945556",
		City:           "Toshkent shahri",
		BranchName:     "TASHKENT CITY",
		LocationNumber: "SHOTA-28",
		CreatedAt:      arrived,
		ExpressLine:    "Auto cargo-Delivery",
	}})
	if len(b.Pending) != 1 || b.Pending[0].Izoh != courierPendingNote {
		t.Fatalf("kuryer turidagi jo'natma izohi o'zgarib ketdi: %+v", b.Pending)
	}
}

// Ikki xil turdagi posilka bo'lsa, umumiy "yetkazish_turi" matni olib
// tashlanadi — u ikkalasiga ham to'g'ri kelmaydi.
func TestBriefDeliveryMixedKindsClearsKind(t *testing.T) {
	arrived := time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05")
	b, _ := BriefDelivery([]DeliveryOrder{
		{ExpressNum: "1", City: "Toshkent shahri", BranchName: "TASHKENT CITY",
			LocationNumber: "SHOTA-28", CreatedAt: arrived, ExpressLine: "Auto cargo-Pickup"},
		{ExpressNum: "2", City: "Toshkent shahri", BranchName: "TASHKENT CITY",
			LocationNumber: "SHOTA-28", CreatedAt: arrived, ExpressLine: "Auto cargo-Delivery"},
	})
	if b.Kind != "" {
		t.Errorf("aralash turda yetkazish_turi bo'sh bo'lishi kerak: %s", b.Kind)
	}
}
