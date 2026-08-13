package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Fallback — ikki provayderni zanjirga qo'yadi: asosiysi xato bersa, so'rov
// zaxiraga tushadi. Lokal model (Ollama) o'chib qolsa mijoz javobsiz
// qolmasligi uchun.
//
// Qaysi model javob bergani Usage.Model da qoladi, ya'ni xarajat hisobi
// o'zi to'g'ri chiqadi: lokal javob $0, zaxira javob esa o'z narxida.
type Fallback struct {
	Primary   Backend
	Secondary Backend
}

// Name — ikkala provayder nomi.
func (f *Fallback) Name() string {
	if !f.secondaryUsable() {
		return f.Primary.Name()
	}
	return f.Primary.Name() + " → " + f.Secondary.Name()
}

// Ready — hech bo'lmasa bittasi ishlashga tayyor bo'lsa yetarli.
func (f *Fallback) Ready() bool {
	return (f.Primary != nil && f.Primary.Ready()) || f.secondaryUsable()
}

// secondaryUsable — zaxira mavjud va tayyormi.
func (f *Fallback) secondaryUsable() bool {
	return f.Secondary != nil && f.Secondary.Ready()
}

// Generate avval asosiy provayderga murojaat qiladi; u xato bersa zaxiraga
// o'tadi. Kontekst bekor qilinganda (dastur to'xtayapti yoki timeout) zaxira
// ishlatilmaydi — bu vaqtinchalik nosozlik emas.
func (f *Fallback) Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error) {
	if f.Primary == nil || !f.Primary.Ready() {
		if !f.secondaryUsable() {
			return "", Usage{}, fmt.Errorf("hech qaysi AI provayderi tayyor emas")
		}
		return f.Secondary.Generate(ctx, system, user, opt)
	}

	out, u, err := f.Primary.Generate(ctx, system, user, opt)
	if err == nil {
		return out, u, nil
	}
	// Kontekst to'lgani — javob baribir bor, zaxiraga o'tish shart emas:
	// chaqiruvchi ogohlantirishni o'zi ko'rsatadi.
	if out != "" {
		return out, u, err
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", u, err
	}
	if !f.secondaryUsable() {
		return "", u, err
	}

	fmt.Fprintf(os.Stderr, "⚠️  %s xato berdi (%v) — %s ga o'tildi\n",
		f.Primary.Name(), err, f.Secondary.Name())

	out, u2, err2 := f.Secondary.Generate(ctx, system, user, opt)
	if err2 != nil {
		// Ikkalasi ham ishlamadi — ikkala sababni ham ko'rsatamiz.
		return "", u2, fmt.Errorf("%s: %w; zaxira %s: %v",
			f.Primary.Name(), err, f.Secondary.Name(), err2)
	}
	return out, u2, nil
}
