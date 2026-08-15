// Package daigou — adminka buyurtmalar endpointi bilan ishlaydi:
//
//	GET {USER_BASE_URL}/api/admin/daigou-orders
//	Authorization: Bearer {ADMINKA_TOKEN_BEARER}
//
// Bu manbada Xitoy tomonidagi ma'lumot bor: buyurtma raqami (order_sn),
// trek raqami, posilka, yo'lga chiqqan/qadoqlangan/omborga kirgan sanalar.
// "Buyurtmam qachon keladi?" degan savolga aynan shu sanalar kerak.
//
// Javob shakli barqaror emas (data / data.data / data.list ...), shuning
// uchun buyurtmalar xom map sifatida o'qiladi va kerakli maydon nom yoki
// yo'l bo'yicha qidiriladi.
package daigou

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultBaseURL — USER_BASE_URL berilmaganda ishlatiladi.
const defaultBaseURL = "https://api.sahiy.uz"

// maxPages — bir qidiruvda o'qiladigan sahifa chegarasi (cheksiz aylanmaslik
// uchun). Bitta mijozning buyurtmalari uchun bundan ko'pi kerak emas.
const maxPages = 5

// maxShown — AI kontekstiga qo'shiladigan buyurtmalar soni.
const maxShown = 5

// Order — bitta buyurtma (xom JSON obyekt).
type Order map[string]any

// Params — qidiruv parametrlari (bo'shlari so'rovga yuboriladi, lekin
// serverda e'tiborga olinmaydi).
type Params struct {
	Page       string
	Size       string
	Status     string
	Keyword    string
	Platform   string
	UserID     string
	OrderSN    string
	ExpressNum string
	BeginDate  string
	EndDate    string
}

// Client — adminka API client.
type Client struct {
	BaseURL string
	Token   string
	http    *http.Client
}

// New yangi client. baseURL bo'sh bo'lsa api.sahiy.uz ishlatiladi.
func New(baseURL, token string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Enabled — token bormi (yo'q bo'lsa qidiruv o'chiq).
func (c *Client) Enabled() bool { return c != nil && c.Token != "" }

// ByOrderSN buyurtma raqami bo'yicha ("DG60605678").
func (c *Client) ByOrderSN(sn string) ([]Order, error) {
	return c.Search(Params{OrderSN: sn})
}

// ByExpressNum Xitoydagi trek raqami bo'yicha.
func (c *Client) ByExpressNum(num string) ([]Order, error) {
	return c.Search(Params{ExpressNum: num})
}

// ByUser mijoz profil id'si bo'yicha.
func (c *Client) ByUser(id int64) ([]Order, error) {
	return c.Search(Params{UserID: strconv.FormatInt(id, 10)})
}

// Search sahifalarni oxirigacha (maxPages chegarasida) aylanib chiqadi.
// Params.Page berilgan bo'lsa faqat o'sha sahifa o'qiladi.
func (c *Client) Search(p Params) ([]Order, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("adminka tokeni yo'q (ADMINKA_TOKEN_BEARER)")
	}
	single := p.Page != ""
	if p.Page == "" {
		p.Page = "1"
	}
	if p.Size == "" {
		p.Size = "20"
	}

	var all []Order
	for page := 1; page <= maxPages; page++ {
		if !single {
			p.Page = strconv.Itoa(page)
		}
		body, err := c.Fetch(p)
		if err != nil {
			return nil, err
		}
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("javobni JSON sifatida o'qib bo'lmadi: %w", err)
		}
		got := FindOrders(payload)
		all = append(all, got...)

		last, _ := PageInfo(payload)
		if single || len(got) == 0 || page >= last {
			break
		}
	}
	return all, nil
}

// Fetch bitta so'rov: xom javob tanasini qaytaradi.
func (c *Client) Fetch(p Params) ([]byte, error) {
	q := url.Values{}
	q.Set("page", p.Page)
	q.Set("size", p.Size)
	q.Set("status", p.Status)
	q.Set("keyword", p.Keyword)
	q.Set("platform", p.Platform)
	q.Set("user_id", p.UserID)
	q.Set("order_sn", p.OrderSN)
	q.Set("express_num", p.ExpressNum)
	q.Set("begin_date", p.BeginDate)
	q.Set("end_date", p.EndDate)

	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/admin/daigou-orders?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	token := c.Token
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, trim(body, 300))
	}
	return body, nil
}

// PageInfo — meta ichidan last_page va total ni oladi.
func PageInfo(payload any) (last, total int) {
	last, total = 1, 0
	m, ok := payload.(map[string]any)
	if !ok {
		return
	}
	meta, ok := m["meta"].(map[string]any)
	if !ok {
		return
	}
	if v, ok := meta["last_page"].(float64); ok && v > 0 {
		last = int(v)
	}
	if v, ok := meta["total"].(float64); ok {
		total = int(v)
	}
	return
}

// FindOrders — javob ichidan buyurtmalar ro'yxatini topadi
// (data, data.data, data.list, data.rows, data.items va h.k.).
func FindOrders(v any) []Order {
	switch t := v.(type) {
	case []any:
		var out []Order
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				return nil
			}
			out = append(out, m)
		}
		if len(out) > 0 && isOrder(out[0]) {
			return out
		}
		return nil
	case map[string]any:
		for _, key := range []string{"data", "list", "rows", "items", "result", "records"} {
			if child, ok := t[key]; ok {
				if got := FindOrders(child); got != nil {
					return got
				}
			}
		}
	}
	return nil
}

func isOrder(m Order) bool {
	for _, key := range []string{"order_sn", "receiver_name", "express_num", "status"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

// row — Summary'da chiqadigan bitta qator.
type row struct {
	label string
	key   string
	paths []string
}

// rows — AI uchun kerakli maydonlar. Terminal chiqishi bilan bir xil manba:
// bu yerda nima bo'lsa, `cmd/orders` ham shuni ko'rsatadi.
var rows = []row{
	{"Holat", "status", nil},
	{"Trek raqami", "express_num", []string{"express.express_num"}},
	{"Posilka", "package_name", []string{"express.package.package_name"}},
	{"Soni", "quantity", []string{"express.package.qty"}},
	{"Yetkazish yo'nalishi", "express_line", nil},
	{"Qabul qiluvchi", "receiver_name", []string{"address.receiver_name"}},
	{"Viloyat", "province", []string{"address.province"}},
	{"Hudud", "area", []string{"address.area"}},
	{"Shahar/tuman", "sub_area", []string{"address.sub_area"}},
	{"Buyurtma yaratilgan", "created_at", nil},
	{"To'lov vaqti", "paid_at", nil},
	{"Yo'lga chiqqan", "shipped_at", []string{"express.package.order.shipped_at"}},
	{"Qadoqlangan", "packed_at", []string{"express.package.order.packed_at"}},
	{"Omborga kirgan", "in_storage_at", []string{"express.package.in_storage_at"}},
}

// Summary buyurtmalarni AI o'qiydigan qisqa matnga aylantiradi.
// Xom JSON emas: javobda yuzlab keraksiz maydon bor, ular token yeydi.
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
		sn := Pick(o, "order_sn")
		if sn == "" {
			sn = "(raqamsiz)"
		}
		fmt.Fprintf(&b, "\n• Buyurtma %s\n", sn)
		for _, r := range rows {
			if v := Pick(o, r.key, r.paths...); v != "" {
				fmt.Fprintf(&b, "  %s: %s\n", r.label, v)
			}
		}
		if amount := Pick(o, "amount"); amount != "" {
			fmt.Fprintf(&b, "  Summa: %s %s\n", amount, Pick(o, "currency"))
		}
	}
	if len(list) > len(shown) {
		fmt.Fprintf(&b, "\n(jami %d ta buyurtma, birinchi %d tasi ko'rsatildi)\n", len(list), len(shown))
	}
	return b.String()
}

// Pick — berilgan yo'llarni tartib bilan sinaydi, topilmasa kalit bo'yicha
// chuqur qidiruvga o'tadi.
func Pick(m Order, key string, paths ...string) string {
	for _, p := range paths {
		if s := path(m, p); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return str(m, key)
}

// str — qiymatni turidan qat'i nazar matnga aylantiradi. Kalit yuqori
// darajada bo'lmasa, ichma-ich obyekt/ro'yxatlardan ham qidiradi.
func str(m Order, key string) string {
	if v, ok := m[key]; ok {
		if s := toStr(named(v)); s != "" {
			return s
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s := searchDeep(m[k], key); s != "" {
			return s
		}
	}
	return ""
}

// path — "address.receiver_name" ko'rinishidagi aniq yo'l bo'yicha qiymat.
func path(m Order, dotted string) string {
	var cur any = map[string]any(m)
	for _, part := range strings.Split(dotted, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = obj[part]
		if !ok {
			return ""
		}
	}
	return toStr(named(cur))
}

func searchDeep(v any, key string) string {
	switch t := v.(type) {
	case map[string]any:
		return str(t, key)
	case []any:
		for _, item := range t {
			if s := searchDeep(item, key); s != "" {
				return s
			}
		}
	}
	return ""
}

// named — obyekt ichida "name" bo'lsa, o'sha o'qiladigan nomni qaytaradi
// (area, sub_area, express_line kabi maydonlar uchun).
func named(v any) any {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"name", "title"} {
			if n, ok := m[k].(string); ok && strings.TrimSpace(n) != "" {
				return n
			}
		}
	}
	return v
}

func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return fmt.Sprintf("%t", t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case []any:
		var parts []string
		for _, item := range t {
			if s := toStr(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func trim(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append(b[:n:n], []byte("...")...)
}
