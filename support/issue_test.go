package support

import (
	"testing"
	"time"
)

// kunOldin - berilgan kun oldingi sanani adminka ko'rinishida qaytaradi.
func kunOldin(d int) string {
	return time.Now().AddDate(0, 0, -d).Format(adminkaTimeLayout)
}

func TestIsProblem(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		payStatus int
		paidAt    string
		want      bool
	}{
		{"to'langan, 8 kun", StatusPaid, 1, kunOldin(8), true},
		{"to'langan, 2 kun", StatusPaid, 1, kunOldin(2), false},
		{"to'langan, aniq 3 kun", StatusPaid, 1, kunOldin(3), false}, // "3 kundan ko'p" shart
		{"kutilmoqda, 5 kun", StatusWaiting, 1, kunOldin(5), true},
		{"yakunlangan, 60 kun", StatusFinished, 1, kunOldin(60), false},
		{"TO'LANMAGAN, 30 kun", StatusPaid, 0, kunOldin(30), false},
		{"sana yo'q", StatusPaid, 1, "", false},
	}
	for _, c := range cases {
		o := AdminkaOrder{OrderSN: "DG1", Status: c.status, PayStatus: c.payStatus, PaidAt: c.paidAt}
		if got := IsProblem(o); got != c.want {
			t.Errorf("%s: %v kutilgan, %v keldi (kun: %d)", c.name, c.want, got, DaysSincePaid(o))
		}
	}
}

// TestPaidTime - kun hisobi paid_at dan olinadi; u bo'sh bo'lsa
// created_at ga qaytiladi.
func TestPaidTime(t *testing.T) {
	// paid_at bor — created_at e'tiborga olinmaydi.
	o := AdminkaOrder{PayStatus: 1, PaidAt: kunOldin(4), CreatedAt: kunOldin(20)}
	if got := DaysSincePaid(o); got != 4 {
		t.Errorf("paid_at bo'yicha 4 kun kutilgan: %d", got)
	}
	// paid_at bo'sh — created_at ishlatiladi.
	o = AdminkaOrder{PayStatus: 1, CreatedAt: kunOldin(6)}
	if got := DaysSincePaid(o); got != 6 {
		t.Errorf("created_at bo'yicha 6 kun kutilgan: %d", got)
	}
}

func TestOrderView(t *testing.T) {
	v := NewOrderView(AdminkaOrder{OrderSN: "DG2", Status: StatusPaid, PayStatus: 1, PaidAt: kunOldin(9)})
	if v.StatusLabel != "sotib olingan, to'langan" {
		t.Errorf("status nomi: %q", v.StatusLabel)
	}
	if v.DaysSincePaid != 9 || !v.Problem {
		t.Errorf("kun: %d, problem: %v", v.DaysSincePaid, v.Problem)
	}
	if !v.Paid {
		t.Error("to'langan deb ko'rsatilishi kerak edi")
	}
	// Xom status raqami saqlanadi (kod uchun), lekin nomi ham bor.
	if v.Status != StatusPaid {
		t.Errorf("status: %d", v.Status)
	}

	// To'lanmagan buyurtma: muammo emas va kun sanalmaydi.
	u := NewOrderView(AdminkaOrder{OrderSN: "DG3", Status: StatusPaid, PayStatus: 0, CreatedAt: kunOldin(30)})
	if u.Paid || u.Problem || u.DaysSincePaid != 0 {
		t.Errorf("to'lanmagan: %+v", u)
	}
}

func TestSanaMatn(t *testing.T) {
	if got := sanaMatn("2026-08-21 10:00:00"); got != "21-avgust" {
		t.Errorf("21-avgust kutilgan: %q", got)
	}
	if got := sanaMatn(""); got != "" {
		t.Errorf("bo'sh kutilgan: %q", got)
	}
}
