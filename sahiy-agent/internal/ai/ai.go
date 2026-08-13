// Package ai — provayderdan mustaqil AI qatlami. Promptlar va javobni
// ajratish shu yerda; HTTP so'rovni esa Backend (gemini yoki openai) bajaradi.
package ai

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	Generate(ctx context.Context, system, user string) (string, Usage, error)
}

// Client — agent ishlatadigan yuqori darajali AI.
type Client struct {
	be     Backend
	Prompt string // tizim (system) prompt — prompt.txt yoki .env'dan
}

// New yangi client (be — tanlangan provayder).
func New(be Backend, systemPrompt string) *Client {
	return &Client{be: be, Prompt: systemPrompt}
}

// Name — joriy provayder nomi.
func (c *Client) Name() string { return c.be.Name() }

// generate — barcha so'rovlar shu yerdan o'tadi: javobni qaytaradi va
// sarflangan tokenlarni ctx'dagi Meter'ga qo'shadi (xato bo'lsa qo'shmaydi —
// muvaffaqiyatsiz so'rov uchun hisob kelmaydi).
func (c *Client) generate(ctx context.Context, system, user string) (string, error) {
	out, u, err := c.be.Generate(ctx, system, user)
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
	Transcript string // suhbatning yangi qismi (Window natijasi)
	Category   string // tanlangan kategoriya matni (bo'lmasligi mumkin)
	OrderInfo  string // track/client id bo'yicha topilgan buyurtma holati (xom JSON)
	HasImage   bool   // mijoz rasm yuborgan — agent uni ko'ra olmaydi
}

// Ask suhbat konteksti asosida agent javobini yozadi.
func (c *Client) Ask(ctx context.Context, req Request) (string, error) {
	// Bugungi sana — usiz model "12-15 kun ichida" kabi muddatlarni
	// buyurtma sanasidan hisoblay olmaydi.
	system := c.Prompt + "\n\nBugungi sana: " + time.Now().Format("2006-01-02")
	if req.Category != "" {
		system += "\n\n--- Shu savolga oid ma'lumot ---\n" + req.Category +
			"\n\nJavobingni faqat shu ma'lumotga tayanib yoz. Bu yerda yo'q narsani o'ylab topma."
	}
	if req.OrderInfo != "" {
		system += "\n\n--- Mijozning buyurtmasi (tizimdan olingan, real holat) ---" + req.OrderInfo +
			"\n\nBu JSON tizimdan olingan haqiqiy ma'lumot. Mijozga uni tushunarli\n" +
			"tilda tushuntir: buyurtma qayerda, holati nima, keyingi qadam nima.\n" +
			"JSON'ni o'zini ko'chirib yozma. Bu yerda yo'q maydonni o'ylab topma.\n" +
			"Agar ma'lumot savolga javob bermasa yoki ziddiyatli bo'lsa — #ESCALATE yoz."
	}
	if req.HasImage {
		system += "\n\n--- Muhim: suhbatda rasm bor ---\n" + imageRule(req.OrderInfo != "")
	}
	return c.generate(ctx, system, req.Transcript)
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
	system := "Sen support jamoasiga muammoni tushuntiruvchi yordamchisan.\n" +
		"Quyida mijoz bilan bo'lgan suhbat tarixi berilgan. Uni to'liq o'qib chiq va\n" +
		"navbatchi xodim uchun qisqa xulosa yoz — xodim suhbatni o'qimasdan ham\n" +
		"muammoni tushunishi kerak.\n\n" +
		"Aynan quyidagi 4 qatorni yoz (o'zbek tilida):\n" +
		"Daraja: <yuqori | o'rta | past>\n" +
		"Muammo: <mijozning umumiy muammosi>\n" +
		"Tafsilot: <muhim faktlar: buyurtma/track raqami, sana, nima urinib ko'rilgan>\n" +
		"Kerak: <xodimdan aniq nima talab qilinadi>\n\n" +
		"Daraja qanday tanlanadi:\n" +
		"- yuqori: yetkazish muddati o'tgan, buyurtma yo'qolgan yoki shikastlangan,\n" +
		"  pul qaytarish yoki to'lov nizosi, mijoz jahli chiqqan yoki bir necha\n" +
		"  marta javobsiz murojaat qilgan\n" +
		"- o'rta: holat noaniq, tekshirish kerak, lekin shoshilinch emas\n" +
		"- past: oddiy savol yoki ma'lumot yetishmayotgani uchun aniqlik kerak\n\n" +
		"Boshqa hech narsa yozma. Suhbatda yo'q ma'lumotni o'ylab topma —\n" +
		"bilinmasa \"noma'lum\" deb yoz."

	if orderInfo != "" {
		transcript += "\n\n--- Tizimdan olingan buyurtma ma'lumoti ---" + orderInfo
	}

	out, err := c.generate(ctx, system, transcript)
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
