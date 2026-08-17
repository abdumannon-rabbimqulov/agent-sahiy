// Shartnoma: model qaytaradigan JSON'lar va ularni o'qish.
//
//	base       → Decision   (murojaat qaysi kategoriyaga tushadi + raqamlar)
//	cat:order  → OrderReply (mijozga nima yozish, xodimlarga nima yetkazish)
//
// Bu yerda birorta prompt matni yo'q — faqat javobning SHAKLI.
package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Decision — asosiy prompt ("base") qaytaradigan qaror. Agent endi mijozga
// matn yozmaydi: model murojaatni kategoriyaga ajratadi va matndan buyurtma
// raqamlarini chiqarib beradi, harakatni esa kod tanlaydi.
//
// Kutilayotgan JSON shakllari:
//
//	{"dashboard":true,"adminka":true,"order_sn":[],"express_num":[]}
//	{"incorrect_order":true,"order_sn":[],"express_num":[]}
//	{"deliver":true}
//	{"category":false}
type Decision struct {
	Dashboard      bool `json:"dashboard"`
	Adminka        bool `json:"adminka"`
	IncorrectOrder bool `json:"incorrect_order"`
	Deliver        bool `json:"deliver"`
	// Category — model faqat `false` qaytaradi ("mos kategoriya yo'q").
	// Ko'rsatkich, chunki maydonning umuman yo'qligi ham ma'noli.
	Category   *bool    `json:"category"`
	OrderSN    []string `json:"order_sn"`
	ExpressNum []string `json:"express_num"`

	// Raw — model qaytargan asl matn (log va dashboard uchun).
	Raw string `json:"-"`
}

// Kind — qaror turi.
type Kind string

const (
	// KindOrderStatus — buyurtma qachon keladi / yo'lga chiqdimi.
	KindOrderStatus Kind = "order_status"
	// KindIncorrectOrder — muddat o'tgan, yo'qolgan, shikastlangan,
	// noto'g'ri tovar, pul qaytarish yoki to'lov nizosi.
	KindIncorrectOrder Kind = "incorrect_order"
	// KindDeliver — yetkazib berish shartlari haqida umumiy savol.
	KindDeliver Kind = "deliver"
	// KindNone — hech qaysi kategoriyaga tushmadi.
	KindNone Kind = "none"
)

// Kind qaror turini aniqlaydi. Model bir vaqtda bir nechta bayroq qaytarsa
// eng "og'ir" holat ustun keladi: muammo (incorrect_order) → buyurtma holati
// → umumiy savol.
func (d Decision) Kind() Kind {
	switch {
	case d.IncorrectOrder:
		return KindIncorrectOrder
	case d.Dashboard || d.Adminka:
		return KindOrderStatus
	case d.Deliver:
		return KindDeliver
	}
	return KindNone
}

// Sources — buyurtma ma'lumoti qaysi manbalardan olinishi kerak:
// delivery — Dashboard (O'zbekistondagi holat), daigou — Adminka (Xitoy
// tomoni). Model ikkala bayroqni ham qo'ymasa ikkalasi ham so'raladi —
// modelning xatosi tufayli javob ma'lumotsiz qolmasligi kerak.
func (d Decision) Sources() (delivery, daigou bool) {
	if !d.Dashboard && !d.Adminka {
		return true, true
	}
	return d.Dashboard, d.Adminka
}

// Label — log va trace uchun o'zbekcha nom.
func (d Decision) Label() string {
	switch d.Kind() {
	case KindIncorrectOrder:
		return "Buyurtmada muammo (incorrect_order)"
	case KindOrderStatus:
		return "Buyurtma holati (dashboard/adminka)"
	case KindDeliver:
		return "Yetkazib berish shartlari (deliver)"
	}
	return "Kategoriyasiz (category:false)"
}

// Numbers — modeldan kelgan buyurtma va ekspress raqamlar bir ro'yxatda
// (bo'shlari va takrorlari tashlanadi). lookupOrders shu ro'yxatni oladi.
func (d Decision) Numbers() []string {
	seen := make(map[string]bool)
	var out []string
	for _, n := range append(append([]string{}, d.OrderSN...), d.ExpressNum...) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// NumbersIn — Numbers() dan FAQAT suhbat matnida haqiqatan uchraganlarini
// qaytaradi. Model buyurtma raqamini o'zidan to'qishi mumkin (masalan
// "12345"), bunday raqam bo'yicha qidiruv bekorga API so'rov yeydi va
// noto'g'ri natijaga olib kelishi mumkin.
//
//	kept     — ishonchli raqamlar
//	invented — model to'qigan, tashlab yuborilganlari (trace uchun)
func (d Decision) NumbersIn(transcript string) (kept, invented []string) {
	up := strings.ToUpper(transcript)
	for _, n := range d.Numbers() {
		if strings.Contains(up, strings.ToUpper(n)) {
			kept = append(kept, n)
		} else {
			invented = append(invented, n)
		}
	}
	return kept, invented
}

// ParseDecision model javobidan qarorni ajratadi. Model JSON atrofiga matn
// yoki ```json bloki qo'shsa ham ishlaydi. Xato bo'lsa ham Raw to'ldirilgan
// Decision qaytadi — chaqiruvchi uni logga yozishi mumkin.
func ParseDecision(out string) (Decision, error) {
	d := Decision{Raw: strings.TrimSpace(out)}
	s := extractJSON(out)
	if s == "" {
		return d, fmt.Errorf("javobda JSON topilmadi: %q", d.Raw)
	}
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return d, fmt.Errorf("qaror JSON'i o'qilmadi: %w (javob: %q)", err, d.Raw)
	}
	return d, nil
}

// OrderReply — "cat:order" prompti qaytaradigan JSON. Buyurtmadagi muammo
// (yo'qolgan, shikastlangan, muddat o'tgan, pul qaytarish) shu prompt orqali
// o'tadi va u ikkita alohida matn qaytaradi:
//
//	{"client":"mijozga yoziladigan javob","help":"xodimlar uchun izoh"}
//
// Kerak bo'lmagan maydon bo'sh qoladi: client bo'sh bo'lsa mijozga hech narsa
// yuborilmaydi, help bo'sh bo'lsa guruhga hech narsa ketmaydi.
type OrderReply struct {
	Client string `json:"client"`
	Help   string `json:"help"`

	// Raw — model qaytargan asl matn (log va dashboard uchun).
	Raw string `json:"-"`
}

// Empty — model ikkala maydonni ham bo'sh qoldirgan (harakat qilinmaydi).
func (r OrderReply) Empty() bool { return r.Client == "" && r.Help == "" }

// ParseOrderReply "cat:order" javobidan matnlarni ajratadi. JSON atrofidagi
// ortiqcha matn yoki ```json bloki e'tiborga olinmaydi.
func ParseOrderReply(out string) (OrderReply, error) {
	r := OrderReply{Raw: strings.TrimSpace(out)}
	s := extractJSON(out)
	if s == "" {
		return r, fmt.Errorf("javobda JSON topilmadi: %q", r.Raw)
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return r, fmt.Errorf("cat:order JSON'i o'qilmadi: %w (javob: %q)", err, r.Raw)
	}
	r.Client, r.Help = strings.TrimSpace(r.Client), strings.TrimSpace(r.Help)
	return r, nil
}

// extractJSON matn ichidan birinchi `{` dan oxirgi `}` gacha bo'lgan bo'lakni
// qaytaradi. Model JSON atrofiga izoh yoki ```json bloki qo'shsa ham ishlaydi.
// Topilmasa bo'sh satr.
func extractJSON(out string) string {
	s := strings.TrimSpace(out)
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return ""
	}
	return s[i : j+1]
}

// --- JSON Schema'lar ---
//
// Ollama `format` maydoniga sxema berilsa, model undan tashqariga chiqa
// olmaydi: maydon nomlari, turlari va to'plami kafolatlanadi. Shu sababli
// prompt matnida "faqat JSON yoz", "markdown ishlatma", "izoh qo'shma"
// kabi qatorlar KERAK EMAS — ular joyni egallaydi va modelni chalg'itadi.
//
// Sxema yuqoridagi struct'larga qat'iy mos bo'lishi shart: maydon nomi
// o'zgarsa ikkala joyda ham o'zgaradi (schema_test.go buni tekshiradi).

// DecisionSchema — "base" prompti qaytaradigan qaror shakli.
//
// Barcha bayroqlar `required`: model "bu kategoriya emas" degani uchun ham
// aniq `false` yozadi, ya'ni maydonning yo'qligi noaniqlik tug'dirmaydi.
//
// maxItems SHART: chegarasiz massivda 8B model bir xil raqamni takrorlab
// to'ldiraveradi va JSON token chegarasida kesilib, umuman o'qilmay
// qoladi. Bitta murojaatda 5 tadan ortiq raqam bo'lishi amalda uchramaydi.
var DecisionSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "dashboard":       {"type": "boolean"},
    "adminka":         {"type": "boolean"},
    "incorrect_order": {"type": "boolean"},
    "deliver":         {"type": "boolean"},
    "category":        {"type": "boolean"},
    "order_sn":        {"type": "array", "items": {"type": "string"}, "maxItems": 5},
    "express_num":     {"type": "array", "items": {"type": "string"}, "maxItems": 5}
  },
  "required": ["dashboard","adminka","incorrect_order","deliver","category","order_sn","express_num"],
  "additionalProperties": false
}`)

// OrderReplySchema — "order" va "cat:xato-mahsulot-kelganda" promptlari
// qaytaradigan shakl: mijozga matn va xodimlarga izoh.
var OrderReplySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "client": {"type": "string"},
    "help":   {"type": "string"}
  },
  "required": ["client","help"],
  "additionalProperties": false
}`)
