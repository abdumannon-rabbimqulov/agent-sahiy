// Rasmdan buyurtma (DG…) va trek (JT…, 7897…) raqamlarini o'qish.
//
// Mijoz ko'pincha raqamni yozmaydi — skrinshot yoki chek rasmini tashlaydi.
// Asosiy zanjirdagi model rasmni ko'ra olmaydi: transcript'ga rasm
// "[rasm yuborildi]" bo'lib tushadi (agent.go/formatTranscript).
//
// Rasmni tesseract OCR o'qiydi — bepul, lokal, modelsiz (ocr.go).
// Ko'ruvchi (vision) modelga umuman borilmaydi: buyurtma va trek raqami
// bosma matn, uni o'qish uchun LLM shart emas.
//
// Ish tartibi:
//
//	res, ok := ReadNumbersFromMessages(ctx, msgs)
//	if !ok { ... }   // rasm yo'q, o'qilmadi yoki ichida raqam yo'q
//	// ok bo'lsa res.OrderSN / res.Express birinchi promtga beriladi
package support

import (
	"context"
	"errors"
	"log"
	"strings"
)

// DefaultMaxImages - bitta suhbatda nechta rasm o'qiladi. Mijoz 5 ta rasm
// tashlasa ham hammasini o'qish shart emas; eng oxirgilari odatda o'sha
// murojaatga tegishli bo'ladi.
const DefaultMaxImages = 2

// MaxImages - bir suhbatda o'qiladigan rasm soni (.env: MAX_IMAGES).
func MaxImages() int { return envInt("MAX_IMAGES", DefaultMaxImages) }

// ImageNumbers - bitta yoki bir necha rasmdan o'qilgan raqamlar.
//
// Links/Raw/Model panelda ko'rsatish uchun: suhbat tafsilotida qaysi rasm
// o'qilgani va undan nima chiqqani ochiq turishi kerak — aks holda
// "raqam qayerdan keldi" degan savolga javob topib bo'lmaydi.
type ImageNumbers struct {
	OrderSN []string `json:"order_sn"`    // DG… buyurtma raqamlari
	Express []string `json:"express_num"` // trek raqamlari
	Text    string   `json:"text"`        // rasmdan o'qilgan xom matn
	Images  int      `json:"images"`      // nechta rasm o'qildi
	Links   []string `json:"links"`       // o'qilgan rasm havolalari
	Raw     string   `json:"raw"`         // OCR ning xom natijasi (panel uchun)
	Model   string   `json:"model"`       // nima o'qidi ("tesseract eng")
}

// Empty - hech qanday raqam topilmadimi.
func (n ImageNumbers) Empty() bool { return len(n.OrderSN) == 0 && len(n.Express) == 0 }

// All - topilgan hamma raqam (log va xabar uchun).
func (n ImageNumbers) All() []string { return mergeNumbers(n.OrderSN, n.Express, 20) }

// ClientImageLinks - MIJOZ yuborgan rasm havolalari, yangisidan eskisiga
// (eng oxirgi rasm birinchi: murojaatga aynan o'sha tegishli).
// Xodim yuborgan rasmlar olinmaydi.
func ClientImageLinks(msgs []Message) []string {
	var out []string
	seen := map[string]bool{}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if !m.FromClient() {
			continue
		}
		link := strings.TrimSpace(m.Message)
		if !isImageLink(link) || seen[link] {
			continue
		}
		seen[link] = true
		out = append(out, link)
	}
	return out
}

// HasClientImage - mijoz rasm yuborganmi.
func HasClientImage(msgs []Message) bool { return len(ClientImageLinks(msgs)) > 0 }

// ReadNumbersFromMessages - suhbatdagi mijoz rasmlarini tesseract bilan
// o'qib, ichidagi buyurtma va trek raqamlarini qaytaradi.
//
// Eng oxirgi rasmdan boshlanadi va BIRINCHI raqam topilgan rasmda
// to'xtaydi. O'qiladigan rasm soni `MAX_IMAGES` bilan cheklangan.
//
// Ikkinchi qiymat — raqam topildimi. false bo'lsa sabab uchta bo'lishi
// mumkin va uchalasida ham keyingi qadam bir xil: mijozdan raqamni matn
// bilan yozish so'raladi.
//   - mijoz rasm yubormagan;
//   - rasm o'qilmadi (tesseract yo'q, havola ishlamadi) — logga yoziladi;
//   - rasm o'qildi, lekin ichida buyurtma yoki trek raqami yo'q.
func ReadNumbersFromMessages(ctx context.Context, msgs []Message) (ImageNumbers, bool) {
	links := ClientImageLinks(msgs)
	if len(links) == 0 {
		return ImageNumbers{}, false
	}
	if n := MaxImages(); len(links) > n {
		links = links[:n]
	}
	if !OCREnabled() {
		return ImageNumbers{}, false
	}

	var res ImageNumbers
	for _, link := range links {
		one, err := ReadImageOCR(ctx, link)
		if errors.Is(err, ErrNoOCR) {
			// tesseract yo'q — qolgan rasmni sinash befoyda.
			log.Printf("ocr: %v", err)
			res.Raw = appendRaw(res.Raw, "xato: "+err.Error())
			return res, false
		}
		res.Images++
		res.Links = append(res.Links, link)
		res.Model = "tesseract " + OCRLangs()
		if err != nil {
			// Bitta rasm o'qilmasa — qolganini sinab ko'ramiz.
			// Xato panelda ham ko'rinsin.
			log.Printf("ocr: %v", err)
			res.Raw = appendRaw(res.Raw, "xato: "+err.Error())
			continue
		}
		res.Raw = appendRaw(res.Raw, one.Raw)
		res.OrderSN = mergeNumbers(res.OrderSN, one.OrderSN, 10)
		res.Express = mergeNumbers(res.Express, one.Express, 10)
		if one.Text != "" {
			res.Text = one.Text
		}
		if !res.Empty() {
			return res, true // topildi — qolgan rasmlar shart emas
		}
	}
	return res, false
}

// appendRaw - bir necha rasm natijasini bitta matnga qo'shadi (panel uchun).
func appendRaw(acc, add string) string {
	add = strings.TrimSpace(add)
	if add == "" {
		return acc
	}
	if acc == "" {
		return add
	}
	return acc + "\n\n" + add
}

// numbersFromText - matndan raqamlarni ajratadi (numbers.go dagi bir xil
// qoidalar bilan: rasmdan o'qilgan matn ham mijoz yozgani kabi ishlanadi).
func numbersFromText(text string) (orderSN, express []string) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return ExtractNumbers([]Message{{SenderType: "client", Message: text}})
}
