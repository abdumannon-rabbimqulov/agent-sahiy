// Package prompts — promptlarni saqlash, tekshirish va xotirada keshlash.
//
// Qatlamlar (har biri faqat pastdagisiga bog'lanadi):
//
//	http.go       — HTTP transport (DTO, status kodlari)
//	service.go    — biznes qoidalari + kesh boshqaruvi
//	repository.go — sof SQL (GORM)
//	cache.go      — o'qish uchun atomik xotira keshi
package prompts

import (
	"errors"
	"net/http"
)

// Qatlamlararo xato turlari. Repository va Service shu xatolarni %w bilan
// o'raydi, transport esa ularni HTTP statusiga aylantiradi — ya'ni service
// net/http haqida hech narsa bilmaydi.
var (
	// ErrNotFound — bunday kalitli prompt yo'q.
	ErrNotFound = errors.New("prompt topilmadi")
	// ErrConflict — qoidaga zid amal (kalit band, majburiy promptni o'chirish).
	ErrConflict = errors.New("amal bajarilmadi")
	// ErrInvalid — kiritilgan ma'lumot noto'g'ri (bo'sh kalit yoki matn).
	ErrInvalid = errors.New("noto'g'ri ma'lumot")
)

// HTTPStatus xatoga mos HTTP status kodini qaytaradi.
func HTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
