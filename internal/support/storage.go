package support

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"sahiy-agent/internal/client"
)

const uploadPath = "/api/v1/storage.upload"

// UploadFile faylni FormData (field nomi "file") sifatida yuklaydi.
// bucket bo'sh bo'lsa "client-chat-images" ishlatiladi.
// Serverning xom javobini qaytaradi (odatda yuklangan fayl URL/kaliti).
func UploadFile(c *client.Client, bucket, filePath string) ([]byte, error) {
	if bucket == "" {
		bucket = "client-chat-images"
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("faylni ochish: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("faylni nusxalash: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("writer yopish: %w", err)
	}

	path := fmt.Sprintf("%s?bucket=%s", uploadPath, bucket)
	body, status, err := c.DoRaw(http.MethodPost, path, w.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("yuklash muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return body, nil
}

// UploadBytes xotiradagi baytlarni (masalan Gemini yaratgan rasm) yuklaydi.
func UploadBytes(c *client.Client, bucket, filename string, data []byte) ([]byte, error) {
	if bucket == "" {
		bucket = "client-chat-images"
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return nil, fmt.Errorf("yozish: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("writer yopish: %w", err)
	}

	path := fmt.Sprintf("%s?bucket=%s", uploadPath, bucket)
	body, status, err := c.DoRaw(http.MethodPost, path, w.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("yuklash muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return body, nil
}
