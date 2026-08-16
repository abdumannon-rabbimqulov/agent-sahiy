// Package ai — provayderdan mustaqil AI qatlami. Promptlar va javobni
// ajratish shu yerda; HTTP so'rovni esa Backend (gemini yoki openai) bajaradi.
package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	PromptBase      = "base"
	BlockCategory   = "block:category"
	BlockOrder      = "block:order"
	BlockImage      = "block:image"
	BlockImageOrder = "block:image_order"
	catPrefix       = "cat:"
)

// RequiredKeys — bazada bo'lishi SHART bo'lgan promptlar. Bittasi yo'q bo'lsa
// agent ishga tushmaydi: kodda zaxira matn yo'q, hammasi Postgres'da.
var RequiredKeys = []string{PromptBase}

// OptionalKeys — bo'lmasa tegishli blok system promptga qo'shilmaydi,
// agent esa ishlayveradi.
var OptionalKeys = []string{BlockCategory, BlockOrder, BlockImage, BlockImageOrder}

// Placeholder'lar — bazadagi prompt matni ichida shu belgilar bo'lsa,
// o'rniga tegishli ma'lumot qo'yiladi. Belgi bo'lmasa ma'lumot prompt
// oxiriga qo'shiladi (prompt matnini yozgan odam joyini o'zi tanlaydi).
const (
	phDate     = "{{DATE}}"
	phCategory = "{{CATEGORY}}"
	phOrders   = "{{ORDERS}}"
)

// orderHeader — "block:order" prompti bazada bo'lmaganda ishlatiladigan
// minimal sarlavha. Tizimdan olingan ma'lumot promptga yetib bormasa model
// uni o'zidan to'qiydi, shuning uchun promptsiz ham qo'shiladi. Prompt
// yozilgan bo'lsa bu matn umuman ishlatilmaydi.
const orderHeader = "Tizimdagi buyurtma ma'lumoti (faqat shunga tayan, o'zingdan to'qima):"

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

// Prompts — promptlar manbai (prompts.Service shuni qondiradi). Promptlar
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

// Request — qaror qabul qilish uchun yig'ilgan kontekst.
type Request struct {
	// Transcript — mijozning ORIGINAL matni (suhbat oynasi). Hech qanday
	// xulosa yoki qayta yozish yo'q: buyurtma raqami mijoz yozgan holida
	// modelga yetib boradi.
	Transcript string
	// CategoryKey — kategoriya slug'i ("yetkazib-berish"). Bo'sh bo'lsa
	// kategoriya bilimi promptga qo'shilmaydi.
	CategoryKey string
	OrderInfo   string // track/client id bo'yicha topilgan buyurtma holati (xom JSON)
	HasImage    bool   // mijoz rasm yuborgan — agent uni ko'ra olmaydi
}

// Decide suhbat konteksti asosida agent qarorini oladi.
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
//
// Model qaytargan JSON Decision'ga aylantiriladi; mijozga hech narsa
// yuborilmaydi — harakatni chaqiruvchi (main.go) tanlaydi.
func (c *Client) Decide(ctx context.Context, req Request) (Decision, error) {
	// user qismi — mijoz matni o'zgarishsiz.
	out, err := c.generate(ctx, c.buildSystem(req), req.Transcript, decideOptions)
	if err != nil {
		return Decision{}, err
	}
	return ParseDecision(out)
}

// decideOptions — qaror qisqa va deterministik JSON bo'lishi kerak.
var decideOptions = GenOptions{MaxTokens: 200, TempZero: true, JSON: true}

// buildSystem — system promptni bloklardan yig'adi (tartib yuqorida
// tushuntirilgan; o'zgartirilmaydi).
func (c *Client) buildSystem(req Request) string {
	var b strings.Builder
	b.WriteString(render(c.p.Get(PromptBase)))

	if req.CategoryKey != "" {
		if cat := c.p.Get(catPrefix + req.CategoryKey); cat != "" {
			b.WriteString(block(render(c.p.Get(BlockCategory)), phCategory, cat))
		}
	}
	if req.OrderInfo != "" {
		b.WriteString(blockData(render(c.p.Get(BlockOrder)), phOrders, orderHeader, withToday(req.OrderInfo)))
	}
	if req.HasImage {
		key := BlockImage
		if req.OrderInfo != "" {
			key = BlockImageOrder
		}
		b.WriteString(block(render(c.p.Get(key)), "", ""))
	}
	return b.String()
}

// Answer — mijozga yuboriladigan MATNLI javob yozadi.
//
// Decide'dan farqi: system prompt "base" emas (u endi JSON qaror qaytaradi),
// balki req.CategoryKey ko'rsatgan kategoriya bilimi ("cat:<slug>"). Ya'ni
// javob faqat o'sha bo'lim bilimiga tayanadi.
//
// Kategoriya prompti bazada bo'lmasa xato qaytadi — kod ichida zaxira matn
// yo'q, hammasi Postgres'da.
func (c *Client) Answer(ctx context.Context, req Request) (string, error) {
	system, err := c.buildCategorySystem(req)
	if err != nil {
		return "", err
	}
	// user qismi — mijoz matni o'zgarishsiz.
	return c.generate(ctx, system, req.Transcript, GenOptions{})
}

// orderOptions — "cat:order" javobi JSON bo'lishi kerak, lekin ichida
// mijozga yoziladigan to'liq matn bor — shuning uchun Decide'dan uzunroq.
var orderOptions = GenOptions{MaxTokens: 600, TempZero: true, JSON: true}

// OrderAnswer — buyurtmadagi muammo bo'yicha "cat:order" bilimi asosida
// qaror: mijozga nima yozish va xodimlarga nima yetkazish (OrderReply).
// Answer'dan farqi faqat shu — javob matn emas, JSON.
func (c *Client) OrderAnswer(ctx context.Context, req Request) (OrderReply, error) {
	system, err := c.buildCategorySystem(req)
	if err != nil {
		return OrderReply{}, err
	}
	out, err := c.generate(ctx, system, req.Transcript, orderOptions)
	if err != nil {
		return OrderReply{}, err
	}
	// Parse xatosida ham javob qaytadi (Raw to'ldirilgan bo'ladi): promt
	// JSON qaytarishga sozlanmagan bo'lsa, chaqiruvchi matnni yo'qotmasin.
	return ParseOrderReply(out)
}

// buildCategorySystem — kategoriya bilimidan ("cat:<slug>") system prompt
// yig'adi. Kategoriya prompti bazada bo'lmasa xato qaytadi — kod ichida
// zaxira matn yo'q, hammasi Postgres'da.
func (c *Client) buildCategorySystem(req Request) (string, error) {
	// Avval "cat:<slug>" (kategoriyalardan ko'chirilgan bilim), topilmasa
	// shu nomdagi oddiy prompt ("order" kabi mustaqil yozilganlar uchun).
	cat := c.p.Get(catPrefix + req.CategoryKey)
	if strings.TrimSpace(cat) == "" {
		cat = c.p.Get(req.CategoryKey)
	}
	if strings.TrimSpace(cat) == "" {
		return "", fmt.Errorf("kategoriya prompti topilmadi: %s%s (yoki %s)",
			catPrefix, req.CategoryKey, req.CategoryKey)
	}

	var b strings.Builder
	// block:category bo'lsa — bilim o'sha ko'rsatma ichiga joylashtiriladi;
	// bo'lmasa kategoriya matni o'zi system prompt bo'ladi.
	if tmpl := c.p.Get(BlockCategory); tmpl != "" {
		b.WriteString(strings.TrimPrefix(block(render(tmpl), phCategory, render(cat)), "\n\n"))
	} else {
		b.WriteString(render(cat))
	}
	if req.OrderInfo != "" {
		b.WriteString(blockData(render(c.p.Get(BlockOrder)), phOrders, orderHeader, withToday(req.OrderInfo)))
	}
	if req.HasImage {
		key := BlockImage
		if req.OrderInfo != "" {
			key = BlockImageOrder
		}
		b.WriteString(block(render(c.p.Get(key)), "", ""))
	}
	return b.String(), nil
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

// withToday — buyurtma ma'lumoti oldiga bugungi sanani qo'yadi. Model
// bugun qaysi kun ekanini bilmaydi, "oxirgi 3 kunda keldimi", "5 kundan
// oshdimi" kabi qoidalar esa aynan shunga tayanadi.
func withToday(data string) string {
	if data == "" {
		return ""
	}
	return "Bugungi sana: " + time.Now().Format("2006-01-02") + "\n\n" + data
}

// blockData — block() bilan bir xil, lekin shablon bo'lmasa ham MA'LUMOTNI
// tashlab yubormaydi: qisqa sarlavha bilan qo'shadi.
func blockData(tmpl, placeholder, header, data string) string {
	if data == "" {
		return ""
	}
	if tmpl == "" {
		return "\n\n" + header + "\n" + data
	}
	return block(tmpl, placeholder, data)
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
