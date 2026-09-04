// Rasmdan buyurtma (DG…) va trek (JT…, 7897…) raqamlarini o'qish.
//
// Mijoz ko'pincha raqamni yozmaydi — skrinshot yoki chek rasmini
// tashlaydi. Asosiy zanjirdagi model rasmni ko'ra olmaydi: transcript'ga
// rasm "[rasm yuborildi]" bo'lib tushadi (agent.go/formatTranscript).
// Shuning uchun rasm ALOHIDA, ko'ruvchi (vision) modelga beriladi va
// undan faqat RAQAMLAR olinadi — javob matnini baribir asosiy zanjir
// yozadi.
//
// Ish tartibi:
//
//	links := ClientImageLinks(msgs)        // mijoz yuborgan rasmlar
//	res, usage, err := ReadNumbersFromMessages(ctx, msgs)
//	if errors.Is(err, ErrNoNumbersInImage) { ... }  // rasmda raqam yo'q
//	// aks holda res.OrderSN / res.Express asosiy zanjirga qo'shiladi
package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultVisionModel - GROQ_VISION_MODEL bo'sh bo'lsa shu ishlatiladi.
// Asosiy zanjir modeli (gpt-oss) rasmni ko'ra olmaydi, shuning uchun
// bu alohida sozlama.
const DefaultVisionModel = "meta-llama/llama-4-scout-17b-16e-instruct"

// DefaultMaxImages - bitta suhbatda nechta rasm ko'riladi. Mijoz 5 ta
// rasm tashlasa ham hammasini modelga berish qimmat; eng oxirgilari
// odatda o'sha murojaatga tegishli bo'ladi.
const DefaultMaxImages = 2

// ErrNoNumbersInImage - rasm(lar) ko'rildi, lekin ichida buyurtma yoki
// trek raqami topilmadi. Chaqiruvchi shu xatoga qarab qaror qiladi
// (masalan mijozdan raqamni matn bilan yozishni so'raydi).
var ErrNoNumbersInImage = errors.New("rasmda buyurtma raqami yo'q")

// ErrNoImages - mijoz umuman rasm yubormagan.
var ErrNoImages = errors.New("suhbatda mijoz yuborgan rasm yo'q")

// MaxImages - bir suhbatda ko'riladigan rasm soni (.env: MAX_IMAGES).
func MaxImages() int { return envInt("MAX_IMAGES", DefaultMaxImages) }

// VisionModel - rasmni o'qiydigan model (.env: GROQ_VISION_MODEL).
func VisionModel() string { return envStr("GROQ_VISION_MODEL", DefaultVisionModel) }

// visionClient - rasm uchun ishlatiladigan klient.
//
// Odatda asosiy Groq hisobi, LEKIN manzil va kalitni alohida berish
// mumkin (VISION_BASE_URL / VISION_API_KEY): ko'ruvchi model hammada
// ham ochiq emas, shuning uchun rasmni boshqa provayderga yuborish
// kerak bo'lishi mumkin — asosiy zanjir Groq'da qolgani holda.
func visionClient() Groq {
	g := GroqFromEnv()
	if v := os.Getenv("VISION_BASE_URL"); v != "" {
		g.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("VISION_API_KEY"); v != "" {
		g.APIKey = v
	}
	return g
}

// ImageNumbers - bitta yoki bir necha rasmdan o'qilgan raqamlar.
//
// Links/Raw/Model panelda ko'rsatish uchun: suhbat tafsilotida qaysi rasm
// ko'rilgani va model nima qaytargani ochiq turishi kerak — aks holda
// "raqam qayerdan keldi" degan savolga javob topib bo'lmaydi.
type ImageNumbers struct {
	OrderSN []string `json:"order_sn"`    // DG… buyurtma raqamlari
	Express []string `json:"express_num"` // trek raqamlari
	Text    string   `json:"text"`        // rasmdan o'qilgan xom matn
	Images  int      `json:"images"`      // nechta rasm ko'rildi
	Links   []string `json:"links"`       // ko'ruvchi modelga yuborilgan rasmlar
	Raw     string   `json:"raw"`         // model qaytargan xom javob (panel uchun)
	Model   string   `json:"model"`       // qaysi model ko'rdi
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

// ReadNumbersFromMessages - suhbatdagi mijoz rasmlarini ko'rib, ichidagi
// buyurtma va trek raqamlarini qaytaradi.
//
// Eng oxirgi rasmdan boshlanadi va BIRINCHI raqam topilgan rasmda
// to'xtaydi — qolganini ko'rish behuda token. Ko'riladigan rasm soni
// `MAX_IMAGES` bilan cheklangan.
//
// Xatolar:
//   - ErrNoImages          — mijoz rasm yubormagan;
//   - ErrNoNumbersInImage  — rasm ko'rildi, raqam yo'q;
//   - qolganlari           — model/tarmoq xatosi.
func ReadNumbersFromMessages(ctx context.Context, msgs []Message) (ImageNumbers, Usage, error) {
	links := ClientImageLinks(msgs)
	if len(links) == 0 {
		return ImageNumbers{}, Usage{}, ErrNoImages
	}
	if n := MaxImages(); len(links) > n {
		links = links[:n]
	}

	// 1-yo'l: OCR (tesseract) — bepul, lokal, modelsiz. Buyurtma va trek
	// raqamlari bosma matn bo'lgani uchun ko'p holatda shu yetadi.
	if OCREnabled() {
		res, err := readAllOCR(ctx, links)
		if err == nil && !res.Empty() {
			return res, Usage{}, nil
		}
		// OCR hech narsa topmadi yoki ishlamadi — model sinab ko'radi
		// (qo'lda yozilgan, qiyshiq yoki sifatsiz rasm bo'lishi mumkin).
		if err != nil && !errors.Is(err, ErrNoOCR) {
			log.Printf("ocr: %v", err)
		}
	}

	g := visionClient()
	var (
		usage Usage
		res   ImageNumbers
		last  error
	)
	for _, link := range links {
		one, u, err := ReadImageNumbers(ctx, g, link)
		usage = usage.Add(u)
		res.Images++
		res.Links = append(res.Links, link)
		if u.Model != "" {
			res.Model = u.Model
		}
		if err != nil {
			// Bitta rasm o'qilmasa — qolganini sinab ko'ramiz.
			// Xato ham panelda ko'rinsin.
			last = err
			res.Raw = appendRaw(res.Raw, "xato: "+err.Error())
			continue
		}
		last = nil
		res.Raw = appendRaw(res.Raw, one.Raw)
		res.OrderSN = mergeNumbers(res.OrderSN, one.OrderSN, 10)
		res.Express = mergeNumbers(res.Express, one.Express, 10)
		if one.Text != "" {
			res.Text = one.Text
		}
		if !res.Empty() {
			return res, usage, nil // topildi — qolgan rasmlar shart emas
		}
	}
	if last != nil && res.Empty() {
		return res, usage, last
	}
	return res, usage, ErrNoNumbersInImage
}

// readAllOCR - rasmlarni tesseract bilan ketma-ket o'qiydi, birinchi
// raqam topilganda to'xtaydi. Model chaqirilmaydi, token sarflanmaydi.
func readAllOCR(ctx context.Context, links []string) (ImageNumbers, error) {
	var (
		res  ImageNumbers
		last error
	)
	for _, link := range links {
		one, err := ReadImageOCR(ctx, link)
		if errors.Is(err, ErrNoOCR) {
			return res, err // tesseract yo'q — qolganini sinash befoyda
		}
		res.Images++
		res.Links = append(res.Links, link)
		res.Model = "tesseract " + OCRLangs()
		if err != nil {
			last = err
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
			return res, nil
		}
	}
	return res, last
}

// visionPromt - ko'ruvchi modelga ketadigan ko'rsatma. Modeldan javob
// matni SO'RALMAYDI: u faqat raqamlarni ko'chiradi, mijozga yoziladigan
// javobni asosiy zanjir yozadi.
const visionPromt = `Sen rasmdagi matnni o'qiydigan yordamchisan.
Rasmda Sahiy Market (Xitoydan O'zbekistonga yetkazish) buyurtmasiga oid
raqamlar bo'lishi mumkin. Faqat shularni qidir:

- buyurtma raqami: "DG" (yoki kirillcha "ДГ") va undan keyingi raqamlar,
  masalan DG60607041;
- trek raqami: 1-2 ta lotin harf + uzun raqam (JT3172404674793,
  YT..., SF...) yoki 11 tadan uzun faqat raqamli kod (78975877791396).

Qoidalar:
- raqamlarni rasmda qanday bo'lsa AYNAN, LOTIN harflarda ko'chir;
- o'zingdan to'qima, taxmin qilma: aniq ko'rinmasa yozma;
- telefon raqami, narx, sana, karta raqami — bular buyurtma raqami EMAS,
  ularni olma;
- rasmda bunday raqam bo'lmasa, ikkala ro'yxatni ham bo'sh qoldir.

Faqat shu JSON'ni qaytar, boshqa hech narsa yozma:
{"order_sn": [], "express_num": [], "matn": "rasmdagi asosiy matn, 1 qator"}`

// visionReply - modeldan kutiladigan JSON.
type visionReply struct {
	OrderSN    []string `json:"order_sn"`
	ExpressNum []string `json:"express_num"`
	Matn       string   `json:"matn"`
}

// ReadImageNumbers - BITTA rasm havolasidan raqamlarni o'qiydi.
//
// Model qaytargan raqamlar ustiga rasmdagi xom matn KOD bilan ham
// tekshiriladi (numbers.go dagi regexp'lar): model raqamni tashlab
// ketsa yoki formatini buzsa ham raqam yo'qolmasin.
func ReadImageNumbers(ctx context.Context, g Groq, imageURL string) (ImageNumbers, Usage, error) {
	if !g.Ready() {
		return ImageNumbers{}, Usage{}, ErrNoGroqKey
	}
	if !isImageLink(imageURL) {
		return ImageNumbers{}, Usage{}, fmt.Errorf("rasm havolasi emas: %s", imageURL)
	}

	raw, u, err := g.generateVision(ctx, visionPromt, imageURL)
	if err != nil {
		return ImageNumbers{}, u, err
	}

	var v visionReply
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
		// Model JSON o'rniga matn qaytarsa ham raqamlarni qutqaramiz.
		sn, ex := numbersFromText(raw)
		if len(sn) == 0 && len(ex) == 0 {
			return ImageNumbers{}, u, fmt.Errorf("rasm javobi JSON emas: %s", snippet([]byte(raw)))
		}
		return ImageNumbers{OrderSN: sn, Express: ex, Text: strings.TrimSpace(raw),
			Images: 1, Links: []string{imageURL}, Raw: raw, Model: u.Model}, u, nil
	}

	// Model qaytargani + rasmdagi matndan kod topgani.
	textSN, textEx := numbersFromText(v.Matn)
	out := ImageNumbers{
		OrderSN: mergeNumbers(normalizeSN(v.OrderSN), textSN, 10),
		Express: mergeNumbers(v.ExpressNum, textEx, 10),
		Text:    strings.TrimSpace(v.Matn),
		Images:  1,
		Links:   []string{imageURL},
		Raw:     strings.TrimSpace(raw),
		Model:   u.Model,
	}
	return out, u, nil
}

// appendRaw - bir necha rasm javobini bitta matnga qo'shadi (panel uchun).
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

// normalizeSN - "ДГ…" yoki "dg 60607041" ni "DG60607041" ga keltiradi.
func normalizeSN(list []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if sn, _ := numbersFromText(s); len(sn) > 0 {
			out = append(out, sn...)
			continue
		}
		out = append(out, s)
	}
	return out
}

// --- Groq vision so'rovi -----------------------------------------------
//
// groq.go dagi Generate faqat MATN yuboradi (content — satr). Rasm uchun
// content massiv bo'lishi kerak, shuning uchun so'rov shu yerda alohida
// yig'iladi — groq.go o'zgarmasdan qoladi.

type visionContent struct {
	Type     string          `json:"type"`                // "text" | "image_url"
	Text     string          `json:"text,omitempty"`      //
	ImageURL *visionImageURL `json:"image_url,omitempty"` //
}

type visionImageURL struct {
	URL string `json:"url"`
}

type visionMessage struct {
	Role    string          `json:"role"`
	Content []visionContent `json:"content"`
}

type visionRequest struct {
	Model          string          `json:"model"`
	Messages       []visionMessage `json:"messages"`
	MaxTokens      int             `json:"max_completion_tokens,omitempty"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *groqFormat     `json:"response_format,omitempty"`
}

// generateVision - rasm + ko'rsatma yuborib, JSON matn qaytaradi.
//
// Ko'rsatma "system" da emas, "user" xabarida rasm bilan birga ketadi:
// vision modellar rasm va unga tegishli savol bitta xabarda bo'lganda
// aniqroq ishlaydi.
func (g Groq) generateVision(ctx context.Context, promt, imageURL string) (string, Usage, error) {
	body := visionRequest{
		Model: VisionModel(),
		Messages: []visionMessage{{
			Role: "user",
			Content: []visionContent{
				{Type: "text", Text: promt},
				{Type: "image_url", ImageURL: &visionImageURL{URL: imageURL}},
			},
		}},
		MaxTokens:      envInt("GROQ_VISION_MAX_TOKENS", 400),
		Temperature:    0,
		ResponseFormat: &groqFormat{Type: "json_object"},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", Usage{}, fmt.Errorf("so'rov yasash: %w", err)
	}

	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(g.BaseURL, "/") + "/chat/completions"
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+g.APIKey)
		return req, nil
	}

	start := time.Now()
	status, respBody, err := doWithRetry(&http.Client{Timeout: timeout}, newReq, Retries())
	if err != nil {
		return "", Usage{}, fmt.Errorf("groq vision so'rovi: %w", err)
	}
	ms := time.Since(start).Milliseconds()

	var out groqResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", Usage{DurationMS: ms}, fmt.Errorf("vision javobi JSON emas (status %d): %s", status, snippet(respBody))
	}

	u := Usage{
		Model:            out.Model,
		PromptTokens:     out.Usage.PromptTokens,
		CachedTokens:     out.Usage.PromptDetails.CachedTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		Calls:            1,
		DurationMS:       ms,
	}
	if u.Model == "" {
		u.Model = VisionModel()
	}

	if out.Error != nil {
		return "", u, fmt.Errorf("groq vision: %s", out.Error.Message)
	}
	if status < 200 || status >= 300 {
		return "", u, fmt.Errorf("groq vision status %d: %s", status, snippet(respBody))
	}
	if len(out.Choices) == 0 {
		return "", u, errors.New("groq vision javobi bo'sh")
	}
	return out.Choices[0].Message.Content, u, nil
}
