package support

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// AdminkaPath — Xitoy tomonidagi (daigou) buyurtmalar ro'yxati.
const AdminkaPath = "/api/admin/daigou-orders"

// DefaultAdminkaBaseURL — USER_BASE_URL bo'sh bo'lsa ishlatiladi.
const DefaultAdminkaBaseURL = "https://api.sahiy.uz"

// Adminka — adminka API'siga ulanish ma'lumoti. Login yo'q: token qo'lda
// .env dagi ADMINKA_TOKEN_BEARER ga qo'yiladi.
type Adminka struct {
	BaseURL string
	Token   string
}

// AdminkaFromEnv .env dan USER_BASE_URL va ADMINKA_TOKEN_BEARER ni oladi.
func AdminkaFromEnv() Adminka {
	a := Adminka{
		BaseURL: os.Getenv("USER_BASE_URL"),
		Token:   strings.TrimPrefix(os.Getenv("ADMINKA_TOKEN_BEARER"), "Bearer "),
	}
	if a.BaseURL == "" {
		a.BaseURL = DefaultAdminkaBaseURL
	}
	return a
}

// OrderFilter — qidiruv shartlari. Uchtasidan bittasi to'ldiriladi:
// UserID (Ilova Profil ID), OrderSN (DG...) yoki ExpressNum (trek raqami).
type OrderFilter struct {
	UserID     int64  `json:"user_id"`
	OrderSN    string `json:"order_sn"`
	ExpressNum string `json:"express_num"`
	Status     string `json:"status"`
	Keyword    string `json:"keyword"`
	Page       int    `json:"page"`
	Size       int    `json:"size"`
}

// AdminkaOrder — buyurtmadan olinadigan maydonlar. Xom javob bir buyurtma
// uchun ~10 KB, shu sababli faqat shular saqlanadi.
type AdminkaOrder struct {
	OrderSN      string `json:"order_sn"`      // asosiy buyurtma raqami (DG...)
	UserID       int64  `json:"user_id"`       // Ilova Profil ID
	Status       int    `json:"status"`        // raqamli holat (6 — yakunlangan)
	Amount       string `json:"amount"`        // umumiy summa
	ReceiverName string `json:"receiver_name"` // qabul qiluvchi
	Province     string `json:"province"`      // viloyat
	Area         string `json:"area"`          // hudud
	SubArea      string `json:"sub_area"`      // shahar/tuman
	Street       string `json:"street"`        // ko'cha, uy
	ExpressLine  string `json:"express_line"`  // yetkazish yo'nalishi
	ExpressNum   string `json:"express_num"`   // Xitoydagi trek/posilka raqami
	PackageName  string `json:"package_name"`  // posilka nomi
	Quantity     int    `json:"quantity"`      // soni
	PayStatus    int    `json:"pay_status"`    // 1 — to'langan, 0 — to'lanmagan
	// B2CPercentage — mijoz turi shu maydondan aniqlanadi:
	// noldan katta bo'lsa oddiy mijoz (B2C), nol bo'lsa ulgurji (B2B).
	B2CPercentage float64 `json:"b2c_percentage"`
	PaidAt        string  `json:"paid_at"`       // to'lov qilingan vaqt
	CreatedAt     string  `json:"created_at"`    // buyurtma yaratilgan vaqt
	ShippedAt     string  `json:"shipped_at"`    // yo'lga chiqqan sana
	PackedAt      string  `json:"packed_at"`     // qadoqlangan vaqt
	InStorageAt   string  `json:"in_storage_at"` // omborga kirgan vaqt
}

// FetchOrders adminkadan buyurtmalarni oladi (GET).
// Barcha parametrlar har doim yuboriladi — bo'shlari serverda e'tiborga olinmaydi.
func FetchOrders(a Adminka, f OrderFilter) ([]AdminkaOrder, error) {
	if a.Token == "" {
		return nil, fmt.Errorf("ADMINKA_TOKEN_BEARER berilmagan")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 10
	}
	base := a.BaseURL
	if base == "" {
		base = DefaultAdminkaBaseURL
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(f.Page))
	q.Set("size", strconv.Itoa(f.Size))
	q.Set("status", f.Status)
	q.Set("keyword", f.Keyword)
	q.Set("platform", "")
	q.Set("user_id", "")
	if f.UserID > 0 {
		q.Set("user_id", strconv.FormatInt(f.UserID, 10))
	}
	q.Set("order_sn", f.OrderSN)
	q.Set("express_num", f.ExpressNum)
	q.Set("begin_date", "")
	q.Set("end_date", "")

	url := strings.TrimRight(base, "/") + AdminkaPath + "?" + q.Encode()
	newReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("so'rov yaratish: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.Token)
		return req, nil
	}

	// Adminka Cloudflare orqasida: 522/5xx vaqtinchalik bo'lishi mumkin.
	client := &http.Client{Timeout: 30 * time.Second}
	status, raw, err := doWithRetry(client, newReq, Retries())
	if err != nil {
		return nil, fmt.Errorf("so'rov yuborish: %w", err)
	}
	if status == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("adminka buyurtmalari (status %d): %s", status, snippet(raw))
	}

	// Javob shakli barqaror emas — xom map sifatida o'qib, maydonlar
	// yo'l bo'yicha olinadi.
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}

	rows := findList(body)
	orders := make([]AdminkaOrder, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		orders = append(orders, pickOrder(m))
	}
	return orders, nil
}

// OrdersJSON buyurtmalarni tayyor JSON matn qilib qaytaradi.
func OrdersJSON(a Adminka, f OrderFilter) ([]byte, error) {
	orders, err := FetchOrders(a, f)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Count  int            `json:"count"`
		Orders []AdminkaOrder `json:"orders"`
	}{len(orders), orders}, "", "  ")
}

// pickOrder xom buyurtmadan kerakli maydonlarni yig'adi.
// Bitta maydon bir nechta yo'lda uchraydi — birinchi topilgani olinadi.
func pickOrder(m map[string]any) AdminkaOrder {
	return AdminkaOrder{
		OrderSN:      str(get(m, "order_sn")),
		UserID:       num64(get(m, "user_id")),
		Status:       int(num64(get(m, "status"))),
		Amount:       str(get(m, "amount")),
		ReceiverName: str(first(m, "address.receiver_name", "express.package.contact_info.receiver_name")),
		Province:     str(get(m, "address.province")),
		Area:         str(get(m, "address.area.name")),
		SubArea:      str(get(m, "address.sub_area.name")),
		Street:       str(get(m, "address.street")),
		ExpressLine:  str(first(m, "express_line.name", "express.line.name")),
		ExpressNum: str(first(m,
			"express.express_num",
			"express.package.express_num",
			"expresses.0.pivot.express_num",
			"purchase_packages.0.express_num",
		)),
		PackageName: str(first(m, "express.package.package_name", "package_name")),
		Quantity:    quantity(m),
		PayStatus:   int(num64(first(m, "pay_status", "order.pay_status", "payment.status"))),
		B2CPercentage: numFloat(first(m,
			"skus.0.sku_info.B2C_percentage",
			"skus.0.sku_info.b2c_percentage",
			"B2C_percentage",
		)),
		PaidAt:      str(first(m, "paid_at", "order.paid_at", "payment.paid_at", "pay_time")),
		CreatedAt:   str(get(m, "created_at")),
		ShippedAt:   str(first(m, "express.package.order.shipped_at", "shipped_at")),
		PackedAt:    str(first(m, "express.package.order.packed_at", "packed_at")),
		InStorageAt: str(first(m, "express.package.in_storage_at", "in_storage_at")),
	}
}

// quantity — buyurtmadagi mahsulotlar sonini qo'shib chiqadi.
func quantity(m map[string]any) int {
	for _, path := range []string{"skus", "platform_order.skus"} {
		list, ok := get(m, path).([]any)
		if !ok {
			continue
		}
		total := 0
		for _, it := range list {
			if sku, ok := it.(map[string]any); ok {
				total += int(num64(sku["quantity"]))
			}
		}
		if total > 0 {
			return total
		}
	}
	return int(num64(get(m, "quantity")))
}

// get "a.b.0.c" ko'rinishidagi yo'l bo'yicha qiymatni oladi.
// Yo'l uzilsa yoki qiymat null bo'lsa nil qaytaradi.
func get(m map[string]any, path string) any {
	var cur any = m
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return nil
			}
			cur = node[i]
		default:
			return nil
		}
		if cur == nil {
			return nil
		}
	}
	return cur
}

// first bir nechta yo'ldan birinchi bo'sh bo'lmaganini qaytaradi.
func first(m map[string]any, paths ...string) any {
	for _, p := range paths {
		if v := get(m, p); v != nil && str(v) != "" {
			return v
		}
	}
	return nil
}

// str istalgan qiymatni matnga aylantiradi (raqam ham matn bo'lib chiqadi).
func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

// numFloat kasrli raqamni oladi (matn bo'lsa ham).
func numFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

// num64 raqamni oladi; matn bo'lsa ham parse qilib ko'radi.
func num64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}

// findList javobdagi buyurtmalar massivini topadi: data, data.data,
// data.list, data.rows, data.items — server versiyasiga qarab o'zgaradi.
func findList(body map[string]any) []any {
	for _, p := range []string{"data", "data.data", "data.list", "data.rows", "data.items"} {
		if list, ok := get(body, p).([]any); ok {
			return list
		}
	}
	return nil
}
