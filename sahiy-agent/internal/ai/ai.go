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
	catPrefix       = "cat:"
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
// System prompt qat'iy TARTIBDA yig'iladi — o'zgarmas qism boshda turadi,
// bu provayderning prompt-keshi (KV-cache) ishlashi uchun muhim:
//
//  1. base        — har doim bir xil, eng katta bo'lak
//  2. bugungi sana — kuniga bir marta o'zgaradi
//  3. cat:<slug>  — kategoriyaga qarab
//  4. buyurtma JSON — har suhbatda boshqa
//  5. rasm qoidasi  — kamdan-kam
func (c *Client) Ask(ctx context.Context, req Request) (string, error) {
	var b strings.Builder
	b.WriteString(c.p.Get(PromptBase))
	b.WriteString("\n\nBugungi sana: " + time.Now().Format("2006-01-02"))

	if req.CategoryKey != "" {
		if cat := c.p.Get(catPrefix + req.CategoryKey); cat != "" {
			b.WriteString("\n\n--- Shu savolga oid ma'lumot ---\n" + cat +
				"\n\nJavobingni faqat shu ma'lumotga tayanib yoz. Bu yerda yo'q narsani o'ylab topma.")
		}
	}
	if req.OrderInfo != "" {
		b.WriteString("\n\n--- Mijozning buyurtmasi (tizimdan olingan, real holat) ---" + req.OrderInfo +
			"\n\nBu JSON tizimdan olingan haqiqiy ma'lumot. Mijozga uni tushunarli\n" +
			"tilda tushuntir: buyurtma qayerda, holati nima, keyingi qadam nima.\n" +
			"JSON'ni o'zini ko'chirib yozma. Bu yerda yo'q maydonni o'ylab topma.\n" +
			"Agar ma'lumot savolga javob bermasa yoki ziddiyatli bo'lsa — #ESCALATE yoz.")
	}
	if req.HasImage {
		b.WriteString("\n\n--- Muhim: suhbatda rasm bor ---\n" + imageRule(req.OrderInfo != ""))
	}

	// user qismi — mijoz matni o'zgarishsiz.
	return c.generate(ctx, b.String(), req.Transcript, GenOptions{})
}

// imageRule — mijoz rasm yuborganda beriladigan qoida. Agent rasmni ko'rmaydi,
// shuning uchun rasmdagi ma'lumotni taxmin qilmasdan buyurtma raqamini so'raydi.
func imageRule(haveOrder bool) string {
	base := "Mijoz suhbatda rasm (skrinshot) yubordi, transkriptda u \"[rasm]\" deb\n" +
		"ko'rsatilgan. Sen rasmni KO'RA OLMAYSAN. Rasmda nima borligini taxmin qilma\n" +
		"va \"rasmni ko'rdim\" deb yozma.\n"
	if haveOrder {
		return base +
			"Buyurtma ma'lumoti yuqorida bor — javobni o'shanga tayanib yoz. Agar rasm\n" +
			"boshqa buyurtmaga tegishli bo'lsa, mijozdan o'sha buyurtmaning raqamini\n" +
			"(yoki track raqamini) matn ko'rinishida yozib yuborishini so'ra."
	}
	return base +
		"Mijozdan buyurtma raqamini yoki track raqamini MATN ko'rinishida yozib\n" +
		"yuborishini xushmuomala so'ra — shunda holatni tizimdan tekshirib bera olasan.\n" +
		"Masalan: \"Rasmni ko'ra olmayapman. Iltimos, buyurtma raqamingizni yoki track\n" +
		"raqamingizni yozib yuboring — darhol tekshirib beraman.\"\n" +
		"Raqam so'rashdan boshqa hech narsani o'ylab topma."
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
	system := c.p.Get(PromptSummarize)

	if orderInfo != "" {
		transcript += "\n\n--- Tizimdan olingan buyurtma ma'lumoti ---" + orderInfo
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

// Route — routerning qarori. Faqat shu ikki maydon o'qiladi: router mijoz
// matnini qayta yozsa ham, u e'tiborga olinmaydi.
type Route struct {
	Category string `json:"category"`
	Escalate bool   `json:"escalate"`
}

// routerOptions — router javobi qisqa va deterministik bo'lishi kerak.
var routerOptions = GenOptions{MaxTokens: 20, TempZero: true, JSON: true}

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
	system := strings.ReplaceAll(tmpl, "{{CATEGORIES}}", strings.TrimRight(list.String(), "\n"))

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
