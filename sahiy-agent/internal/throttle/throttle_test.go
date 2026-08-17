package throttle

import (
	"testing"
	"time"
)

// clock — testda vaqtni qo'lda suramiz, shuning uchun test tez va barqaror.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newTest(n int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)}
	l := New(n, window)
	l.now = c.now
	return l, c
}

func TestAllowWithinWindow(t *testing.T) {
	l, c := newTest(5, 2*time.Minute)

	for i := 1; i <= 5; i++ {
		if !l.Allow() {
			t.Fatalf("%d-chi ish rad etildi, oynada joy bor edi", i)
		}
	}
	if l.Allow() {
		t.Fatal("6-chi ish o'tib ketdi — chegara ishlamadi")
	}
	if got := l.Left(); got != 0 {
		t.Errorf("Left()=%d, 0 kutilgandi", got)
	}

	// Oyna to'lgan: eng eski yozuv chiqquncha kutish kerak.
	if d := l.Until(); d != 2*time.Minute {
		t.Errorf("Until()=%s, 2m kutilgandi", d)
	}

	// Yarim oyna o'tdi — hali ham joy yo'q.
	c.add(time.Minute)
	if l.Allow() {
		t.Error("oyna hali to'lmagan, ish o'tmasligi kerak edi")
	}
	if d := l.Until(); d != time.Minute {
		t.Errorf("Until()=%s, 1m kutilgandi", d)
	}

	// Oyna to'liq o'tdi — hammasi bo'shaydi.
	c.add(time.Minute + time.Second)
	if got := l.Left(); got != 5 {
		t.Errorf("oyna o'tgach Left()=%d, 5 kutilgandi", got)
	}
	if !l.Allow() {
		t.Error("oyna o'tgach ish o'tishi kerak edi")
	}
}

// TestSlidingWindow — oyna surilib boradi: yozuvlar birdan emas, birma-bir
// bo'shaydi.
func TestSlidingWindow(t *testing.T) {
	l, c := newTest(2, time.Minute)

	l.Allow() // 12:00:00
	c.add(30 * time.Second)
	l.Allow() // 12:00:30
	if l.Allow() {
		t.Fatal("uchinchi ish o'tdi")
	}

	// 12:01:01 — birinchi yozuv oynadan chiqdi, bittaga joy bor.
	c.add(31 * time.Second)
	if !l.Allow() {
		t.Error("birinchi yozuv chiqqach joy bo'shashi kerak edi")
	}
	if l.Allow() {
		t.Error("ikkinchi yozuv hali oynada — ish o'tmasligi kerak edi")
	}
}

// TestOff — chegara o'chirilgan holat (.env da 0 berilsa).
func TestOff(t *testing.T) {
	for _, l := range []*Limiter{New(0, time.Minute), New(5, 0), nil} {
		if !l.Off() {
			t.Errorf("%v: Off() true bo'lishi kerak", l)
		}
		for i := 0; i < 100; i++ {
			if !l.Allow() {
				t.Fatalf("%v: chegara o'chiq bo'lsa hamma ish o'tishi kerak", l)
			}
		}
		if got := l.Left(); got != -1 {
			t.Errorf("chegarasiz Left()=%d, -1 kutilgandi", got)
		}
		if got := l.Until(); got != 0 {
			t.Errorf("chegarasiz Until()=%s, 0 kutilgandi", got)
		}
	}
}

func TestString(t *testing.T) {
	if got := New(5, 2*time.Minute).String(); got != "2m0s da 5 ta" {
		t.Errorf("String()=%q", got)
	}
	if got := New(0, 0).String(); got != "o'chiq" {
		t.Errorf("String()=%q", got)
	}
}

// TestAgentCadence — agentning haqiqiy tsikliga taqlid: har 30 soniyada
// yangi tsikl, har tsiklda 20 tagacha nomzod suhbat. Chegara 2 daqiqada
// 5 ta bo'lsa, 10 daqiqada 25 tadan ortiq suhbat o'tmasligi kerak.
func TestAgentCadence(t *testing.T) {
	const (
		cycle      = 30 * time.Second
		candidates = 20 // har tsiklda navbatda turgan suhbatlar
		minutes    = 10
	)
	l, c := newTest(5, 2*time.Minute)

	handled := 0
	for tick := 0; tick < int(minutes*time.Minute/cycle); tick++ {
		for i := 0; i < candidates; i++ {
			if !l.Allow() {
				break // qolganlari keyingi tsiklga
			}
			handled++
		}
		c.add(cycle)
	}

	// 10 daqiqa = 5 ta to'liq oyna → 25 ta.
	if handled != 25 {
		t.Errorf("%d daqiqada %d ta suhbat o'tdi, 25 kutilgandi", minutes, handled)
	}
	t.Logf("%d daqiqa: %d ta suhbat (%.1f ta/daqiqa)",
		minutes, handled, float64(handled)/minutes)
}
