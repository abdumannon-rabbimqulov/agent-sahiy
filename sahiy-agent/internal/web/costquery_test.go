package web

import (
	"net/http/httptest"
	"testing"
)

func query(raw string) costQuery {
	return parseCostQuery(httptest.NewRequest("GET", "/api/costs"+raw, nil))
}

func TestParseCostQueryDefault(t *testing.T) {
	q := query("")
	if q.Group != "day" || q.Days != 30 || q.Sort != "cost" || q.Limit != 50 {
		t.Errorf("default = %+v", q)
	}
}

func TestParseCostQueryGuruhVaSaralash(t *testing.T) {
	if q := query("?group=client&sort=last"); q.Group != "client" || q.Sort != "last" {
		t.Errorf("= %+v", q)
	}
	if q := query("?group=conversation"); q.Group != "conversation" {
		t.Errorf("= %+v", q)
	}
	// Noto'g'ri qiymatlar default'ga tushadi.
	if q := query("?group=xato&sort=xato"); q.Group != "day" || q.Sort != "cost" {
		t.Errorf("noto'g'ri qiymat default'ga tushmadi: %+v", q)
	}
}

func TestParseCostQueryDavr(t *testing.T) {
	// "all" va 0 — butun tarix.
	for _, v := range []string{"?days=all", "?days=ALL", "?days=0"} {
		if q := query(v); q.Days != 0 {
			t.Errorf("%s → Days = %d, 0 kutilgan", v, q.Days)
		}
	}
	if q := query("?days=7"); q.Days != 7 {
		t.Errorf("days=7 → %d", q.Days)
	}
	// Chegara: 365 kundan oshmaydi.
	if q := query("?days=10000"); q.Days != 365 {
		t.Errorf("days=10000 → %d, 365 kutilgan", q.Days)
	}
	// Manfiy yoki noto'g'ri — default 30 bo'lib qoladi.
	for _, v := range []string{"?days=-5", "?days=abc"} {
		if q := query(v); q.Days != 30 {
			t.Errorf("%s → Days = %d, 30 kutilgan", v, q.Days)
		}
	}
}

func TestParseCostQueryLimit(t *testing.T) {
	if q := query("?limit=100"); q.Limit != 100 {
		t.Errorf("limit=100 → %d", q.Limit)
	}
	if q := query("?limit=9999"); q.Limit != 500 {
		t.Errorf("limit=9999 → %d, 500 kutilgan", q.Limit)
	}
	if q := query("?limit=0"); q.Limit != 50 {
		t.Errorf("limit=0 → %d, 50 kutilgan", q.Limit)
	}
}
