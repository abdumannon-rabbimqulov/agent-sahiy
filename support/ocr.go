// Rasmni MODELSIZ o'qish: tesseract OCR kutubxonasi.
//
// Buyurtma (DG…) va trek raqamlari — bosma, aniq shriftdagi lotin harf
// va raqamlar. Bunday matnni o'qish uchun katta ko'ruvchi model shart
// emas: tesseract buni bepul, lokal va bir soniyada qiladi. Model
// faqat OCR hech narsa topmaganda ishga tushadi (image_numbers.go).
//
// Nega tashqi buyruq, kutubxona emas: gosseract kabi bog'lovchilar cgo
// talab qiladi va build'ni og'irlashtiradi (hozir CGO_ENABLED=0). Bu
// yerda tesseract'ning o'zi ishlatiladi — Docker image'ga bitta paket
// qo'shiladi, kod esa toza Go bo'lib qoladi.
package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultOCRLangs - tesseract tillari. Raqam va lotin harflari uchun
// "eng" yetarli; rus tilidagi matn ham raqamlarga xalaqit bermaydi.
const DefaultOCRLangs = "eng"

// MaxImageBytes - yuklab olinadigan rasm hajmi chegarasi (10 MB).
const MaxImageBytes = 10 << 20

// ErrNoOCR - tesseract o'rnatilmagan.
var ErrNoOCR = errors.New("tesseract topilmadi (OCR o'chirilgan)")

// OCREnabled - rasmni avval OCR bilan o'qishga urinamizmi (.env: OCR_ENABLED,
// default — ha). O'chirilsa rasm to'g'ridan-to'g'ri modelga ketadi.
func OCREnabled() bool { return envStr("OCR_ENABLED", "true") != "false" }

// OCRLangs - tesseract til paketlari (.env: OCR_LANGS).
func OCRLangs() string { return envStr("OCR_LANGS", DefaultOCRLangs) }

// ocrBin - tesseract fayli (.env: TESSERACT_BIN, default PATH dan).
func ocrBin() string { return envStr("TESSERACT_BIN", "tesseract") }

// OCRAvailable - tesseract shu mashinada bormi.
func OCRAvailable() bool {
	_, err := exec.LookPath(ocrBin())
	return err == nil
}

// ReadImageOCR - rasm havolasini yuklab olib, tesseract bilan matnini
// o'qiydi va undan buyurtma/trek raqamlarini ajratadi.
//
// Raqamlar `numbers.go` dagi bir xil qoidalar bilan ajratiladi: OCR
// matni ham mijoz yozgan matn kabi ishlanadi.
func ReadImageOCR(ctx context.Context, imageURL string) (ImageNumbers, error) {
	if !OCRAvailable() {
		return ImageNumbers{}, ErrNoOCR
	}
	path, err := downloadImage(ctx, imageURL)
	if err != nil {
		return ImageNumbers{}, err
	}
	defer os.Remove(path)

	text, err := runTesseract(ctx, path)
	if err != nil {
		return ImageNumbers{}, err
	}

	sn, ex := numbersFromText(text)
	return ImageNumbers{
		OrderSN: sn,
		Express: ex,
		Text:    trimText(strings.Join(strings.Fields(text), " "), 300),
		Images:  1,
		Links:   []string{imageURL},
		Raw:     strings.TrimSpace(text),
		Model:   "tesseract " + OCRLangs(),
	}, nil
}

// runTesseract - `tesseract <fayl> stdout` chaqiradi va matnni qaytaradi.
func runTesseract(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(envInt("OCR_TIMEOUT_SEC", 30))*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ocrBin(), path, "stdout", "-l", OCRLangs())
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %w: %s", err, snippet(errBuf.Bytes()))
	}
	return out.String(), nil
}

// downloadImage - rasmni vaqtincha faylga yuklab oladi. Fayl nomi
// kengaytmasi saqlanadi: tesseract turni shundan ham aniqlaydi.
func downloadImage(ctx context.Context, imageURL string) (string, error) {
	if !isImageLink(imageURL) {
		return "", fmt.Errorf("rasm havolasi emas: %s", imageURL)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(envInt("OCR_FETCH_TIMEOUT_SEC", 30))*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("so'rov yaratish: %w", err)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("rasmni yuklash: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("rasmni yuklash (status %d)", resp.StatusCode)
	}

	ext := strings.ToLower(filepath.Ext(strings.SplitN(imageURL, "?", 2)[0]))
	if ext == "" {
		ext = ".jpg"
	}
	f, err := os.CreateTemp("", "sahiy-rasm-*"+ext)
	if err != nil {
		return "", fmt.Errorf("vaqtincha fayl: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, io.LimitReader(resp.Body, MaxImageBytes)); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("rasmni saqlash: %w", err)
	}
	return f.Name(), nil
}
