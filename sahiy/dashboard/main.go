// sahiy/dashboard — yetkazma (delivery) buyurtmalari bo'yicha agent QANDAY
// ma'lumot olayotganini ko'rsatadi: qaysi so'rovlar ketadi, server nima
// qaytaradi va shulardan modelga AYNAN nima yetib boradi.
//
// Faqat O'QIYDI (GET so'rovlar), hech narsa o'zgartirmaydi.
//
// Ishlatish:
//
//	go run ./sahiy/dashboard -track YT7635822034113
//	go run ./sahiy/dashboard -client 8162004
//	go run ./sahiy/dashboard -text "buyurtmam qachon keladi? YT7635822034113"
//	go run ./sahiy/dashboard -track YT7635822034113 -raw
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sahiy-agent/internal/envfile"
	"sahiy-agent/internal/orders"
	"sahiy-agent/internal/service"
	"sahiy-agent/internal/sources"
)

func main() {
	track := flag.String("track", "", "track / express raqami")
	clientID := flag.Int64("client", 0, "mijoz id (support'dagi client_id = delivery'dagi user_id)")
	text := flag.String("text", "", "mijoz xabari — undan qaysi raqamlar ajratilishini ko'rsatadi")
	raw := flag.Bool("raw", false, "xom JSON javobni chiqarish")
	envPath := flag.String("env", ".env", ".env fayl yo'li")
	flag.Parse()

	if *track == "" && *clientID == 0 && *text == "" {
		fmt.Fprintln(os.Stderr, "xato: -track, -client yoki -text bering")
		flag.Usage()
		os.Exit(1)
	}

	env := loadEnv(*envPath)
	svc := newService(env)
	if !svc.Enabled() {
		fail(fmt.Errorf("SERVICE_PHONE / SERVICE_PASSWORD topilmadi (env yoki .env)"))
	}
	lookup := orders.New(svc)
	// Qidiruvning o'zi agent va /api/source/delivery bilan bir xil yo'ldan
	// ketsin — shu paket ikkalasiga ham xizmat qiladi.
	src := &sources.Sources{Orders: lookup}

	// Mijoz xabaridan raqamlarni ajratish — agent ham shu funksiyani
	// ishlatadi (main.go: orderNumbers → orders.Tracks).
	if *text != "" {
		nums := orders.Tracks(*text)
		head("MATNDAN AJRATILGAN RAQAMLAR")
		fmt.Printf("Xabar: %s\n", *text)
		if len(nums) == 0 {
			fmt.Println("Raqam topilmadi — agent mijozning barcha buyurtmalarini ko'radi (ByUser).")
		} else {
			fmt.Printf("Topildi: %s\n", strings.Join(nums, ", "))
			fmt.Println("(telefon raqamiga o'xshashlari va 3 tadan ortig'i tashlanadi)")
		}
		if *track == "" && len(nums) > 0 {
			*track = nums[0]
			fmt.Printf("\nBirinchi raqam bo'yicha qidiriladi: %s\n", *track)
		}
	}

	if *track != "" {
		lookupOne(svc, src, orders.TrackParam, *track, *raw,
			sources.Query{Track: *track})
	}
	if *clientID != 0 {
		id := strconv.FormatInt(*clientID, 10)
		lookupOne(svc, src, orders.UserParam, id, *raw,
			sources.Query{ClientID: *clientID})
	}
}

// lookupOne — bitta qidiruv: avval ikkita xom so'rov ko'rsatiladi, so'ng
// paketning o'zi bergan natija va modelga ketadigan matn.
func lookupOne(svc *service.Client, src *sources.Sources, param, value string,
	raw bool, q sources.Query) {

	head(fmt.Sprintf("QIDIRUV: %s = %s", param, value))

	// delivered — majburiy filtr, shuning uchun har qidiruvda IKKITA so'rov.
	fmt.Println("Server majburiy `delivered` filtrini talab qiladi, shuning uchun")
	fmt.Println("har qidiruvga ikkita so'rov ketadi:")
	for _, delivered := range []bool{false, true} {
		path := orders.FilterPath(param, value, delivered)
		body, status, err := svc.Get(path)
		fmt.Printf("\n  GET %s\n", path)
		if err != nil {
			fmt.Printf("  → xato: %v\n", err)
			continue
		}
		fmt.Printf("  → status %d, javob %d bayt\n", status, len(body))
		if raw {
			fmt.Println(indent(pretty(body), "    "))
		}
	}

	b := src.Delivery(q)
	if b.Error != "" {
		fmt.Printf("\nQidiruv xatosi: %s\n", b.Error)
		return
	}
	list := b.Items

	head2("NATIJA")
	if len(list) == 0 {
		fmt.Println("Buyurtma topilmadi — bu holatda modelga buyurtma bloki umuman qo'shilmaydi.")
		return
	}
	fmt.Printf("Jami %d ta buyurtma (eng yangisi birinchi)\n", len(list))
	for i, o := range list {
		mark := " "
		if i >= 5 {
			mark = "×" // Summary faqat 5 tasini oladi
		}
		fmt.Printf(" %s #%-10d track=%-18s delivered=%-5v yaratilgan=%s\n",
			mark, o.OrderID, o.ExpressNum, o.Delivered, o.CreatedAt)
	}
	if len(list) > 5 {
		fmt.Println("\n× belgilanganlari modelga BORMAYDI (eng yangi 5 tasi olinadi)")
	}

	head2("AI KO'RADIGAN MATN")
	fmt.Println("Modelga xom JSON emas, aynan shu matn ketadi")
	fmt.Println("(/api/source/delivery javobidagi `summary` ham aynan shu):")
	fmt.Println(b.Summary)
}

// --- yordamchilar ---

// loadEnv — .env ni topadi va qiymatlarni qaytaradi. config.Load ishlatilmaydi:
// u LOGIN/PASSWORD bo'lmasa xato beradi, bu dastur esa faqat SERVICE_* ga
// bog'liq bo'lishi kerak.
func loadEnv(path string) map[string]string {
	env, found := envfile.Find(path)
	if found != "" {
		fmt.Fprintln(os.Stderr, ".env o'qildi:", found)
	}
	return env
}

// newService — main.go:195 dagi bilan bir xil qurilma; default qiymatlar
// ham config.go dagidek.
func newService(env map[string]string) *service.Client {
	get := func(key, def string) string {
		return envfile.FirstNonEmpty(os.Getenv(key), env[key], def)
	}
	apk, _ := strconv.Atoi(get("SERVICE_APK_TYPE", "2"))
	if apk == 0 {
		apk = 2
	}
	dataDir := get("DATA_DIR", ".")
	return service.New(get("SERVICE_BASE_URL", "https://api.sahiy.uz"), service.LoginRequest{
		Phone:      get("SERVICE_PHONE", ""),
		Password:   get("SERVICE_PASSWORD", ""),
		APKType:    apk,
		DeviceID:   get("SERVICE_DEVICE_ID", ""),
		DeviceName: get("SERVICE_DEVICE_NAME", "phone"),
		DeviceType: get("SERVICE_DEVICE_TYPE", "mobile"),
		FcmToken:   get("SERVICE_FCM_TOKEN", ""),
	}, filepath.Join(dataDir, "service-token.json"))
}

func pretty(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return string(body)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(body)
	}
	return string(out)
}

func indent(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func head(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("═", len([]rune(title))))
}

func head2(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("─", len([]rune(title))))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "xato:", err)
	os.Exit(1)
}
