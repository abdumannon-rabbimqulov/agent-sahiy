package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"sahiy-agent/internal/daigou"
	"sahiy-agent/internal/envfile"
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

	token := envfile.FirstNonEmpty(os.Getenv("ADMINKA_TOKEN_BEARER"))
	baseURL := os.Getenv("USER_BASE_URL")
	if token == "" || baseURL == "" {
		env, path := envfile.Find(*envPath)
		if token == "" {
			token = envfile.FirstNonEmpty(env["ADMINKA_TOKEN_BEARER"])
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
