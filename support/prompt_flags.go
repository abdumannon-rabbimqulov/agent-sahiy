// Kod topgan holatlar modelga JSON bo'lib ketadi.
//
// Ilgari kod promtning oxiriga erkin matnli KO'RSATMA yopishtirardi:
// "Javob matnida AYNAN shu buyurtma raqamini yoz…", "Bu mijozga BUGUN
// birinchi javobimiz — salom bilan boshla…", "bekor qilish haqida va'da
// berma…". Model esa bir vaqtda ikki manbadan ko'rsatma olardi —
// bazadagi promtdan va kod yopishtirgan matndan. Ular bir-biriga zid
// kelganda javob buzilardi: til almashib ketardi, ichki atamalar
// ("status") mijozga chiqardi, qadamlar o'rinsiz takrorlanardi.
//
// Endi chegara aniq:
//
//	KOD  → faqat MA'LUMOT (JSON): nima topilgani, qanday bayroqlar borligi.
//	PROMT → qanday yozish: ohang, til, taqiqlar, qadamlar.
//
// Ya'ni bu yerdagi kalitlarning MA'NOSI bazadagi promtlarda yozilgan.
// Yangi kalit qo'shsangiz, uni promtlarda ham tushuntirish kerak —
// aks holda model uni e'tiborsiz qoldiradi.
package support

import (
	"encoding/json"
	"sort"
	"strings"
)

// Promtga ketadigan bayroq nomlari. Bir joyda turadi: promt matnidagi
// nom bilan koddagi nom bir xil bo'lishi kerak.
const (
	FlagSalom      = "salom"               // bugungi birinchi javob (greeting.go)
	FlagCancelAsk  = "bekor_qilish_sorovi" // bekor qilish / pul qaytarish (cancel.go)
	FlagPicked     = "tanlangan_tovar"     // mijoz almashtirishga tovar tanladi (reorder.go)
	FlagImageNums  = "rasmdan_oqilgan_raqamlar"
	FlagImageNoNum = "rasmdan_raqam_chiqmadi"
)

// flagsHeader - JSON dan oldin turadigan qator. Ma'lumot blokidan
// ajralib tursin: model nimaga qarayotganini bilsin.
const flagsHeader = "Murojaat belgilari:\n"

// flagsBlock - bayroqlar JSON i ("Murojaat belgilari:\n{…}").
// Bo'sh map — bo'sh satr, blok umuman qo'shilmaydi.
func flagsBlock(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	return flagsHeader + mapJSON(m)
}

// mapJSON - mapni barqaror (kalitlar tartiblangan) JSON ga aylantiradi.
// Tartib muhim: bir xil murojaat har safar bir xil matn bersin — aks
// holda LLM keshi behuda buziladi.
func mapJSON(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(m[k])
		if err != nil {
			continue
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.String()
}
