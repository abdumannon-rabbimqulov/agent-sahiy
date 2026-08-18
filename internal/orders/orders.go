// Package orders — mijoz xabaridagi id yoki track raqami bo'yicha buyurtma
// holatini ikkinchi saytdan (api.sahiy.uz admin delivery API) qidiradi.
//
// Endpoint: GET /api/v2/admin/delivery/orders/filter
// Muhim: `delivered` filtri MAJBURIY — usiz server doim "NO ORDERS FOUND"
// qaytaradi. Shuning uchun har qidiruvda delivered=true va delivered=false
// alohida so'raladi va natijalar birlashtiriladi.
package orders

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"sahiy-agent/internal/service"
)

// filterPath — buyurtma qidiruv endpointi.
const filterPath = "/api/v2/admin/delivery/orders/filter"

// maxTracks — bitta xabardan qidiriladigan track raqamlari soni chegarasi.
const maxTracks = 3

// pageSize — bitta so'rovda olinadigan buyurtmalar soni.
const pageSize = 20

// maxShown — Gemini kontekstiga qo'shiladigan buyurtmalar soni.
const maxShown = 5

// Order — javobdagi bitta buyurtma (faqat kerakli maydonlar).
type Order struct {
	ID            int64   `json:"id"`
	OrderID       int64   `json:"order_id"`
	UserID        int64   `json:"user_id"`
	FullName      string  `json:"full_name"`
	Phone         string  `json:"phone"`
	City          string  `json:"city"`
	ExpressNum    string  `json:"express_num"` // track raqami
	Status        int     `json:"status"`
	Delivered     bool    `json:"delivered"`
	PaymentStatus bool    `json:"payment_status"`
	PaymentFee    float64 `json:"payment_fee"`
	Weight        float64 `json:"weight"` // gramm
	CreatedAt     string  `json:"created_at"`
	PaidAt        string  `json:"paid_at"`
	DeliveredAt   string  `json:"delivered_at"`
	Comment       string  `json:"comment"`
	Station       *struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"station"`
	DeliveryAddress *struct {
		BranchName    string `json:"branch_name"`
		BranchAddress string `json:"branch_address"`
	} `json:"delivery_address"`
}

// response — server javobi. Diqqat: buyurtma topilmasa "data" obyekt emas,
// bo'sh massiv ([]) bo'lib keladi — shuning uchun RawMessage orqali o'qiladi.
type response struct {
	Ret  int             `json:"ret"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type dataBlock struct {
	Orders     []Order `json:"orders"`
	TotalItems int     `json:"total_items"`
}

// Lookup buyurtma qidiruvchi.
type Lookup struct {
	svc *service.Client
}

// New yangi Lookup.
func New(svc *service.Client) *Lookup { return &Lookup{svc: svc} }

// Enabled — service API login ma'lumotlari bormi.
func (l *Lookup) Enabled() bool { return l != nil && l.svc.Enabled() }

// trackRe — track raqami: 8+ xonali son yoki harf bilan boshlanuvchi kod
// (masalan 79017498359954, YT7594703873671, LP00123456789CN).
var trackRe = regexp.MustCompile(`\b[A-Z]{2}[A-Z0-9]{6,}\b|\b\d{8,}\b`)

// digitRe — kodda kamida bitta raqam bormi (oddiy so'zlarni chetlash uchun:
// matn UPPERCASE ga o'girilgani sabab "BUYURTMAM" ham regexga tushadi).
var digitRe = regexp.MustCompile(`\d`)

// Tracks matndan track raqamiga o'xshash bo'laklarni ajratadi
// (takrorlanmaydi, maxTracks tagacha).
func Tracks(text string) []string {
	found := trackRe.FindAllString(strings.ToUpper(text), -1)
	seen := map[string]bool{}
	var out []string
	for _, f := range found {
		if seen[f] || !digitRe.MatchString(f) || isPhone(f) {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= maxTracks {
			break
		}
	}
	return out
}

// isPhone — O'zbekiston telefon raqamiga o'xshash son (998XXXXXXXXX yoki
// 9 xonali mahalliy). Bunday raqamlar track emas, keraksiz so'rov qilmaymiz.
func isPhone(s string) bool {
	if !digitOnly(s) {
		return false
	}
	return (len(s) == 12 && strings.HasPrefix(s, "998")) ||
		(len(s) == 13 && strings.HasPrefix(s, "9998"))
}

func digitOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// FilterPath — bitta so'rovning to'liq yo'li (so'rov satri bilan).
//
// Alohida chiqarilgan, chunki sahiy/dashboard dasturi "qanday so'rov
// ketyapti" degan savolga javob beradi — u ham AYNAN shu funksiyani
// chaqiradi, aks holda ikkita nusxa vaqt o'tib bir-biridan uzoqlashardi.
//
// TrackParam / UserParam — qidiruv maydonlari nomi.
const (
	TrackParam = "track_number"
	UserParam  = "user_id"
)

func FilterPath(param, value string, delivered bool) string {
	q := url.Values{}
	q.Set("page", "1")
	q.Set("size", fmt.Sprint(pageSize))
	q.Set(param, value)
	q.Set("delivered", fmt.Sprint(delivered))
	return filterPath + "?" + q.Encode()
}

// fetch bitta so'rov: berilgan filtr va delivered qiymati bilan.
func (l *Lookup) fetch(param, value string, delivered bool) ([]Order, error) {
	body, status, err := l.svc.Get(FilterPath(param, value, delivered))
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("qidiruv muvaffaqiyatsiz (status %d)", status)
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}
	// "NO ORDERS FOUND" holatida data = [] — bu xato emas, shunchaki bo'sh.
	var d dataBlock
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return nil, nil
	}
	return d.Orders, nil
}

// find bitta filtr bo'yicha yetkazilgan va yetkazilmagan buyurtmalarni
// birlashtirib qaytaradi (delivered majburiy filtr bo'lgani uchun).
func (l *Lookup) find(param, value string) ([]Order, error) {
	var all []Order
	var firstErr error
	for _, delivered := range []bool{false, true} {
		got, err := l.fetch(param, value, delivered)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		all = append(all, got...)
	}
	if len(all) == 0 {
		return nil, firstErr
	}
	// Eng yangi buyurtmalar birinchi — mijoz odatda oxirgisini so'raydi.
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt > all[j].CreatedAt })
	return all, nil
}

// ByTrack track raqami bo'yicha qidiradi.
func (l *Lookup) ByTrack(track string) ([]Order, error) {
	return l.find(TrackParam, track)
}

// ByUser mijoz id'si bo'yicha qidiradi (eng yangilari birinchi emas —
// server tartibida keladi).
func (l *Lookup) ByUser(userID int64) ([]Order, error) {
	return l.find(UserParam, fmt.Sprint(userID))
}

// Summary buyurtmalarni odam (va AI) o'qiydigan qisqa matnga aylantiradi.
// Xom JSON emas — chunki javobda kerak bo'lmagan yuzlab maydon bor.
func Summary(list []Order) string {
	if len(list) == 0 {
		return ""
	}
	shown := list
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}

	var b strings.Builder
	for _, o := range shown {
		fmt.Fprintf(&b, "\n• Buyurtma #%d (yetkazma id %d)\n", o.OrderID, o.ID)
		if o.ExpressNum != "" {
			fmt.Fprintf(&b, "  Track: %s\n", o.ExpressNum)
		}
		fmt.Fprintf(&b, "  Holat: %s\n", statusText(o))
		if o.Weight > 0 {
			fmt.Fprintf(&b, "  Og'irlik: %.2f kg\n", o.Weight/1000)
		}
		if br := branch(o); br != "" {
			fmt.Fprintf(&b, "  Filial: %s\n", br)
		}
		if o.City != "" {
			fmt.Fprintf(&b, "  Shahar: %s\n", o.City)
		}
		fmt.Fprintf(&b, "  To'lov: %s\n", paymentText(o))
		if d := date(o.CreatedAt); d != "" {
			fmt.Fprintf(&b, "  Yaratilgan: %s\n", d)
		}
		if d := date(o.DeliveredAt); d != "" {
			fmt.Fprintf(&b, "  Topshirilgan: %s\n", d)
		}
		if o.Comment != "" {
			fmt.Fprintf(&b, "  Izoh: %s\n", o.Comment)
		}
	}
	if len(list) > len(shown) {
		fmt.Fprintf(&b, "\n(jami %d ta buyurtma, birinchi %d tasi ko'rsatildi)\n", len(list), len(shown))
	}
	return b.String()
}

// statusText holatni tushunarli qilib yozadi.
// Diqqat: `status` raqamining to'liq lug'ati bizda yo'q, shuning uchun
// asosiy xulosa `delivered` bayrog'idan olinadi, raqam qavsda beriladi.
func statusText(o Order) string {
	if o.Delivered {
		if d := date(o.DeliveredAt); d != "" {
			return fmt.Sprintf("mijozga topshirilgan (%s, status %d)", d, o.Status)
		}
		return fmt.Sprintf("mijozga topshirilgan (status %d)", o.Status)
	}
	return fmt.Sprintf("hali topshirilmagan — yo'lda yoki filialda (status %d)", o.Status)
}

func paymentText(o Order) string {
	if o.PaymentStatus {
		return "to'langan"
	}
	if o.PaymentFee > 0 {
		return fmt.Sprintf("to'lanmagan (%.0f)", o.PaymentFee)
	}
	return "to'lanmagan"
}

func branch(o Order) string {
	if o.Station != nil && o.Station.Name != "" {
		return o.Station.Name
	}
	if o.DeliveryAddress != nil && o.DeliveryAddress.BranchName != "" {
		return o.DeliveryAddress.BranchName
	}
	return ""
}

// date "2026-08-01T05:48:46.000000Z" yoki "2026-08-05 16:41:12" dan
// "2026-08-01" ni ajratadi.
func date(s string) string {
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "T "); i > 0 {
		return s[:i]
	}
	return s
}
