package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sahiy-agent/internal/daigou"
)

func main() {
	var p daigou.Params
	flag.StringVar(&p.Page, "page", "", "sahifa raqami (bo'sh bo'lsa hamma sahifa o'qiladi)")
	flag.StringVar(&p.Size, "size", "20", "sahifadagi buyurtmalar soni")
	flag.StringVar(&p.Status, "status", "", "buyurtma holati (masalan 6)")
	flag.StringVar(&p.Keyword, "keyword", "", "qidiruv so'zi")
	flag.StringVar(&p.Platform, "platform", "", "platforma")
	flag.StringVar(&p.UserID, "user_id", "", "ilova profil ID")
	flag.StringVar(&p.OrderSN, "order_sn", "", "asosiy buyurtma raqami")
	flag.StringVar(&p.ExpressNum, "express_num", "", "xitoydagi trek raqami")
	flag.StringVar(&p.BeginDate, "begin_date", "", "boshlanish sanasi (YYYY-MM-DD)")
	flag.StringVar(&p.EndDate, "end_date", "", "tugash sanasi (YYYY-MM-DD)")
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
		fmt.Fprintln(os.Stderr, "xato: ADMINKA_TOKEN_BEARER topilmadi (env yoki .env)")
		fmt.Fprintln(os.Stderr, "maslahat: .env yo'lini ko'rsating -> -env /Users/user/Sahiy/sahiy-agent/.env")
		os.Exit(1)
	}
	c := daigou.New(baseURL, token)

	// Hech qanday qidiruv qiymati berilmagan bo'lsa — terminaldan so'raymiz.
	if p.UserID == "" && p.OrderSN == "" && p.ExpressNum == "" && p.Keyword == "" {
		kind, value, err := askQuery()
		if err != nil {
			fmt.Fprintln(os.Stderr, "xato:", err)
			os.Exit(1)
		}
		switch kind {
		case "user_id":
			p.UserID = value
		case "order_sn":
			p.OrderSN = value
		case "express_num":
			p.ExpressNum = value
		}
		fmt.Printf("Qidirilmoqda: %s = %s\n", kind, value)
	}

	if *raw {
		if p.Page == "" {
			p.Page = "1"
		}
		body, err := c.Fetch(p)
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

	orders, err := c.Search(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xato:", err)
		os.Exit(1)
	}
	if len(orders) == 0 {
		fmt.Println("Buyurtma topilmadi.")
		return
	}

	fmt.Printf("\nJami %d ta buyurtma topildi\n", len(orders))
	fmt.Println(daigou.Summary(orders))
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
