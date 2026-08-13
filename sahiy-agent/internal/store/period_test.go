package store

import (
	"testing"
	"time"
)

func TestSinceDays(t *testing.T) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// days=1 → faqat bugun (bugungi yarim tundan).
	got, ok := sinceDays(1)
	if !ok || !got.Equal(midnight) {
		t.Errorf("sinceDays(1) = %v (ok=%v), kutilgan %v", got, ok, midnight)
	}
	// days=7 → bugun + oldingi 6 kun.
	got, ok = sinceDays(7)
	if want := midnight.AddDate(0, 0, -6); !ok || !got.Equal(want) {
		t.Errorf("sinceDays(7) = %v, kutilgan %v", got, want)
	}
	// days<=0 → davr cheklanmaydi.
	for _, d := range []int{0, -5} {
		if got, ok := sinceDays(d); ok || !got.IsZero() {
			t.Errorf("sinceDays(%d) = %v (ok=%v), cheklovsiz kutilgan", d, got, ok)
		}
	}
}

func TestOrderBy(t *testing.T) {
	if got := orderBy("last"); got != "MAX(created) DESC" {
		t.Errorf(`orderBy("last") = %q`, got)
	}
	// Default — xarajat bo'yicha (qimmatlari tepada).
	for _, s := range []string{"cost", "", "nomalum"} {
		if got := orderBy(s); got != "SUM(cost_usd) DESC, MAX(created) DESC" {
			t.Errorf("orderBy(%q) = %q", s, got)
		}
	}
}

func TestCapLimit(t *testing.T) {
	cases := map[int]int{0: 50, -1: 50, 10: 10, 500: 500, 5000: 500}
	for in, want := range cases {
		if got := capLimit(in); got != want {
			t.Errorf("capLimit(%d) = %d, kutilgan %d", in, got, want)
		}
	}
}
