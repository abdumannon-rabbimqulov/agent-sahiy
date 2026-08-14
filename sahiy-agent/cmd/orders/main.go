// Command orders — adminka daigou-orders ro'yxatini o'qib, terminalga chiqaradi.
//
// Ishlatish:
//
//	go run ./cmd/orders -user_id 7988331 -status 6
//	go run ./cmd/orders -order_sn DG60605678
//	go run ./cmd/orders -raw            // xom JSON javobni ko'rsatadi
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.sahiy.uz"

// statusText — buyurtmaning raqamli holati uchun izoh.
var statusText = map[string]string{
	"6": "Tranzaksiya yakunlangan",
}

type params struct {
	page       string
	size       string
	status     string
	keyword    string
	platform   string
	userID     string
	orderSN    string
	expressNum string
	beginDate  string
	endDate    string
}

func main() {
	var p params
	flag.StringVar(&p.page, "page", "", "sahifa raqami (bo'sh bo'lsa hamma sahifa o'qiladi)")
	flag.StringVar(&p.size, "size", "20", "sahifadagi buyurtmalar soni")
	flag.StringVar(&p.status, "status", "", "buyurtma holati (masalan 6)")
	flag.StringVar(&p.keyword, "keyword", "", "qidiruv so'zi")
	flag.StringVar(&p.platform, "platform", "", "platforma")
	flag.StringVar(&p.userID, "user_id", "", "ilova profil ID")
	flag.StringVar(&p.orderSN, "order_sn", "", "asosiy buyurtma raqami")
	flag.StringVar(&p.expressNum, "express_num", "", "xitoydagi trek raqami")
	flag.StringVar(&p.beginDate, "begin_date", "", "boshlanish sanasi (YYYY-MM-DD)")
	flag.StringVar(&p.endDate, "end_date", "", "tugash sanasi (YYYY-MM-DD)")
	raw := flag.Bool("raw", false, "xom JSON javobni chiqarish")
	envPath := flag.String("env", ".env", ".env fayl yo'li")
	flag.Parse()

	token := firstNonEmpty(os.Getenv("ADMINKA_TOKEN_BAARER"), os.Getenv("ADMINKA_TOKEN_BEARER"))
	baseURL := os.Getenv("USER_BASE_URL")
	if token == "" || baseURL == "" {
		env, path := loadEnvFile(*envPath)
		if token == "" {
			token = firstNonEmpty(env["ADMINKA_TOKEN_BAARER"], env["ADMINKA_TOKEN_BEARER"])
		}
		if baseURL == "" {
			baseURL = env["USER_BASE_URL"]
		}
		if token != "" && path != "" {
			fmt.Fprintln(os.Stderr, ".env o'qildi:", path)
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "xato: ADMINKA_TOKEN_BAARER topilmadi (env yoki .env)")
		fmt.Fprintln(os.Stderr, "maslahat: .env yo'lini ko'rsating -> -env /Users/user/Sahiy/sahiy-agent/.env")
		os.Exit(1)
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Hech qanday qidiruv qiymati berilmagan bo'lsa — terminaldan so'raymiz.
	if p.userID == "" && p.orderSN == "" && p.expressNum == "" && p.keyword == "" {
		kind, value, err := askQuery()
		if err != nil {
			fmt.Fprintln(os.Stderr, "xato:", err)
			os.Exit(1)
		}
		switch kind {
		case "user_id":
			p.userID = value
		case "order_sn":
			p.orderSN = value
		case "express_num":
			p.expressNum = value
		}
		fmt.Printf("Qidirilmoqda: %s = %s\n", kind, value)
	}

	if *raw {
		if p.page == "" {
			p.page = "1"
		}
		body, err := fetch(baseURL, token, p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "xato:", err)
			os.Exit(1)
		}
		var pretty any
		if json.Unmarshal(body, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(out))
			return
		}
		fmt.Println(string(body))
		return
	}

	orders, err := fetchAll(baseURL, token, p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xato:", err)
		os.Exit(1)
	}
	if len(orders) == 0 {
		fmt.Println("Buyurtma topilmadi.")
		return
	}

	fmt.Printf("\nJami %d ta buyurtma topildi\n", len(orders))
	for i, o := range orders {
		printOrder(i+1, o)
	}
}

// askQuery — foydalanuvchidan bitta qiymat so'raydi va uning turini aniqlaydi.
func askQuery() (kind, value string, err error) {
	fmt.Println("Qidiruv qiymatini kiriting:")
	fmt.Println("  - Profil ID       (masalan 7988331)")
	fmt.Println("  - Buyurtma raqami (masalan DG60605678)")
	fmt.Println("  - Trek raqami     (masalan 435294684627493)")
	fmt.Print("> ")

	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		if e := sc.Err(); e != nil {
			return "", "", e
		}
		return "", "", fmt.Errorf("qiymat kiritilmadi")
	}
	value = strings.TrimSpace(sc.Text())
	if value == "" {
		return "", "", fmt.Errorf("qiymat kiritilmadi")
	}
	return detectKind(value), value, nil
}

// detectKind — kiritilgan qiymat qaysi maydonga tegishli ekanini aniqlaydi.
//
//	DG... yoki harf bilan boshlansa       -> order_sn
//	faqat raqam va uzunligi <= 10         -> user_id
//	qolgan hollarda (uzun raqam, YT...)   -> express_num
func detectKind(v string) string {
	up := strings.ToUpper(v)
	switch {
	case strings.HasPrefix(up, "DG"):
		return "order_sn"
	case isDigits(v) && len(v) <= 10:
		return "user_id"
	default:
		return "express_num"
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// fetchAll — sahifalarni oxirigacha aylanib, barcha buyurtmalarni yig'adi.
// -page berilgan bo'lsa faqat o'sha sahifa o'qiladi.
func fetchAll(baseURL, token string, p params) ([]map[string]any, error) {
	single := p.page != ""
	if p.page == "" {
		p.page = "1"
	}

	var all []map[string]any
	for page := 1; ; page++ {
		if !single {
			p.page = strconv.Itoa(page)
		}
		body, err := fetch(baseURL, token, p)
		if err != nil {
			return nil, err
		}
		var payload any
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("javobni JSON sifatida o'qib bo'lmadi: %w\n%s", err, trim(body, 500))
		}
		orders := findOrders(payload)
		all = append(all, orders...)

		last, total := pageInfo(payload)
		if !single && last > 1 {
			fmt.Printf("sahifa %d/%d o'qildi (%d/%d)\n", page, last, len(all), total)
		}
		if single || len(orders) == 0 || page >= last {
			return all, nil
		}
	}
}

// pageInfo — meta ichidan last_page va total ni oladi.
func pageInfo(payload any) (last, total int) {
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

func fetch(baseURL, token string, p params) ([]byte, error) {
	q := url.Values{}
	q.Set("page", p.page)
	q.Set("size", p.size)
	q.Set("status", p.status)
	q.Set("keyword", p.keyword)
	q.Set("platform", p.platform)
	q.Set("user_id", p.userID)
	q.Set("order_sn", p.orderSN)
	q.Set("express_num", p.expressNum)
	q.Set("begin_date", p.beginDate)
	q.Set("end_date", p.endDate)

	endpoint := strings.TrimRight(baseURL, "/") + "/api/admin/daigou-orders?" + q.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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

// findOrders — javob ichidan buyurtmalar ro'yxatini topadi
// (data, data.data, data.list, data.rows, data.items va h.k.).
func findOrders(v any) []map[string]any {
	switch t := v.(type) {
	case []any:
		var out []map[string]any
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
				if got := findOrders(child); got != nil {
					return got
				}
			}
		}
	}
	return nil
}

func isOrder(m map[string]any) bool {
	for _, key := range []string{"order_sn", "receiver_name", "express_num", "status"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func printOrder(n int, o map[string]any) {
	sn := pick(o, "order_sn", "order_sn")
	fmt.Printf("\n─── #%d  %s ───\n", n, dash(sn))

	st := pick(o, "status", "status")
	if txt, ok := statusText[st]; ok {
		st = fmt.Sprintf("%s (%s)", st, txt)
	}

	rows := [][2]string{
		{"Profil ID (user_id)", pick(o, "user_id", "user_id", "user.id")},
		{"Buyurtma raqami (order_sn)", sn},
		{"Holat (status)", st},
		{"Umumiy summa (amount)", pick(o, "amount", "amount") + " " + pick(o, "currency", "currency")},
		{"Qabul qiluvchi (receiver_name)", pick(o, "receiver_name", "address.receiver_name")},
		{"Viloyat (province)", pick(o, "province", "address.province")},
		{"Hudud (area)", pick(o, "area", "address.area")},
		{"Shahar/tuman (sub_area)", pick(o, "sub_area", "address.sub_area")},
		{"Manzil (street)", pick(o, "street", "address.street", "address.address")},
		{"Yetkazish yo'nalishi (express_line)", pick(o, "express_line", "express_line")},
		{"Trek raqami (express_num)", pick(o, "express_num", "express.express_num")},
		{"Posilka nomi (package_name)", pick(o, "package_name", "express.package.package_name")},
		{"Soni (quantity)", pick(o, "quantity", "express.package.qty")},
		{"To'lov vaqti (paid_at)", pick(o, "paid_at", "paid_at", "created_at")},
		{"Buyurtma yaratilgan (created_at)", pick(o, "created_at", "created_at")},
		{"Yo'lga chiqqan (shipped_at)", pick(o, "shipped_at", "express.package.order.shipped_at")},
		{"Qadoqlangan (packed_at)", pick(o, "packed_at", "express.package.order.packed_at")},
		{"Omborga kirgan (in_storage_at)", pick(o, "in_storage_at", "express.package.in_storage_at")},
	}
	for _, r := range rows {
		fmt.Printf("  %-36s : %s\n", r[0], dash(r[1]))
	}
}

// str — qiymatni qaysi turda bo'lishidan qat'i nazar matnga aylantiradi.
// Kalit yuqori darajada bo'lmasa, ichma-ich obyekt/ro'yxatlardan ham qidiradi.
func str(m map[string]any, key string) string {
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

// path — "address.receiver_name" ko'rinishidagi aniq yo'l bo'yicha qiymat oladi.
func path(m map[string]any, dotted string) string {
	var cur any = m
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

// pick — berilgan yo'llarni tartib bilan sinaydi, topilmasa kalit bo'yicha
// chuqur qidiruvga o'tadi.
func pick(m map[string]any, key string, paths ...string) string {
	for _, p := range paths {
		if s := path(m, p); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return str(m, key)
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

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func trim(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append(b[:n:n], []byte("...")...)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// loadEnvFile — .env ni topadi: berilgan yo'l, so'ng joriy papkadan yuqoriga
// qarab har bir papkada ".env" va "sahiy-agent/.env" tekshiriladi.
func loadEnvFile(explicit string) (map[string]string, string) {
	var candidates []string
	if explicit != "" && explicit != ".env" {
		candidates = append(candidates, explicit)
	}
	// Manba fayl joylashgan papka (go run /path/.../main.go holati uchun).
	if _, file, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(file)
		for i := 0; i < 4; i++ {
			candidates = append(candidates, filepath.Join(dir, ".env"))
			dir = filepath.Dir(dir)
		}
	}
	if dir, err := os.Getwd(); err == nil {
		for {
			candidates = append(candidates,
				filepath.Join(dir, ".env"),
				filepath.Join(dir, "sahiy-agent", ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if env := loadEnv(c); len(env) > 0 {
			return env, c
		}
	}
	return map[string]string{}, ""
}

func loadEnv(path string) map[string]string {
	env := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return env
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		env[strings.TrimSpace(k)] = v
	}
	return env
}
