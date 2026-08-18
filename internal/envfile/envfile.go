// Package envfile — .env faylini topish va o'qish. Alohida ishga
// tushiriladigan dasturlar (cmd/orders, sahiy/chat, sahiy/dashboard)
// odatda repo ichidan emas, ixtiyoriy papkadan chaqiriladi, shuning uchun
// fayl yuqoriga qarab qidiriladi.
//
// internal/config dagi loadDotEnv'dan farqi: u bitta yo'lni oladi va
// qiymatlarni os.Setenv qiladi; bu yerda esa qidiruv bor va natija oddiy
// map bo'lib qaytadi.
package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Find — .env ni topadi: avval berilgan yo'l, so'ng shu manba fayl
// joylashgan papkadan va joriy papkadan yuqoriga qarab har bir papkada
// ".env" va "sahiy-agent/.env" tekshiriladi.
//
// Topilmasa bo'sh map va bo'sh yo'l qaytadi (xato emas — qiymatlar
// jarayon muhitidan kelayotgan bo'lishi mumkin).
func Find(explicit string) (map[string]string, string) {
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
		if env := Read(c); len(env) > 0 {
			return env, c
		}
	}
	return map[string]string{}, ""
}

// Read — bitta faylni o'qiydi. Ochilmasa bo'sh map (dasturlar .env'siz
// ham, faqat muhit o'zgaruvchilari bilan ishlashi kerak).
func Read(path string) map[string]string {
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

// FirstNonEmpty — birinchi bo'sh bo'lmagan qiymat.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
