// Bitta buyurtma kartasi: DG yoki trek raqami bo'yicha adminka va
// yetkazma ma'lumoti birga qaytariladi.
package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Bitta buyurtma kartasi uchun sahifa hajmi — DG yoki trek bo'yicha qidiruvda
// javob bir nechta qatordan oshmaydi.
const orderCardSize = 20

// OrderCard — bitta buyurtmaning ikkala tomondagi ma'lumoti.
// `dashboard` maydoni: yetkazmada topilmasa `false`, topilsa obyekt.
type OrderCard struct {
	Query      string        `json:"query"`       // qidirilgan matn (DG.. yoki trek)
	QueryKind  string        `json:"query_kind"`  // "order_sn" yoki "express_num"
	Found      bool          `json:"found"`       // adminkada topildimi
	OrderSN    string        `json:"order_sn"`    // topilgan buyurtma raqami
	ExpressNum string        `json:"express_num"` // trek raqami (bo'sh bo'lishi mumkin)
	Adminka    *AdminkaOrder `json:"adminka"`     // topilmasa null
	Dashboard  any           `json:"dashboard"`   // false yoki DeliveryOrder
	// DashboardCount — bitta trekka bir nechta yetkazma qatori kelsa,
	// `dashboard` da birinchisi turadi, soni shu yerda ko'rinadi.
	DashboardCount int    `json:"dashboard_count"`
	Note           string `json:"note,omitempty"`
}

// FetchOrderCard bitta buyurtmani DG raqami yoki trek raqami bo'yicha topadi:
// avval adminkadan (FetchOrders), keyin o'sha buyurtmaning trek raqami bilan
// yetkazmadan (FetchDelivery). Trek yo'q bo'lsa `dashboard: false` qaytadi.
func FetchOrderCard(a Adminka, s Service, token, query string) (OrderCard, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return OrderCard{}, fmt.Errorf("order_sn yoki express_num berilmagan")
	}

	card := OrderCard{
		Query:     query,
		QueryKind: queryKind(query),
		Dashboard: false,
	}

	// 1. Adminka tomoni — DG bo'lsa order_sn, aks holda express_num bo'yicha.
	f := OrderFilter{Size: orderCardSize}
	if card.QueryKind == "order_sn" {
		f.OrderSN = query
	} else {
		f.ExpressNum = query
	}
	orders, err := FetchOrders(a, f)
	if err != nil {
		// Adminka tokeni yetkazma tokeni emas — qayta urinishdan foyda yo'q.
		if errors.Is(err, ErrUnauthorized) {
			return OrderCard{}, ErrAdminkaUnauthorized
		}
		return OrderCard{}, err
	}

	track := ""
	if len(orders) > 0 {
		o := orders[0]
		card.Found = true
		card.Adminka = &o
		card.OrderSN = o.OrderSN
		card.ExpressNum = o.ExpressNum
		track = trackKey(o.ExpressNum)
		if len(orders) > 1 {
			card.Note = fmt.Sprintf("adminkada %d ta qator topildi, birinchisi olindi", len(orders))
		}
	}

	// 2. Trek raqami: adminkadagisi, u bo'lmasa qidiruvning o'zi.
	if track == "" && card.QueryKind == "express_num" {
		track = trackKey(query)
		if card.ExpressNum == "" {
			card.ExpressNum = query
		}
	}
	if track == "" {
		// Trek yo'q — solishtirishga hech narsa yo'q, dashboard so'ralmaydi.
		if card.Note == "" {
			if card.Found {
				// To'langan, lekin Xitoyda hali trek berilmagan.
				card.Note = "trek raqami yo'q — yetkazma tomoni tekshirilmadi"
			} else {
				card.Note = "adminkada topilmadi"
			}
		}
		return card, nil
	}

	// 3. Yetkazma tomoni.
	rows, err := FetchDelivery(s, token, DeliveryFilter{TrackNumber: track, Size: orderCardSize})
	if err != nil {
		return OrderCard{}, err
	}
	card.DashboardCount = len(rows)
	if len(rows) > 0 {
		card.Dashboard = rows[0]
		if !card.Found {
			// Adminkada yo'q, lekin yetkazmada bor — trek bo'yicha qidiruv.
			card.Note = "adminkada topilmadi, faqat yetkazma ma'lumoti bor"
		}
	}
	return card, nil
}

// queryKind qidiruv matni DG buyurtma raqamimi yoki trek raqamimi — shuni
// aniqlaydi. Adminkadagi buyurtma raqami doim "DG" bilan boshlanadi.
func queryKind(q string) string {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(q)), "DG") {
		return "order_sn"
	}
	return "express_num"
}

// OrderCardJSON natijani tayyor JSON matn qilib qaytaradi.
func OrderCardJSON(a Adminka, s Service, token, query string) ([]byte, error) {
	card, err := FetchOrderCard(a, s, token, query)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(card, "", "  ")
}
