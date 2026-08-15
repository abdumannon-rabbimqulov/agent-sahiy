// Package ai — provayderdan mustaqil AI qatlami. Promptlar va javobni
// ajratish shu yerda; HTTP so'rovni esa Backend (gemini yoki openai) bajaradi.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Prompt kalitlari (models paketidagilar bilan bir xil — ai paketi models'ga
// bog'lanmasligi uchun shu yerda ham e'lon qilingan).
const (
	PromptBase      = "base"
	PromptClassify  = "classify"
	PromptSummarize = "summarize"
	BlockCategory   = "block:category"
	BlockOrder      = "block:order"
	BlockImage      = "block:image"
	BlockImageOrder = "block:image_order"
	catPrefix       = "cat:"
)

// Placeholder'lar — bazadagi prompt matni ichida shu belgilar bo'lsa,
// o'rniga tegishli ma'lumot qo'yiladi. Belgi bo'lmasa ma'lumot prompt
// oxiriga qo'shiladi (prompt matnini yozgan odam joyini o'zi tanlaydi).
const (
	phDate     = "{{DATE}}"
	phCategory = "{{CATEGORY}}"
	phOrders   = "{{ORDERS}}"
)

// Backend — bitta LLM provayderi (gemini, openai, ...).
type Backend interface {
	// Name — loglarda ko'rinadigan nom ("gemini gemini-2.5-flash-lite").
	Name() string
	// Ready — kalit bor va so'rov yuborish mumkin.
	Ready() bool
	// Generate — matnli so'rov. Javob bilan birga sarflangan tokenlarni
	// (provayder qaytargan aniq sonlarni) beradi.
	//
	// Kelishuv: javob ham, xato ham qaytsa — bu "qisman muvaffaqiyat"
	// (masalan lokal modelda kontekst to'lib, prompt kesilgan). Javob
	// ishlatiladi, xato esa ogohlantirish sifatida qayd etiladi.
	Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error)
}

// Prompts — promptlar manbai (prompts.Store shuni qondiradi). Promptlar
// bazada yotadi va dashboarddan tahrirlanadi, shuning uchun ular HAR
// CHAQIRUVDA yangidan o'qiladi — agentni qayta ishga tushirish shart emas.
type Prompts interface {
	// Get — kalit bo'yicha prompt matni (topilmasa bo'sh satr).
	Get(key string) string
	// Keys — prefiks bilan boshlanadigan kalitlar (masalan "cat:").
	Keys(prefix string) []string
}

// Client — agent ishlatadigan yuqori darajali AI.
type Client struct {
	be Backend
	p  Prompts
}

// New yangi client (be — tanlangan provayder, p — promptlar manbai).
func New(be Backend, p Prompts) *Client {
	return &Client{be: be, p: p}
}

// Name — joriy provayder nomi.
func (c *Client) Name() string { return c.be.Name() }

// generate — barcha so'rovlar shu yerdan o'tadi: javobni qaytaradi va
// sarflangan tokenlarni ctx'dagi Meter'ga qo'shadi (xato bo'lsa qo'shmaydi —
// muvaffaqiyatsiz so'rov uchun hisob kelmaydi).
func (c *Client) generate(ctx context.Context, system, user string, opt GenOptions) (string, error) {
	out, u, err := c.be.Generate(ctx, system, user, opt)
	// Javob bor, lekin xato ham bor — qisman muvaffaqiyat: javobni
	// ishlatamiz, xatoni ogohlantirish qilib yozamiz.
	if err != nil && out != "" {
		m := meterFrom(ctx)
		m.Add(u)
		m.Warn(err.Error())
		return out, nil
	}
	if err != nil {
		return "", err
	}
	meterFrom(ctx).Add(u)
	return out, nil
}

// Ready — provayder ishlashga tayyor (API kaliti bor).
func (c *Client) Ready() bool { return c.be != nil && c.be.Ready() }

// Request — javob yozish uchun yig'ilgan kontekst.
type Request struct {
	// Transcript — mijozning ORIGINAL matni (suhbat oynasi). Hech qanday
	// xulosa yoki qayta yozish yo'q: buyurtma raqami mijoz yozgan holida
	// modelga yetib boradi.
	Transcript string
	// CategoryKey — router tanlagan kategoriya slug'i ("yetkazib-berish").
	CategoryKey string
	OrderInfo   string // track/client id bo'yicha topilgan buyurtma holati (xom JSON)
	HasImage    bool   // mijoz rasm yuborgan — agent uni ko'ra olmaydi
}

// Ask suhbat konteksti asosida agent javobini yozadi.
//
// MUHIM: bu yerda birorta prompt matni yo'q — hammasi Postgres'dan
// (dashboard /prompts) olinadi. Kod faqat bloklarni TARTIB bilan yig'adi;
// tartib o'zgarmas, chunki provayderning prompt-keshi (KV-cache) shunga
// tayanadi:
//
//  1. base            — har doim bir xil, eng katta bo'lak
//  2. block:category  — kategoriyaga qarab
//  3. block:order     — tizimdan olingan buyurtma (har suhbatda boshqa)
//  4. block:image     — kamdan-kam
//
// Blok prompti bazada bo'lmasa — o'sha blok umuman qo'shilmaydi.
func (c *Client) Ask(ctx context.Context, req Request) (string, error) {
	var b strings.Builder
	b.WriteString(render(c.p.Get(PromptBase)))

	if req.CategoryKey != "" {
		if cat := c.p.Get(catPrefix + req.CategoryKey); cat != "" {
			b.WriteString(block(render(c.p.Get(BlockCategory)), phCategory, cat))
		}
	}
	if req.OrderInfo != "" {
		b.WriteString(block(render(c.p.Get(BlockOrder)), phOrders, req.OrderInfo))
	}
	if req.HasImage {
		key := BlockImage
		if req.OrderInfo != "" {
			key = BlockImageOrder
		}
		b.WriteString(block(render(c.p.Get(key)), "", ""))
	}

	// user qismi — mijoz matni o'zgarishsiz.
	return c.generate(ctx, b.String(), req.Transcript, GenOptions{})
}

// render — har qanday prompt matnidagi umumiy placeholder'larni to'ldiradi.
func render(tmpl string) string {
	return strings.ReplaceAll(tmpl, phDate, time.Now().Format("2006-01-02"))
}

// block — bitta blokni system promptga qo'shiladigan ko'rinishga keltiradi.
// tmpl bo'sh bo'lsa (bazada bunday prompt yo'q) — bo'sh satr qaytadi.
// tmpl ichida placeholder bo'lsa ma'lumot o'sha joyga, bo'lmasa oxiriga
// qo'yiladi.
func block(tmpl, placeholder, data string) string {
	if tmpl == "" {
		return ""
	}
	switch {
	case data == "":
	case strings.Contains(tmpl, placeholder):
		tmpl = strings.ReplaceAll(tmpl, placeholder, data)
	default:
		tmpl += "\n" + data
	}
	return "\n\n" + tmpl
}

// Daraja — muammoning shoshilinchlik darajasi.
type Daraja string

const (
	Yuqori Daraja = "yuqori"
	Orta   Daraja = "o'rta"
	Past   Daraja = "past"
)

// Belgi — guruh xabarida ko'rinadigan rangli belgi.
func (d Daraja) Belgi() string {
	switch d {
	case Yuqori:
		return "🔴"
	case Past:
		return "🟢"
	default:
		return "🟡"
	}
}

// Sarlavha — xabar boshidagi matn.
func (d Daraja) Sarlavha() string {
	switch d {
	case Yuqori:
		return "YUQORI"
	case Past:
		return "PAST"
	default:
		return "O'RTA"
	}
}

// Summarize suhbatni xodimlar guruhi uchun qisqa xulosaga aylantiradi va
// muammoning shoshilinchlik darajasini aniqlaydi.
func (c *Client) Summarize(ctx context.Context, transcript, orderInfo string) (Daraja, string, error) {
	system := render(c.p.Get(PromptSummarize))
	if orderInfo != "" {
		system += block(render(c.p.Get(BlockOrder)), phOrders, orderInfo)
	}

	out, err := c.generate(ctx, system, transcript, GenOptions{})
	if err != nil {
		return Orta, "", err
	}
	daraja, body := splitDaraja(out)
	return daraja, body, nil
}

// splitDaraja javobdan "Daraja:" qatorini ajratib oladi; qolgan matn
// xodimga ko'rsatiladigan xulosa bo'lib qoladi.
func splitDaraja(out string) (Daraja, string) {
	daraja := Orta
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, ":")
		if i > 0 && strings.ToLower(strings.Trim(line[:i], "*_-# \t")) == "daraja" {
			val := strings.ToLower(strings.Trim(line[i+1:], "*_ \t."))
			switch {
			case strings.Contains(val, "yuqori"):
				daraja = Yuqori
			case strings.Contains(val, "past"):
				daraja = Past
			}
			continue // bu qator xulosaga kirmaydi
		}
		kept = append(kept, line)
	}
	return daraja, strings.TrimSpace(strings.Join(kept, "\n"))
}

// Route — routerning qarori. Faqat shu uch maydon o'qiladi: router mijoz
// matnini qayta yozsa ham, u e'tiborga olinmaydi.
type Route struct {
	Category string `json:"category"`
	Escalate bool   `json:"escalate"`
	// Order — mijoz o'z buyurtmasi haqida so'rayaptimi. true bo'lsa agent
	// Dashboard API'ga GET so'rov yuborib, buyurtma holatini oladi.
	// Router bu maydonni qaytarmasa false bo'ladi (bunda so'rov faqat
	// xabarda aniq buyurtma/track raqami bo'lsa yuboriladi).
	Order bool `json:"order"`
}

// routerOptions — router javobi qisqa va deterministik bo'lishi kerak.
var routerOptions = GenOptions{MaxTokens: 40, TempZero: true, JSON: true}

// Classify mijoz murojaatini kategoriyaga ajratadi.
//
// Kategoriyalar ro'yxati bazadagi "cat:" promptlaridan DINAMIK yig'iladi —
// dashboarddan yangi kategoriya qo'shilsa router uni darhol ko'radi.
// Kategoriya bo'lmasa router umuman chaqirilmaydi (bekorga token ketmasin).
func (c *Client) Classify(ctx context.Context, transcript string) (Route, error) {
	keys := c.p.Keys(catPrefix)
	if len(keys) == 0 {
		return Route{}, nil
	}
	tmpl := c.p.Get(PromptClassify)
	if tmpl == "" {
		return Route{}, nil
	}

	var list strings.Builder
	valid := make(map[string]bool, len(keys))
	for _, k := range keys {
		slug := strings.TrimPrefix(k, catPrefix)
		valid[slug] = true
		fmt.Fprintf(&list, "- %s\n", slug)
	}
	system := render(strings.ReplaceAll(tmpl, "{{CATEGORIES}}", strings.TrimRight(list.String(), "\n")))

	out, err := c.generate(ctx, system, transcript, routerOptions)
	if err != nil {
		return Route{}, err
	}

	r := parseRoute(out)
	// Router o'ylab topgan kategoriya qabul qilinmaydi — faqat ro'yxatdagisi.
	if r.Category != "" && !valid[r.Category] {
		r.Category = ""
	}
	return r, nil
}

// parseRoute router javobidan JSON'ni ajratadi. Model JSON atrofiga matn
// yoki ```json bloki qo'shsa ham ishlaydi.
func parseRoute(out string) Route {
	var r Route
	s := strings.TrimSpace(out)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return Route{}
	}
	r.Category = strings.TrimSpace(strings.ToLower(r.Category))
	return r
}
