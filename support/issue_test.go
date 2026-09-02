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

// TestStaffAnsweredIn - muammoni faqat XODIM javobi yopadi; AI agentning
// o'z javobi (AGENT_SENDER_ID) hisobga olinmaydi.
func TestStaffAnsweredIn(t *testing.T) {
	t.Setenv("AGENT_SENDER_ID", "74")

	client := Message{ID: 1, SenderID: 5, SenderType: "client", Message: "qachon keladi?"}
	ai := Message{ID: 2, SenderID: 74, SenderType: "agent", Message: "tekshirilmoqda",
		CreatedAt: "2026-08-30T10:00:00Z"}
	xodim := Message{ID: 3, SenderID: 56, SenderType: "agent", Message: "ertaga jo'natamiz",
		CreatedAt: "2026-08-30T12:00:00Z"}

	if ok, _ := staffAnsweredIn([]Message{client}); ok {
		t.Error("oxirgi so'z mijozniki — javob berilmagan")
	}
	if ok, _ := staffAnsweredIn([]Message{client, ai}); ok {
		t.Error("AI javobi muammoni yopmasligi kerak")
	}
	ok, at := staffAnsweredIn([]Message{client, ai, xodim})
	if !ok {
		t.Fatal("xodim javobi tanilishi kerak")
	}
	if at.IsZero() {
		t.Error("javob vaqti o'qilmadi")
	}
	if ok, _ := staffAnsweredIn(nil); ok {
		t.Error("bo'sh suhbat")
	}
}

func TestIssuesTextGrouped(t *testing.T) {
	list := []*OrderIssue{
		{OrderSN: "DG1", ClientID: 7, ConversationID: 9, StatusLabel: "kutilmoqda",
			PaidAt: "2026-08-21 10:00:00", DaysSincePaid: 5, PackageName: "telefon"},
		{OrderSN: "DG2", ClientID: 7, ConversationID: 9, StatusLabel: "yo'lda",
			PaidAt: "2026-08-22 10:00:00", DaysSincePaid: 4},
	}

	// Ikkita muammo — bitta matn, ikkalasi ham ichida.
	got := issuesText(list)
	for _, want := range []string{"2 ta", "DG1", "DG2", "Mijoz: 7", "suhbat #9", "hammasini yopadi"} {
		if !contains(got, want) {
			t.Errorf("guruh matnida %q yo'q:\n%s", want, got)
		}
	}
	if n := countSub(got, "Mijoz: 7"); n != 1 {
		t.Errorf("mijoz qatori %d marta yozilgan, 1 marta bo'lishi kerak:\n%s", n, got)
	}

	// Bitta muammo — eski qisqa ko'rinish.
	one := issuesText(list[:1])
	if !contains(one, "Muammoli buyurtma — DG1") || contains(one, "hammasini yopadi") {
		t.Errorf("yakka muammo matni noto'g'ri:\n%s", one)
	}

	if issuesText(nil) != "" {
		t.Error("bo'sh ro'yxatga matn yozilmasligi kerak")
	}
}

func TestRemindTextGrouped(t *testing.T) {
	items := []remindItem{
		{Issue: &OrderIssue{OrderSN: "DG1", ClientID: 7, ConversationID: 9, StatusLabel: "kutilmoqda"}, Days: 5},
		{Issue: &OrderIssue{OrderSN: "DG2", ClientID: 7, ConversationID: 9, StatusLabel: "yo'lda"}, Days: 4},
	}
	got := remindText(items)
	for _, want := range []string{"2 ta buyurtma", "DG1", "DG2", "BERILMAGAN", "hammasini yopadi"} {
		if !contains(got, want) {
			t.Errorf("eslatma matnida %q yo'q:\n%s", want, got)
		}
	}
	if n := countSub(got, "BERILMAGAN"); n != 1 {
		t.Errorf("javob holati %d marta yozilgan, 1 marta bo'lishi kerak:\n%s", n, got)
	}
	if remindText(nil) != "" {
		t.Error("bo'sh ro'yxatga eslatma yozilmasligi kerak")
	}
}

// countSub - matnda qism necha marta uchraydi.
func countSub(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func TestHasPendingOrders(t *testing.T) {
	paid := func(status int) OrderView {
		return NewOrderView(AdminkaOrder{OrderSN: "DG1", Status: status, PayStatus: 1,
			PaidAt: time.Now().Format(adminkaTimeLayout)})
	}
	if !HasPendingOrders([]OrderView{paid(StatusPaid)}) {
		t.Error("to'langan, yakunlanmagan buyurtma — kelmagan hisoblanishi kerak")
	}
	if !HasPendingOrders([]OrderView{paid(StatusFinished), paid(StatusWaiting)}) {
		t.Error("bittasi yakunlanmagan — kelmagan bor")
	}
	if HasPendingOrders([]OrderView{paid(StatusFinished)}) {
		t.Error("yakunlangan buyurtma kelmagan emas")
	}
	// To'lanmagan buyurtma hali yo'lga chiqmagan — hisobga olinmaydi.
	unpaid := NewOrderView(AdminkaOrder{OrderSN: "DG2", Status: StatusPaid})
	if HasPendingOrders([]OrderView{unpaid}) {
		t.Error("to'lanmagan buyurtma kelmagan hisoblanmasligi kerak")
	}
	if HasPendingOrders(nil) {
		t.Error("bo'sh ro'yxat")
	}
}
