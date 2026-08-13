package models

import "testing"

func TestStatusLabel(t *testing.T) {
	cases := map[string]string{
		StatusAISent:    "AI hal qildi",
		StatusPending:   "Jarayonda — xodim javobi kutilmoqda",
		StatusStaffSent: "Xodim hal qildi",
		"nomalum":       "nomalum", // noma'lum status o'zicha qaytadi
	}
	for in, want := range cases {
		if got := StatusLabel(in); got != want {
			t.Errorf("StatusLabel(%q) = %q, kutilgan %q", in, got, want)
		}
	}
}

func TestEscalationResolved(t *testing.T) {
	if (Escalation{Status: StatusPending}).Resolved() {
		t.Error("pending — hal qilinmagan bo'lishi kerak")
	}
	if !(Escalation{Status: StatusStaffSent}).Resolved() {
		t.Error("staff_sent — hal qilingan bo'lishi kerak")
	}
}
