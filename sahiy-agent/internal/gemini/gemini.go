package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://generativelanguage.googleapis.com/v1beta/models"

// maxRetries — 429/5xx uchun qayta urinishlar soni.
const maxRetries = 3

// Client Gemini API bilan ishlaydi.
type Client struct {
	APIKey string
	Model  string // masalan "gemini-2.5-flash"
	Prompt string // tizim (system) prompt — prompt.txt yoki .env'dan
	http   *http.Client
}

// New yangi Gemini client yaratadi.
func New(apiKey, model, systemPrompt string) *Client {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Client{
		APIKey: apiKey,
		Model:  model,
		Prompt: systemPrompt,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// --- so'rov/javob tuzilmalari ---

type genRequest struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
}

// inlineData — rasm baytlari base64 ko'rinishida (Gemini vision).
type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type genResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Ask transkriptni (chat tarixi) yuborib, agent javobini qaytaradi.
// extra — tanlangan kategoriya ma'lumoti (bo'sh bo'lishi mumkin).
// orderInfo — id/track raqami bo'yicha topilgan buyurtma holati (xom JSON).
func (c *Client) Ask(ctx context.Context, transcript, extra, orderInfo string) (string, error) {
	system := c.Prompt
	if extra != "" {
		system += "\n\n--- Shu savolga oid ma'lumot ---\n" + extra +
			"\n\nJavobingni faqat shu ma'lumotga tayanib yoz. Bu yerda yo'q narsani o'ylab topma."
	}
	if orderInfo != "" {
		system += "\n\n--- Mijozning buyurtmasi (tizimdan olingan, real holat) ---" + orderInfo +
			"\n\nBu JSON tizimdan olingan haqiqiy ma'lumot. Mijozga uni tushunarli\n" +
			"tilda tushuntir: buyurtma qayerda, holati nima, keyingi qadam nima.\n" +
			"JSON'ni o'zini ko'chirib yozma. Bu yerda yo'q maydonni o'ylab topma.\n" +
			"Agar ma'lumot savolga javob bermasa yoki ziddiyatli bo'lsa — #ESCALATE yoz."
	}
	return c.generate(ctx, system, transcript)
}

// Summarize suhbatni xodimlar guruhi uchun qisqa xulosaga aylantiradi:
// mijozning umumiy muammosi, muhim tafsilotlar va xodimdan nima kerakligi.
func (c *Client) Summarize(ctx context.Context, transcript, orderInfo string) (string, error) {
	system := "Sen support jamoasiga muammoni tushuntiruvchi yordamchisan.\n" +
		"Quyida mijoz bilan bo'lgan suhbat tarixi berilgan. Uni to'liq o'qib chiq va\n" +
		"navbatchi xodim uchun qisqa xulosa yoz — xodim suhbatni o'qimasdan ham\n" +
		"muammoni tushunishi kerak.\n\n" +
		"Aynan quyidagi 3 qatorni yoz (har biri 1-2 gap, o'zbek tilida):\n" +
		"Muammo: <mijozning umumiy muammosi>\n" +
		"Tafsilot: <suhbatdagi muhim faktlar: buyurtma/tovar nomi, raqam, sana, nima urinib ko'rilgan>\n" +
		"Kerak: <xodimdan aniq nima talab qilinadi>\n\n" +
		"Boshqa hech narsa yozma. Suhbatda yo'q ma'lumotni o'ylab topma —\n" +
		"bilinmasa \"noma'lum\" deb yoz."

	if orderInfo != "" {
		transcript += "\n\n--- Tizimdan olingan buyurtma ma'lumoti ---" + orderInfo
	}

	out, err := c.generate(ctx, system, transcript)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DescribeImage mijoz yuborgan rasmni tahlil qiladi. Javob qat'iy formatda
// qaytadi, shuning uchun uni ParseImageAnswer bilan ajratish mumkin:
//
//	Raqamlar: DG60582375, 79019342930966
//	Tavsif: Buyurtma tafsilotlari skrinshoti.
func (c *Client) DescribeImage(ctx context.Context, mime string, data []byte) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bo'sh")
	}
	system := "Sen support agentiga mijoz yuborgan rasmni tushuntirasan.\n" +
		"Rasmni diqqat bilan ko'r va AYNAN quyidagi ikki qatorni yoz:\n" +
		"Raqamlar: <rasmda ko'rinadigan buyurtma raqami, track raqami yoki\n" +
		"shtrix-kod raqamlari, vergul bilan. Hech qanday raqam bo'lmasa: yo'q>\n" +
		"Tavsif: <rasmda nima borligi, 1-2 gap, o'zbek tilida>\n\n" +
		"Boshqa hech narsa yozma. Raqamlarni o'zgartirmasdan, aynan ko'chirib yoz.\n" +
		"Aniq ko'rinmayotgan raqamni taxmin qilma."

	return c.generateWithImage(ctx, system, "Bu mijoz support chatga yuborgan rasm.", mime, data)
}

// ParseImageAnswer DescribeImage javobidan raqamlar va tavsifni ajratadi.
// Model ba'zan markdown qo'shadi ("**Raqamlar:** ..."), shuning uchun
// kalit ham, qiymat ham `*` va `_` belgilaridan tozalanadi.
func ParseImageAnswer(out string) (numbers []string, description string) {
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.ToLower(strings.Trim(line[:i], "*_-# \t"))
		val := strings.Trim(line[i+1:], "*_ \t")

		switch key {
		case "raqamlar":
			if val == "" || strings.EqualFold(val, "yo'q") || strings.EqualFold(val, "yoq") {
				continue
			}
			for _, p := range strings.Split(val, ",") {
				if p = strings.Trim(p, "*_` \t"); p != "" {
					numbers = append(numbers, p)
				}
			}
		case "tavsif":
			description = val
		}
	}
	if description == "" {
		description = strings.TrimSpace(out)
	}
	return numbers, description
}

// Classify mijoz savoliga mos kategoriya id'sini tanlaydi.
// Mos kategoriya topilmasa 0 qaytaradi (xato emas).
func (c *Client) Classify(ctx context.Context, catalog, transcript string) (uint, error) {
	if catalog == "" {
		return 0, nil
	}
	system := "Sen matnni tasniflaysan. Quyida kategoriyalar ro'yxati bor.\n\n" +
		catalog +
		"\nMijozning oxirgi savoli qaysi kategoriyaga tegishli ekanini aniqla.\n" +
		"Javob sifatida FAQAT bitta raqam (kategoriya id) yoz. Hech qanday izoh yozma.\n" +
		"Agar hech qaysi kategoriyaga to'g'ri kelmasa 0 yoz."

	out, err := c.generate(ctx, system, transcript)
	if err != nil {
		return 0, err
	}
	m := regexp.MustCompile(`\d+`).FindString(out)
	if m == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(m, 10, 64)
	if err != nil {
		return 0, nil
	}
	return uint(id), nil
}

// generate — Gemini'ga bitta matnli so'rov (429/5xx da qayta urinish bilan).
func (c *Client) generate(ctx context.Context, systemPrompt, userText string) (string, error) {
	return c.send(ctx, systemPrompt, []part{{Text: userText}})
}

// generateWithImage — matn + rasm (vision) so'rovi.
func (c *Client) generateWithImage(ctx context.Context, systemPrompt, userText, mime string, data []byte) (string, error) {
	return c.send(ctx, systemPrompt, []part{
		{InlineData: &inlineData{MimeType: mime, Data: base64.StdEncoding.EncodeToString(data)}},
		{Text: userText},
	})
}

// send — Gemini'ga so'rov yuboradi (429/5xx da qayta urinish bilan).
func (c *Client) send(ctx context.Context, systemPrompt string, parts []part) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bo'sh")
	}

	reqBody := genRequest{
		Contents: []content{{Role: "user", Parts: parts}},
	}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &content{Parts: []part{{Text: systemPrompt}}}
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", apiBase, c.Model, c.APIKey)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second // 2s, 4s
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("gemini so'rov: %w", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Vaqtinchalik xatolar — qayta urinamiz.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("gemini xatosi (status %d): %s", resp.StatusCode, string(body))
			continue
		}

		var gr genResponse
		if err := json.Unmarshal(body, &gr); err != nil {
			return "", fmt.Errorf("gemini javobini o'qib bo'lmadi: %w\nXom: %s", err, string(body))
		}
		if gr.Error != nil {
			return "", fmt.Errorf("gemini xatosi: %s", gr.Error.Message)
		}
		if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("gemini javob qaytarmadi\nXom: %s", string(body))
		}
		return gr.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", lastErr
}
