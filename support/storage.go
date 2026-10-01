// Fayl (rasm) yuklash: xodim guruhda yuborgan rasmni mijozga yetkazish
// uchun avval support serverining omboriga qo'yiladi.
//
// Telegram fayli to'g'ridan-to'g'ri mijozga berilmaydi: uning havolasi
// ichida bot tokeni turadi (api.telegram.org/file/bot<TOKEN>/…) va u
// mijozga ko'rinib qolardi.
package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// StorageUploadPath - support serveridagi fayl yuklash endpointi.
// Javobi: {"data":"https://storage…/market.images/<id>-<nom>"}.
const StorageUploadPath = "/api/v1/storage.upload"

// MaxUploadBytes - yuklanadigan faylning eng katta hajmi (8 MB).
// Telegram siqilgan rasmi bundan ancha kichik; chegara faqat kutilmagan
// katta fayl xotirani yeb qo'ymasligi uchun.
const MaxUploadBytes = 8 << 20

// UploadFile faylni support omboriga qo'yadi va ochiq havolasini
// qaytaradi.
func UploadFile(baseURL, token, name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("bo'sh fayl")
	}
	if len(data) > MaxUploadBytes {
		return "", fmt.Errorf("fayl juda katta (%d bayt)", len(data))
	}
	if strings.TrimSpace(name) == "" {
		name = "image.jpg"
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", path.Base(name))
	if err != nil {
		return "", fmt.Errorf("multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("multipart yozish: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("multipart yopish: %w", err)
	}

	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(base, "/")+StorageUploadPath, &body)
	if err != nil {
		return "", fmt.Errorf("so'rov yaratish: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("fayl yuklash: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fayl yuklanmadi (status %d): %s", resp.StatusCode, snippet(raw))
	}

	var out struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || strings.TrimSpace(out.Data) == "" {
		return "", fmt.Errorf("javobdan havola o'qilmadi: %s", snippet(raw))
	}
	return strings.TrimSpace(out.Data), nil
}

// UploadToStorage - UploadFile, token bilan (eskirgan bo'lsa yangilanadi).
func UploadToStorage(name string, data []byte) (string, error) {
	return withToken(func(baseURL, token string) (string, error) {
		return UploadFile(baseURL, token, name, data)
	})
}

// TelegramFile - Telegram serveridan faylni yuklab oladi (getFile →
// file_path → yuklab olish). Qaytadigan nom kengaytmasi bilan.
func TelegramFile(fileID string) (name string, data []byte, err error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return "", nil, fmt.Errorf("TELEGRAM_BOT_TOKEN berilmagan")
	}
	if strings.TrimSpace(fileID) == "" {
		return "", nil, fmt.Errorf("file_id berilmagan")
	}

	cl := &http.Client{Timeout: 60 * time.Second}
	url := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", TelegramAPI(), token, fileID)
	resp, err := cl.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("getFile: %w", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("getFile (status %d): %s", resp.StatusCode, snippet(raw))
	}
	var meta struct {
		Result struct {
			FilePath string `json:"file_path"`
			FileSize int64  `json:"file_size"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.Result.FilePath == "" {
		return "", nil, fmt.Errorf("getFile javobi o'qilmadi: %s", snippet(raw))
	}
	if meta.Result.FileSize > MaxUploadBytes {
		return "", nil, fmt.Errorf("fayl juda katta (%d bayt)", meta.Result.FileSize)
	}

	dl := fmt.Sprintf("%s/file/bot%s/%s", TelegramAPI(), token, meta.Result.FilePath)
	fresp, err := cl.Get(dl)
	if err != nil {
		return "", nil, fmt.Errorf("faylni olish: %w", err)
	}
	defer fresp.Body.Close()
	if fresp.StatusCode < 200 || fresp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("faylni olish (status %d)", fresp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(fresp.Body, MaxUploadBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("faylni o'qish: %w", err)
	}
	if len(data) > MaxUploadBytes {
		return "", nil, fmt.Errorf("fayl juda katta")
	}
	return path.Base(meta.Result.FilePath), data, nil
}
