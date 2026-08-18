package service

import (
	"fmt"
	"io"
	"net/http"
)

// maxBody — javobning o'qiladigan eng katta hajmi (himoya uchun).
const maxBody = 2 << 20 // 2 MB

func readAll(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}
	return data, nil
}
