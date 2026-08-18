package ai

import (
	"context"
	"fmt"
)

// Fallback — ikkita backend: asosiysi ishlamay qolsa zaxirasi ishlaydi.
//
// Nima uchun: lokal model (Ollama) hostda ishlaydi va u o'chib qolsa yoki
// RAM yetmasa agent butunlay javob bera olmay qoladi. Zaxira bulut modeli
// shu teshikni yopadi — sozlanmagan bo'lsa xatti-harakat avvalgidek.
//
// Zaxiraga QACHON o'tiladi: asosiy backend javobni umuman bermaganda.
// Javob kelib, yonida xato ham bo'lsa (masalan kontekst to'lgan) — bu
// qisman muvaffaqiyat, javob ishlatiladi va zaxira chaqirilmaydi.
type Fallback struct {
	primary   Backend
	secondary Backend
}

// NewFallback — zaxirali backend. secondary nil yoki tayyor emas bo'lsa
// asosiysining o'zi qaytadi (ortiqcha qatlam qo'shilmaydi).
func NewFallback(primary, secondary Backend) Backend {
	if secondary == nil || !secondary.Ready() {
		return primary
	}
	return &Fallback{primary: primary, secondary: secondary}
}

// Name — "ollama llama3.1:8b → groq openai/gpt-oss-120b". Ajratgich " → "
// ataylab: main.go dagi modelName() shu bo'yicha asosiy modelni ajratadi
// (narx jadvalida qidirish uchun).
func (f *Fallback) Name() string {
	return f.primary.Name() + " → " + f.secondary.Name()
}

// Ready — bittasi tayyor bo'lsa yetadi.
func (f *Fallback) Ready() bool { return f.primary.Ready() || f.secondary.Ready() }

// Generate — avval asosiysi; javob bo'lmasa zaxirasi.
//
// Asosiysining xatosi yo'qolmaydi: u ctx'dagi hisoblagichga ogohlantirish
// bo'lib tushadi, ya'ni dashboardda "lokal model ishlamadi" ko'rinadi.
func (f *Fallback) Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error) {
	if f.primary.Ready() {
		out, u, err := f.primary.Generate(ctx, system, user, opt)
		if out != "" {
			return out, u, err
		}
		if err == nil {
			return out, u, nil
		}
		meterFrom(ctx).Warn(fmt.Sprintf("%s ishlamadi, zaxiraga o'tildi: %v",
			f.primary.Name(), err))
	}
	return f.secondary.Generate(ctx, system, user, opt)
}
