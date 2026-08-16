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

// row — Summary'da chiqadigan bitta qator. key/paths bo'yicha qiymat
// olinadi; hisoblanadigan qatorlar uchun fn ishlatiladi.
type row struct {
	label string
	key   string
	paths []string
	fn    func(Order) string
}

// rows — javobga qo'shiladigan maydonlar, kelishilgan tartibda. Terminal
// chiqishi bilan bir xil manba: bu yerda nima bo'lsa, `cmd/orders` ham,
// AI ham shuni ko'radi. Yo'llar haqiqiy javobdan olingan (aniq yo'l topilmasa
// Pick kalit bo'yicha chuqur qidiruvga o'tadi).
var rows = []row{
	{label: "Profil ID", key: "user_id"},
	{label: "Holat", key: "status", fn: statusText},
	{label: "Summa", key: "amount", fn: amountText},
	{label: "Qabul qiluvchi", key: "receiver_name", paths: []string{"address.receiver_name"}},
	{label: "Viloyat", key: "province", paths: []string{"address.province"}},
	{label: "Hudud", key: "area", paths: []string{"address.area"}},
	{label: "Shahar/tuman", key: "sub_area", paths: []string{"address.sub_area"}},
	{label: "Ko'cha va uy", key: "street", paths: []string{"address.street", "address.address"}},
	{label: "Yetkazish yo'nalishi", key: "express_line"},
	{label: "Trek raqami", key: "express_num", paths: []string{"express.express_num"}},
	{label: "Posilka", key: "package_name", paths: []string{"express.package.package_name"}},
	{label: "Soni", key: "quantity", paths: []string{"express.package.qty"}},
	{label: "Buyurtma yaratilgan", key: "created_at"},
	{label: "Yo'lga chiqqan", key: "shipped_at", paths: []string{"express.package.order.shipped_at"}},
	{label: "Qadoqlangan", key: "packed_at", paths: []string{"express.package.order.packed_at"}},
	{label: "Omborga kirgan", key: "in_storage_at", paths: []string{"express.package.in_storage_at"}},
	{label: "Mijoz turi", fn: ClientType},
}

// statusNames — buyurtmaning raqamli holati. Javobdagi status_name xitoycha
// ("交易完成"), shuning uchun o'zbekcha nom shu yerda saqlanadi. Ro'yxatda
// yo'q kod raqam holida ko'rsatiladi — yangi kod uchrasa shu yerga qo'shing.
var statusNames = map[string]string{
	"6":  "Tranzaksiya yakunlangan", // 交易完成
	"10": "Bekor qilingan",          // 已取消
}

// statusText — "6 (Tranzaksiya yakunlangan)" ko'rinishi. Raqamning o'zi
// AI uchun ma'nosiz, shuning uchun nomi ham qo'shiladi.
func statusText(o Order) string {
	v := Pick(o, "status")
	if v == "" {
		return ""
	}
	if name, ok := statusNames[v]; ok {
		return v + " (" + name + ")"
	}
	return v
}

// amountText — summa va valyuta birga ("27 CNY").
func amountText(o Order) string {
	amount := Pick(o, "amount")
	if amount == "" {
		return ""
	}
	return strings.TrimSpace(amount + " " + Pick(o, "currency"))
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
			v := ""
			if r.fn != nil {
				v = r.fn(o)
			} else {
				v = Pick(o, r.key, r.paths...)
			}
			if v != "" {
				fmt.Fprintf(&b, "  %s: %s\n", r.label, v)
			}
		}
	}
	if len(list) > len(shown) {
		fmt.Fprintf(&b, "\n(jami %d ta buyurtma, birinchi %d tasi ko'rsatildi)\n", len(list), len(shown))
	}
	return b.String()
}

// b2cKey — mijoz turini ko'rsatadigan maydon. Asosiy yo'li
// skus[0].sku_info.B2C_percentage; topilmasa buyurtma ichidan kalit
// bo'yicha qidiriladi (javob shakli barqaror emas).
const b2cKey = "B2C_percentage"

// b2cPaths — b2cKey qidiriladigan aniq yo'llar.
var b2cPaths = []string{
	"skus[0].sku_info." + b2cKey,
	"skus[0]." + b2cKey,
	"sku_info." + b2cKey,
}

// ClientType — mijoz turi: faqat "B2C" yoki "B2B".
//
// B2C_percentage — B2C mijozga qo'shiladigan foiz: noldan katta bo'lsa mijoz
// B2C, nol bo'lsa B2B. Maydon umuman topilmasa bo'sh satr qaytadi va bu
// qator ma'lumotga qo'shilmaydi (taxmin qilinmaydi).
func ClientType(o Order) string {
	raw := strings.TrimSpace(Pick(o, b2cKey, b2cPaths...))
	if raw == "" {
		return ""
	}
	pct, err := strconv.ParseFloat(raw, 64)
	if err != nil || pct <= 0 {
		return "B2B"
	}
	return "B2C"
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

// path — aniq yo'l bo'yicha qiymat. Nuqta bilan ajratilgan bo'laklar, kerak
// bo'lsa massiv indeksi bilan:
//
//	"address.receiver_name"
//	"skus[0].sku_info.B2C_percentage"
func path(m Order, dotted string) string {
	var cur any = map[string]any(m)
	for _, part := range strings.Split(dotted, ".") {
		name, idx := splitIndex(part)
		if name != "" {
			obj, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur, ok = obj[name]
			if !ok {
				return ""
			}
		}
		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return ""
			}
			cur = arr[idx]
		}
	}
	return toStr(named(cur))
}

// splitIndex — "skus[0]" ni "skus" va 0 ga ajratadi. Indeks bo'lmasa -1.
func splitIndex(part string) (string, int) {
	open := strings.IndexByte(part, '[')
	if open < 0 || !strings.HasSuffix(part, "]") {
		return part, -1
	}
	idx, err := strconv.Atoi(part[open+1 : len(part)-1])
	if err != nil || idx < 0 {
		return part, -1
	}
	return part[:open], idx
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
