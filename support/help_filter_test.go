package support

import "testing"

func TestHelpOnlyAsksNumber(t *testing.T) {
	skip := []string{
		"Mijoz muammo haqida yozdi, lekin buyurtma raqami keltirilmagan. " +
			"Buyurtma raqami so'ralmoqda.",
		"Buyurtma raqami yo'q — mijozdan so'raldi.",
		"Mijozdan buyurtma raqamini so'radik.",
		"Заказ: номер заказа не указан, запрашивается у клиента.",
	}
	for _, s := range skip {
		if !helpOnlyAsksNumber(s) {
			t.Errorf("helpOnlyAsksNumber(%q) = false, kerak true", s)
		}
	}

	send := []string{
		"",
		"Mijoz kechikish haqida shikoyat qilmoqda, buyurtma DG60646563 tekshirish kerak.",
		"Buyurtma raqami so'ralmoqda: DG60495870 bo'yicha aniqlik yo'q.",
		"YT7639840508511 — kuryerga berilganiga 15 kun bo'ldi.",
		"Mijoz pul qaytarishni so'ramoqda, xodim ko'rib chiqsin.",
	}
	for _, s := range send {
		if helpOnlyAsksNumber(s) {
			t.Errorf("helpOnlyAsksNumber(%q) = true, kerak false", s)
		}
	}
}
