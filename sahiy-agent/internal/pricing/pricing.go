// Package pricing — model narxlari va token → USD hisobi.
//
// Narxlar 1 million token uchun USD'da. Kodda faqat boshlang'ich jadval bor;
// u eskirishi mumkin, shuning uchun .env orqali ustidan yozish mumkin
// (AI_PRICE_IN / AI_PRICE_CACHED_IN / AI_PRICE_OUT).
package pricing

import (
	"fmt"
	"sort"
	"strings"
)

// Price — 1 million token uchun narx (USD).
type Price struct {
	In       float64 // kirish tokeni
	CachedIn float64 // kesh'dan olingan kirish tokeni (arzonroq); 0 bo'lsa In ishlatiladi
	Out      float64 // chiqish tokeni
}

// table — boshlang'ich narxlar.
//
// ⚠️ MUHIM: bu raqamlar qo'lda yozilgan va provayder narxni o'zgartirsa
// eskiradi. Ishlatishdan oldin provayder billing sahifasi bilan solishtiring;
// farq bo'lsa .env dagi AI_PRICE_* bilan ustidan yozing — kodga tegish shart emas.
var table = map[string]Price{
	// OpenAI
	"gpt-4o-mini":  {In: 0.15, CachedIn: 0.075, Out: 0.60},
	"gpt-4o":       {In: 2.50, CachedIn: 1.25, Out: 10.00},
	"gpt-4.1-mini": {In: 0.40, CachedIn: 0.10, Out: 1.60},
	"gpt-4.1-nano": {In: 0.10, CachedIn: 0.025, Out: 0.40},
	"gpt-4.1":      {In: 2.00, CachedIn: 0.50, Out: 8.00},
	// Google Gemini
	"gemini-2.5-flash-lite": {In: 0.10, Out: 0.40},
	"gemini-2.5-flash":      {In: 0.30, Out: 2.50},
	"gemini-2.0-flash":      {In: 0.10, Out: 0.40},
}

// override — .env dan berilgan narx (Set bilan o'rnatiladi).
var override *Price

// MarkFree modelni "bepul" deb belgilaydi (lokal model — Ollama). Shundan
// keyin narx noma'lum deb ogohlantirilmaydi va xarajat 0 bo'lib yoziladi.
func MarkFree(model string) {
	name := strings.ToLower(strings.TrimSpace(model))
	if name != "" {
		table[name] = Price{}
	}
}

// Free — narxi nol, ya'ni lokal (bepul) modelmi.
func (p Price) Free() bool { return p.In == 0 && p.CachedIn == 0 && p.Out == 0 }

// Set .env dagi narxni o'rnatadi. Barcha uchtasi 0 bo'lsa override o'chadi.
// cachedIn 0 bo'lsa kesh tokeni oddiy kirish narxida hisoblanadi.
func Set(in, cachedIn, out float64) {
	if in == 0 && cachedIn == 0 && out == 0 {
		override = nil
		return
	}
	override = &Price{In: in, CachedIn: cachedIn, Out: out}
}

// Lookup model narxini topadi. Avval .env override, keyin jadval: to'liq nom,
// so'ng eng uzun mos prefiks ("gpt-4o-mini-2024-07-18" → "gpt-4o-mini").
func Lookup(model string) (Price, bool) {
	if override != nil {
		return *override, true
	}
	name := strings.ToLower(strings.TrimSpace(model))
	if p, ok := table[name]; ok {
		return p, true
	}
	// Eng uzun mos prefiksni tanlaymiz — "gpt-4.1-mini-..." "gpt-4.1" ga
	// emas, "gpt-4.1-mini" ga tushishi kerak.
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.HasPrefix(name, k) {
			return table[k], true
		}
	}
	return Price{}, false
}

// Cost — sarflangan tokenlarning USD qiymati.
// cached — prompt ichidagi kesh'dan olingan qism (prompt'dan ayriladi).
func (p Price) Cost(prompt, cached, completion int) float64 {
	if cached > prompt {
		cached = prompt
	}
	cachedPrice := p.CachedIn
	if cachedPrice == 0 {
		cachedPrice = p.In
	}
	const perToken = 1_000_000.0
	return float64(prompt-cached)*p.In/perToken +
		float64(cached)*cachedPrice/perToken +
		float64(completion)*p.Out/perToken
}

// CostOf — model nomi bo'yicha hisoblaydi. Narx noma'lum bo'lsa 0 va false
// (tokenlar baribir saqlanadi, keyin narx qo'yilsa qayta hisoblash mumkin).
func CostOf(model string, prompt, cached, completion int) (float64, bool) {
	p, ok := Lookup(model)
	if !ok {
		return 0, false
	}
	return p.Cost(prompt, cached, completion), true
}

// Describe — ishga tushganda ko'rsatiladigan narx satri.
func Describe(model string) string {
	p, ok := Lookup(model)
	if !ok {
		return fmt.Sprintf("%s narxi noma'lum", model)
	}
	if p.Free() {
		return fmt.Sprintf("%s: lokal model — bepul", model)
	}
	src := "jadval"
	if override != nil {
		src = ".env"
	}
	return fmt.Sprintf("%s: kirish $%.4g / chiqish $%.4g (1M token, %s)",
		model, p.In, p.Out, src)
}
