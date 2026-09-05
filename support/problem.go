// Kelmagan buyurtmalarni topish: adminkadagi TO'LANGAN buyurtmalar
// yetkazmadagi KELGAN posilkalar bilan trek raqami orqali solishtiriladi
// — mos kelmagani muammoli hisoblanadi.
package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Muammoli buyurtma sabablari.
const (
	ReasonNoTrack       = "trek_yoq"        // to'langan, lekin Xitoyda hali trek berilmagan
	ReasonNotInDelivery = "dashboardda_yoq" // trek bor, lekin yetkazmaga tushmagan
)

// Sahifalash standartlari — solishtirish uchun butun ro'yxat kerak, shuning
// uchun /api/orders dagi size=10 dan kattaroq olinadi.
const (
	DefaultProblemSize     = 50
	DefaultProblemMaxPages = 20
)

// ErrAdminkaUnauthorized — adminka tokeni rad etildi. ErrUnauthorized dan
// alohida: adminka tokeni avtomatik yangilanmaydi (.env dan olinadi), shuning
// uchun chaqiruvchi qayta urinmasligi kerak.
var ErrAdminkaUnauthorized = errors.New("adminka tokeni rad etildi (401) — .env dagi ADMINKA_TOKEN_BEARER ni yangilang")

// ProblemFilter — qidiruv sharti: uchtasidan kamida bittasi to'ldiriladi.
type ProblemFilter struct {
	UserID     int64  `json:"user_id"`
	OrderSN    string `json:"order_sn"`
	ExpressNum string `json:"express_num"` // trek raqami
	Size       int    `json:"size"`        // bir sahifadagi soni
	MaxPages   int    `json:"max_pages"`   // cheksiz sikldan himoya
}

// ProblemOrder — adminkadagi buyurtma + nega muammoli ekani.
type ProblemOrder struct {
	AdminkaOrder
	Reason string `json:"reason"`
}

// ProblemResult — solishtirish natijasi.
type ProblemResult struct {
	UserID         int64 `json:"user_id"`
	AdminkaCount   int   `json:"adminka_count"`   // to'langan buyurtmalar
	DashboardCount int   `json:"dashboard_count"` // yetkazmaga tushganlari
	MissingCount   int   `json:"missing_count"`
	// Truncated — ro'yxat max_pages ga tirab qolgan: solishtirish to'liq emas,
	// `missing` ichida soxta "dashboardda_yoq" bo'lishi mumkin.
	Truncated bool           `json:"truncated"`
	Warning   string         `json:"warning,omitempty"`
	Missing   []ProblemOrder `json:"missing"`
}

// FindProblemOrders adminkadagi (to'langan) buyurtmalarni dashboarddagi
// (kelgan) yetkazmalar bilan trek raqami orqali solishtiradi va dashboardda
// uchramaganlarini qaytaradi.
func FindProblemOrders(a Adminka, s Service, token string, f ProblemFilter) (ProblemResult, error) {
	if f.UserID <= 0 && f.OrderSN == "" && f.ExpressNum == "" {
		return ProblemResult{}, fmt.Errorf("user_id, order_sn yoki express_num berilmagan")
	}
	if f.Size < 1 {
		f.Size = DefaultProblemSize
	}
	if f.MaxPages < 1 {
		f.MaxPages = DefaultProblemMaxPages
	}

	orders, ordersCut, err := fetchAllOrders(a, f)
	if err != nil {
		return ProblemResult{}, err
	}

	arrived, tracksCut, err := fetchArrivedTracks(s, token, f, orders)
	if err != nil {
		return ProblemResult{}, err
	}

	res := ProblemResult{
		UserID:         f.UserID,
		AdminkaCount:   len(orders),
		DashboardCount: len(arrived),
		Missing:        []ProblemOrder{},
	}
	for _, o := range orders {
		switch {
		case trackKey(o.ExpressNum) == "":
			res.Missing = append(res.Missing, ProblemOrder{o, ReasonNoTrack})
		case !arrived[trackKey(o.ExpressNum)]:
			res.Missing = append(res.Missing, ProblemOrder{o, ReasonNotInDelivery})
		}
		if res.UserID == 0 {
			res.UserID = o.UserID
		}
	}
	// Yangi to'lovlar yuqorida tursin.
	sort.SliceStable(res.Missing, func(i, j int) bool {
		return res.Missing[i].CreatedAt > res.Missing[j].CreatedAt
	})
	res.MissingCount = len(res.Missing)
	res.Truncated = ordersCut || tracksCut
	if res.Truncated {
		res.Warning = fmt.Sprintf(
			"ro'yxat max_pages=%d chegarasiga tirab qoldi (%s) — natija to'liq emas, max_pages yoki size ni oshiring",
			f.MaxPages, cutSource(ordersCut, tracksCut))
	}
	return res, nil
}

// cutSource qaysi tomon qirqilganini matn qilib beradi.
func cutSource(orders, tracks bool) string {
	switch {
	case orders && tracks:
		return "adminka va dashboard"
	case orders:
		return "adminka"
	default:
		return "dashboard"
	}
}

// fetchAllOrders adminkadan hamma sahifani yig'adi: to'liq sahifa kelsa
// keyingisi so'raladi, kam qator kelsa oxiri. Ikkinchi qiymat — ro'yxat
// max_pages ga tirab qolgani (ya'ni yana sahifa bo'lishi mumkin).
func fetchAllOrders(a Adminka, f ProblemFilter) ([]AdminkaOrder, bool, error) {
	var all []AdminkaOrder
	truncated := false
	for page := 1; page <= f.MaxPages; page++ {
		part, err := FetchOrders(a, OrderFilter{
			UserID:     f.UserID,
			OrderSN:    f.OrderSN,
			ExpressNum: f.ExpressNum,
			Page:       page,
			Size:       f.Size,
		})
		if err != nil {
			// Adminka tokeni yetkazma tokeni emas — qayta urinishdan foyda yo'q.
			if errors.Is(err, ErrUnauthorized) {
				return nil, false, ErrAdminkaUnauthorized
			}
			return nil, false, err
		}
		all = append(all, part...)
		if len(part) < f.Size {
			break
		}
		truncated = page == f.MaxPages
	}
	return all, truncated, nil
}

// fetchArrivedTracks dashboarddagi (kelgan) buyurtmalarning trek raqamlarini
// to'plam qilib qaytaradi.
func fetchArrivedTracks(s Service, token string, f ProblemFilter, orders []AdminkaOrder) (map[string]bool, bool, error) {
	arrived := map[string]bool{}
	truncated := false

	// Mijoz bo'yicha qidirilsa — butun yetkazma ro'yxati bir marta olinadi.
	userID := f.UserID
	if userID == 0 {
		for _, o := range orders {
			if o.UserID > 0 {
				userID = o.UserID
				break
			}
		}
	}
	if userID > 0 {
		cut, err := collectTracks(s, token, f, DeliveryFilter{UserID: userID}, arrived)
		if err != nil {
			return nil, false, err
		}
		return arrived, cut, nil
	}

	// user_id topilmadi (order_sn/trek bo'yicha qidiruv) — har bir trek alohida.
	seen := map[string]bool{}
	for _, o := range orders {
		key := trackKey(o.ExpressNum)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		cut, err := collectTracks(s, token, f, DeliveryFilter{TrackNumber: o.ExpressNum}, arrived)
		if err != nil {
			return nil, false, err
		}
		truncated = truncated || cut
	}
	return arrived, truncated, nil
}

// collectTracks bitta filtr bo'yicha hamma sahifani aylanib, trek raqamlarini
// to'plamga qo'shadi. FetchDelivery bitta chaqiruvda delivered=false va true
// natijalarini birlashtiradi — shuning uchun to'xtash sharti "bo'sh sahifa".
// Qaytaruvchi qiymat — sikl max_pages ga tirab qolgani.
func collectTracks(s Service, token string, f ProblemFilter, df DeliveryFilter, arrived map[string]bool) (bool, error) {
	df.Size = f.Size
	truncated := false
	for page := 1; page <= f.MaxPages; page++ {
		df.Page = page
		part, err := FetchDelivery(s, token, df)
		if err != nil {
			return false, err
		}
		if len(part) == 0 {
			break
		}
		for _, d := range part {
			if key := trackKey(d.ExpressNum); key != "" {
				arrived[key] = true
			}
		}
		truncated = page == f.MaxPages
	}
	return truncated, nil
}

// trackKey trek raqamini solishtirishga tayyorlaydi — trek ba'zan harfli
// (YT75...) va atrofida bo'shliq bilan keladi.
func trackKey(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// ProblemJSON natijani tayyor JSON matn qilib qaytaradi.
func ProblemJSON(a Adminka, s Service, token string, f ProblemFilter) ([]byte, error) {
	res, err := FindProblemOrders(a, s, token, f)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(res, "", "  ")
}
