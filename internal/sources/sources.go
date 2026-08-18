// Package sources — agent tashqi API'lardan oladigan ma'lumotning YAGONA
// yig'ilish joyi.
//
// Uchta manba bor va uchalasi ham xom javobida bizga keraksiz yuzlab maydon
// olib keladi:
//
//	delivery — GET /api/v2/admin/delivery/orders/filter  (internal/orders)
//	daigou   — GET /api/admin/daigou-orders              (internal/daigou)
//	support  — GET /api/v1/support.chat.message/...      (internal/support)
//
// Ilgari "qaysi raqam bo'yicha qaysi manbaga borish" mantig'i main.go da,
// undan nusxasi esa sahiy/chat, sahiy/dashboard va cmd/orders da yashardi.
// Endi u faqat shu yerda: main.go ham, CLI'lar ham, /api/source/* HTTP
// endpointlari ham AYNAN shu funksiyalarni chaqiradi.
//
// Paket faqat O'QIYDI: "o'qildi" qo'yilmaydi, xabar yuborilmaydi, hech narsa
// yozilmaydi.
package sources

import (
	"fmt"
	"strings"

	"sahiy-agent/internal/client"
	"sahiy-agent/internal/daigou"
	"sahiy-agent/internal/orders"
	"sahiy-agent/internal/support"
)

// defaultLimit — suhbatdan o'qiladigan xabarlar soni (chaqiruvchi bermasa).
const defaultLimit = 50

// maxImages — javobga qo'shiladigan rasm havolalari soni (agentdagi bilan
// bir xil chegara).
const maxImages = 3

// Sources — manbalar to'plami. Nil maydon "bu manba o'chiq" degani:
// endpoint 200 qaytaradi, blok esa `disabled` bilan keladi.
type Sources struct {
	Orders *orders.Lookup
	// Adminka — daigou (Xitoy tomoni) mijozi. Nomi metod nomi bilan
	// to'qnashmasligi uchun manba nomidan farq qiladi.
	Adminka *daigou.Client
	// Chat — support API mijozi. Funksiya, chunki token muddati tugaydi
	// va har so'rovda yangilanishi mumkin (main.go: apiClient).
	Chat func() (*client.Client, error)
}

// Query — qidiruv shartlari. Hammasi ixtiyoriy, lekin kamida bittasi
// bo'lishi kerak (Empty tekshiradi).
type Query struct {
	ClientID       int64  // support'dagi client_id = delivery'dagi user_id
	Track          string // track / express_num — ikkala manbada ham ishlaydi
	OrderSN        string // daigou buyurtma raqami (DG...)
	ConversationID int64  // support suhbati
	Limit          int    // suhbatdan nechta xabar (0 — defaultLimit)
}

// Empty — hech qanday qidiruv sharti berilmaganmi.
func (q Query) Empty() bool {
	return q.ClientID == 0 && q.ConversationID == 0 &&
		strings.TrimSpace(q.Track) == "" && strings.TrimSpace(q.OrderSN) == ""
}

func (q Query) limit() int {
	if q.Limit > 0 {
		return q.Limit
	}
	return defaultLimit
}

// DeliveryBlock — O'zbekiston tomoni: qaysi filialda, topshirilganmi, to'lov.
type DeliveryBlock struct {
	Disabled bool           `json:"disabled,omitempty"`
	Query    string         `json:"query"` // qanday qidirildi
	Count    int            `json:"count"`
	Items    []orders.Order `json:"items"`
	Summary  string         `json:"summary"` // modelga ketadigan matn
	Error    string         `json:"error,omitempty"`
}

// DaigouBlock — Xitoy tomoni: yo'lga chiqqan/qadoqlangan/omborga kirgan
// sanalar, posilka, trek.
//
// Items — xom javob emas: daigou.Fields orqali kelishilgan maydonlargina
// qoladi (xom obyekt bir buyurtma uchun ~10 KB).
type DaigouBlock struct {
	Disabled bool                `json:"disabled,omitempty"`
	Query    string              `json:"query"`
	Count    int                 `json:"count"`
	Items    []map[string]string `json:"items"`
	Summary  string              `json:"summary"`
	Error    string              `json:"error,omitempty"`

	// Raw — xom buyurtmalar. JSON'ga chiqmaydi (juda katta), lekin
	// chaqiruvchi bir nechta qidiruv natijasini birlashtirib bitta
	// daigou.Summary yasashi uchun kerak.
	Raw []daigou.Order `json:"-"`
}

// Message — suhbatning bitta xabari (xom javobdan kerakli maydonlar).
type Message struct {
	ID        int64  `json:"id"`
	From      string `json:"from"` // "client" yoki "agent"
	Text      string `json:"text"`
	IsImage   bool   `json:"is_image,omitempty"`
	CreatedAt string `json:"created_at"`
}

// SupportBlock — yordam xizmati suhbati.
type SupportBlock struct {
	Disabled       bool      `json:"disabled,omitempty"`
	ConversationID int64     `json:"conversation_id"`
	ClientID       int64     `json:"client_id,omitempty"`
	Title          string    `json:"title,omitempty"`
	Count          int       `json:"count"`
	Messages       []Message `json:"messages"`
	Images         []string  `json:"images,omitempty"`
	Transcript     string    `json:"transcript"` // modelga ketadigan matn
	Error          string    `json:"error,omitempty"`
}

// Result — /api/source/all javobi.
type Result struct {
	Query    Query          `json:"-"`
	Delivery *DeliveryBlock `json:"delivery,omitempty"`
	Daigou   *DaigouBlock   `json:"daigou,omitempty"`
	Support  *SupportBlock  `json:"support,omitempty"`
}

// Delivery — yetkazma buyurtmalari.
//
// Track berilgan bo'lsa u bo'yicha, aks holda mijoz id'si bo'yicha
// qidiriladi (agentdagi tartibning aynan o'zi).
func (s *Sources) Delivery(q Query) *DeliveryBlock {
	b := &DeliveryBlock{Items: []orders.Order{}}
	if s == nil || s.Orders == nil || !s.Orders.Enabled() {
		b.Disabled = true
		b.Error = "yetkazma qidiruvi o'chiq (SERVICE_PHONE / SERVICE_PASSWORD yo'q)"
		return b
	}

	var (
		list []orders.Order
		err  error
	)
	switch {
	case strings.TrimSpace(q.Track) != "":
		b.Query = orders.TrackParam + "=" + q.Track
		list, err = s.Orders.ByTrack(strings.TrimSpace(q.Track))
	case q.ClientID != 0:
		b.Query = fmt.Sprintf("%s=%d", orders.UserParam, q.ClientID)
		list, err = s.Orders.ByUser(q.ClientID)
	default:
		b.Error = "track yoki client_id kerak"
		return b
	}
	if err != nil {
		b.Error = err.Error()
		return b
	}
	if len(list) > 0 {
		b.Items = list
	}
	b.Count = len(list)
	b.Summary = orders.Summary(list)
	return b
}

// Daigou — adminka buyurtmalari.
//
// OrderSN → order_sn, Track → express_num, aks holda user_id bo'yicha
// (agentdagi tanlov mantig'i: "DG" bilan boshlansa buyurtma raqami).
func (s *Sources) Daigou(q Query) *DaigouBlock {
	b := &DaigouBlock{Items: []map[string]string{}}
	if s == nil || s.Adminka == nil || !s.Adminka.Enabled() {
		b.Disabled = true
		b.Error = "adminka qidiruvi o'chiq (ADMINKA_TOKEN_BEARER yo'q)"
		return b
	}

	sn := strings.TrimSpace(q.OrderSN)
	track := strings.TrimSpace(q.Track)
	// Track maydoniga DG... yozilgan bo'lsa u aslida buyurtma raqami.
	if sn == "" && strings.HasPrefix(strings.ToUpper(track), "DG") {
		sn, track = track, ""
	}

	var (
		list []daigou.Order
		err  error
	)
	switch {
	case sn != "":
		b.Query = "order_sn=" + sn
		list, err = s.Adminka.ByOrderSN(sn)
	case track != "":
		b.Query = "express_num=" + track
		list, err = s.Adminka.ByExpressNum(track)
	case q.ClientID != 0:
		b.Query = fmt.Sprintf("user_id=%d", q.ClientID)
		list, err = s.Adminka.ByUser(q.ClientID)
	default:
		b.Error = "order_sn, express_num yoki client_id kerak"
		return b
	}
	if err != nil {
		b.Error = err.Error()
		return b
	}

	for _, o := range daigou.Sorted(list) {
		b.Items = append(b.Items, daigou.Fields(o))
	}
	b.Raw = list
	b.Count = len(list)
	b.Summary = daigou.Summary(list)
	return b
}

// Support — suhbat xabarlari.
//
// ConversationID berilmasa mijozning eng oxirgi suhbati olinadi.
func (s *Sources) Support(q Query) *SupportBlock {
	b := &SupportBlock{Messages: []Message{}, ConversationID: q.ConversationID, ClientID: q.ClientID}
	if s == nil || s.Chat == nil {
		b.Disabled = true
		b.Error = "support API o'chiq"
		return b
	}
	c, err := s.Chat()
	if err != nil {
		b.Error = err.Error()
		return b
	}

	convID := q.ConversationID
	if convID == 0 {
		if q.ClientID == 0 {
			b.Error = "conversation_id yoki client_id kerak"
			return b
		}
		chats, err := support.SearchByClient(c, q.ClientID, 1, 20, nil)
		if err != nil {
			b.Error = err.Error()
			return b
		}
		if len(chats) == 0 {
			return b
		}
		// Eng oxirgi suhbat — id eng kattasi.
		latest := chats[0]
		for _, ch := range chats[1:] {
			if ch.ID > latest.ID {
				latest = ch
			}
		}
		convID = latest.ID
		b.Title = latest.Title
		if latest.ClientID != nil {
			b.ClientID = *latest.ClientID
		}
	}
	b.ConversationID = convID

	msgs, err := support.FetchMessages(c, convID, 1, q.limit())
	if err != nil {
		b.Error = err.Error()
		return b
	}
	for _, m := range msgs {
		b.Messages = append(b.Messages, Message{
			ID:        m.ID,
			From:      m.SenderType,
			Text:      m.Message,
			IsImage:   m.IsImage(),
			CreatedAt: m.CreatedAt,
		})
	}
	b.Count = len(msgs)
	b.Images = support.ImageURLs(msgs, maxImages)
	b.Transcript = support.Transcript(msgs)
	return b
}

// All — uchala manba bitta so'rovda.
//
// Bitta manba yiqilsa qolgani baribir qaytadi (blokdagi `error`) — agentning
// xulqi ham shunday: yarim ma'lumot hech nimadan yaxshi.
//
// client_id berilmagan bo'lsa, u avval suhbatdan aniqlanadi va shundan keyin
// buyurtmalar so'raladi: conversation_id bilan kelgan so'rov ham to'liq
// javob bersin.
func (s *Sources) All(q Query) Result {
	res := Result{Query: q}

	if q.ConversationID != 0 || q.ClientID != 0 {
		res.Support = s.Support(q)
		if q.ClientID == 0 && res.Support.ClientID != 0 {
			q.ClientID = res.Support.ClientID
		}
	}

	if q.ClientID != 0 || strings.TrimSpace(q.Track) != "" {
		res.Delivery = s.Delivery(q)
	}
	if q.ClientID != 0 || strings.TrimSpace(q.Track) != "" || strings.TrimSpace(q.OrderSN) != "" {
		res.Daigou = s.Daigou(q)
	}
	return res
}

// OrderSummary — modelga beriladigan buyurtma matni: ikkala manba birga.
//
// main.go dagi lookupOrders shuni chaqiradi — model ko'radigan matn
// endpoint qaytaradigan `summary` bilan bir xil bo'lishi uchun.
func (s *Sources) OrderSummary(q Query, delivery, adminka bool) string {
	var parts []string
	if delivery {
		if b := s.Delivery(q); b.Summary != "" {
			parts = append(parts, b.Summary)
		}
	}
	if adminka {
		if b := s.Daigou(q); b.Summary != "" {
			parts = append(parts, b.Summary)
		}
	}
	return strings.Join(parts, "\n")
}
