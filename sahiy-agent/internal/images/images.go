// Package images — chat rasmlarini yuklab olish va diskda saqlash.
//
// Rasm URL'i mijoz xabaridan keladi, ya'ni ISHONCHSIZ manba. Shuning uchun
// faqat ma'lum host'ga ruxsat beriladi (aks holda mijoz ichki manzil yozib
// qo'ysa server o'sha yerga so'rov yuborardi — SSRF), kengaytma esa
// Content-Type'dan olinadi (URL yo'lidagi "../" ta'sir qilmasligi uchun).
package images

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AllowedHosts — rasm yuklab olish mumkin bo'lgan hostlar.
var AllowedHosts = map[string]bool{
	"storage.abusahiy.uz": true,
}

// MaxSize — bitta rasmning eng katta hajmi.
const MaxSize = 10 << 20 // 10 MB

// extByMime — Content-Type'dan fayl kengaytmasi.
var extByMime = map[string]string{
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/heic": ".heic",
}

// Result — yuklab olingan rasm haqida ma'lumot.
type Result struct {
	Path string // diskdagi to'liq yo'l
	Mime string
	Size int64
	Data []byte // tahlil (Gemini) uchun
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Download rasmni yuklab olib, dir/<conversationID>/<messageID>.<ext> ga
// saqlaydi va baytlarini ham qaytaradi (darhol tahlil qilish uchun).
func Download(rawURL, dir string, conversationID, messageID int64) (*Result, error) {
	if err := checkURL(rawURL); err != nil {
		return nil, err
	}

	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("rasm so'rovi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rasm yuklanmadi (status %d)", resp.StatusCode)
	}

	mime := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	ext, ok := extByMime[mime]
	if !ok {
		return nil, fmt.Errorf("qo'llab-quvvatlanmaydigan tur: %q", mime)
	}

	// LimitReader — MaxSize+1 o'qib, oshib ketganini aniqlaymiz.
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("rasmni o'qish: %w", err)
	}
	if int64(len(data)) > MaxSize {
		return nil, fmt.Errorf("rasm juda katta (%d MB dan oshdi)", MaxSize>>20)
	}

	// Yo'l faqat bizning raqamlarimizdan quriladi — URL'dan emas.
	folder := filepath.Join(dir, fmt.Sprint(conversationID))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, fmt.Errorf("katalog yaratish: %w", err)
	}
	path := filepath.Join(folder, fmt.Sprintf("%d%s", messageID, ext))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("rasmni saqlash: %w", err)
	}

	return &Result{Path: path, Mime: mime, Size: int64(len(data)), Data: data}, nil
}

// checkURL manzil ruxsat etilgan hostda va https ekanini tekshiradi.
func checkURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("noto'g'ri URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("qo'llab-quvvatlanmaydigan sxema: %q", u.Scheme)
	}
	if !AllowedHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("ruxsat etilmagan host: %q", u.Hostname())
	}
	return nil
}

// Read saqlangan rasmni diskdan o'qiydi (kesh'dan tahlil uchun kerak bo'lsa).
func Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}
