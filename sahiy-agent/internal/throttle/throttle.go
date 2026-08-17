// Package throttle — oddiy oyna-hisoblagich: berilgan vaqt oynasida
// eng ko'pi N ta ishga ruxsat beradi.
//
// Nima uchun kerak: agent suhbatlarni tsikl bo'yicha ketma-ket qayta
// ishlaydi va hech qanday tezlik chegarasi yo'q edi — o'lchov bo'yicha
// eng band daqiqada 26 ta suhbat o'tgan. AI mijoz muammosini qanday
// tushunayotganini qo'lda ko'rib chiqish uchun bu juda tez.
package throttle

import (
	"fmt"
	"sync"
	"time"
)

// Limiter — "oynada N ta" chegarasi. Nol qiymatli Limiter ishlatilmaydi,
// New orqali yarating.
type Limiter struct {
	mu     sync.Mutex
	n      int           // oynada ruxsat etilgan ish soni (0 — chegarasiz)
	window time.Duration // oyna uzunligi
	seen   []time.Time   // oyna ichidagi ishlar vaqti (eskisidan yangisiga)

	// now — vaqt manbai. Testda almashtiriladi, shuning uchun testlar
	// haqiqiy soatni kutmaydi.
	now func() time.Time
}

// New — oynada n ta ishga ruxsat beruvchi chegara. n yoki window nolga
// teng bo'lsa chegara o'chiq (Allow doim true) — .env da o'chirib qo'yish
// shu orqali ishlaydi.
func New(n int, window time.Duration) *Limiter {
	return &Limiter{n: n, window: window, now: time.Now}
}

// Off — chegara o'chiqmi.
func (l *Limiter) Off() bool { return l == nil || l.n <= 0 || l.window <= 0 }

// Allow — joy bo'lsa true qaytaradi va ishni hisobga oladi. Joy bo'lmasa
// false: chaqiruvchi ishni keyingi safarga qoldiradi (kutib turmaydi).
func (l *Limiter) Allow() bool {
	if l.Off() {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.trim(now)
	if len(l.seen) >= l.n {
		return false
	}
	l.seen = append(l.seen, now)
	return true
}

// Left — oynada yana nechta ishga joy bor.
func (l *Limiter) Left() int {
	if l.Off() {
		return -1 // chegarasiz
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.trim(l.now())
	return l.n - len(l.seen)
}

// Until — keyingi joy qachon bo'shashi (joy bor bo'lsa 0). Log uchun:
// "chegara to'ldi, 42s dan keyin davom etadi".
func (l *Limiter) Until() time.Duration {
	if l.Off() {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.trim(now)
	if len(l.seen) < l.n {
		return 0
	}
	// Eng eski yozuv oynadan chiqqanda joy bo'shaydi.
	if d := l.window - now.Sub(l.seen[0]); d > 0 {
		return d
	}
	return 0
}

// String — sozlamani odam o'qiydigan ko'rinishda ("2m0s da 5 ta").
func (l *Limiter) String() string {
	if l.Off() {
		return "o'chiq"
	}
	return fmt.Sprintf("%s da %d ta", l.window, l.n)
}

// trim — oynadan chiqib ketgan yozuvlarni tashlaydi. mu ushlab turilgan
// holda chaqiriladi.
func (l *Limiter) trim(now time.Time) {
	cut := now.Add(-l.window)
	i := 0
	for i < len(l.seen) && !l.seen[i].After(cut) {
		i++
	}
	l.seen = l.seen[i:]
}
