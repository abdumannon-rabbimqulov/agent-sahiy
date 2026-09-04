package support

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCustomerType(t *testing.T) {
	b2c := []AdminkaOrder{{OrderSN: "DG1", PayStatus: 1, B2CPercentage: 15}}
	if got := CustomerType(b2c); got != CustomerB2C {
		t.Errorf("B2C kutilgan: %q", got)
	}

	b2b := []AdminkaOrder{{OrderSN: "DG2", PayStatus: 1, B2CPercentage: 0}}
	if got := CustomerType(b2b); got != CustomerB2B {
		t.Errorf("B2B kutilgan: %q", got)
	}

	if got := CustomerType(nil); got != CustomerUnknown {
		t.Errorf("noma'lum kutilgan: %q", got)
	}

	// Bitta buyurtmada foiz bo'lsa — oddiy mijoz (aralash holatda ustun).
	mix := []AdminkaOrder{{OrderSN: "DG3", PayStatus: 1}, {OrderSN: "DG4", PayStatus: 1, B2CPercentage: 15}}
	if got := CustomerType(mix); got != CustomerB2C {
		t.Errorf("aralashda B2C kutilgan: %q", got)
	}
}

func TestBriefDelivery(t *testing.T) {
	iso := func(d int) string { return time.Now().AddDate(0, 0, -d).Format(time.RFC3339) }

	rows := []DeliveryOrder{
		// Olinmagan — filialda kutmoqda.
		{ExpressNum: "JT1", Delivered: false, BranchName: "Chirchiq",
			BranchAddress: "Toshkent viloyati, Chirchiq", CreatedAt: iso(12)},
		// Kuryerga berilgan, 3 kun ichida — yo'lda.
		{ExpressNum: "JT2", Delivered: true, DeliveredAt: iso(1), BranchName: "SAHIY JIZZAX"},
		{ExpressNum: "JT3", Delivered: true, DeliveredAt: iso(3), BranchName: "SAHIY JIZZAX"},
		// Muddati o'tgan — holati noaniq, tekshirish kerak.
		{ExpressNum: "JT4", Delivered: true, DeliveredAt: iso(40), BranchName: "SAHIY JIZZAX"},
		// Sanasi o'qilmadi — u ham tekshiriladi.
		{ExpressNum: "JT5", Delivered: true, DeliveredAt: "", BranchName: "SAHIY JIZZAX"},
	}

	b := BriefDelivery(rows)
	if len(b.Pending) != 1 || b.Pending[0].Branch != "Chirchiq" {
		t.Fatalf("olinmagan: %+v", b.Pending)
	}
	if b.Pending[0].ArrivedAt == "" {
		t.Error("kelgan sanasi bo'sh")
	}
	if len(b.InDelivery) != 2 {
		t.Fatalf("yetkazilmoqda: %+v", b.InDelivery)
	}
	// Yangisi birinchi.
	if b.InDelivery[0].ExpressNum != "JT2" || b.InDelivery[0].Days != 1 {
		t.Errorf("yetkazilmoqda tartibi/kuni: %+v", b.InDelivery)
	}
	if b.InDelivery[0].SentAt == "" {
		t.Error("berilgan sanasi bo'sh")
	}
	if len(b.NeedCheck) != 2 {
		t.Fatalf("tekshirish kerak: %+v", b.NeedCheck)
	}
	if b.NeedCheck[0].ExpressNum != "JT4" || b.NeedCheck[0].Days != 40 {
		t.Errorf("tekshiriladigan yozuv: %+v", b.NeedCheck)
	}

	// Aniq chegara: 3 kun — hali yo'lda, 4 kun — tekshirish kerak.
	chek := BriefDelivery([]DeliveryOrder{
		{ExpressNum: "JT6", Delivered: true, DeliveredAt: iso(DeliveryDays)},
		{ExpressNum: "JT7", Delivered: true, DeliveredAt: iso(DeliveryDays + 1)},
	})
	if len(chek.InDelivery) != 1 || chek.InDelivery[0].ExpressNum != "JT6" {
		t.Errorf("%d kun yo'lda bo'lishi kerak: %+v", DeliveryDays, chek.InDelivery)
	}
	if len(chek.NeedCheck) != 1 || chek.NeedCheck[0].ExpressNum != "JT7" {
		t.Errorf("%d kun tekshirilishi kerak: %+v", DeliveryDays+1, chek.NeedCheck)
	}
	if b.Empty {
		t.Error("yozuv bor edi")
	}

	// Umuman yozuv bo'lmasa.
	if e := BriefDelivery(nil); !e.Empty {
		t.Error("yozuv_yoq kutilgan")
	}
	// Eski yozuv endi YASHIRILMAYDI: "yetkazildi" deb aytib bo'lmaydi,
	// shuning uchun u tekshiriladiganlar ro'yxatiga tushadi.
	old := BriefDelivery([]DeliveryOrder{{Delivered: true, DeliveredAt: iso(40)}})
	if old.Empty || len(old.NeedCheck) != 1 {
		t.Errorf("eski yozuv tekshirishga tushishi kerak: %+v", old)
	}
}

// TestBriefOrdersCompact - modelga xom maydonlar emas, saralangani ketadi.
func TestBriefOrders(t *testing.T) {
	views := []OrderView{NewOrderView(AdminkaOrder{
		OrderSN: "DG1", Status: StatusPaid, PayStatus: 1,
		PaidAt:      time.Now().AddDate(0, 0, -8).Format(adminkaTimeLayout),
		ShippedAt:   "2026-08-24 10:00:00",
		ExpressNum:  "P787568962692",
		PackageName: "Test",
		// Modelga kerak bo'lmagan maydonlar:
		ReceiverName: "Ism", Street: "Ko'cha", Amount: "13.41", Province: "Toshkent",
	})}

	raw, err := json.Marshal(BriefOrders(views))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, bad := range []string{"receiver_name", "street", "amount", "province", "Ism"} {
		if contains(s, bad) {
			t.Errorf("ortiqcha maydon modelga ketyapti: %s", bad)
		}
	}
	for _, want := range []string{"DG1", "24-avgust", "P787568962692", "days_since_paid"} {
		if !contains(s, want) {
			t.Errorf("kerakli maydon yo'q: %s\n%s", want, s)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestIsImageLink(t *testing.T) {
	yes := []string{
		"https://storage.abusahiy.uz/client-chat-images/1788073552277627254-1000042348.jpg",
		"https://example.com/a/b.PNG",
		"http://x.uz/chat-images/abc",
	}
	no := []string{
		"DG60607041 qayerda?",
		"https://sahiy.uz/tariflar",
		"rasm: https://x.uz/a.jpg va yana matn",
		"",
	}
	for _, s := range yes {
		if !isImageLink(s) {
			t.Errorf("rasm deb tanilishi kerak: %q", s)
		}
	}
	for _, s := range no {
		if isImageLink(s) {
			t.Errorf("rasm emas: %q", s)
		}
	}
}

// TestPendingChats - javobsiz suhbatlar to'g'ri saralanadimi.
// (DB bo'lmagani uchun "ishlangan" holatlar tekshirilmaydi — bu yerda
// faqat filtr va tartib sinaladi.)
func TestPendingChats(t *testing.T) {
	chats := []Chat{
		{ID: 1, OperatorUnseenCount: 0, MsCreatedAt: "2026-08-30T16:00:00Z"}, // javob berilgan
		{ID: 2, OperatorUnseenCount: 1, MsCreatedAt: "2026-08-29T10:00:00Z"}, // eski, javobsiz
		{ID: 3, OperatorUnseenCount: 3, MsCreatedAt: "2026-08-30T15:00:00Z"}, // yangi, javobsiz
		{ID: 4, OperatorUnseenCount: 2, MsCreatedAt: "2026-08-30T09:00:00Z"},
	}

	got := pendingChats(chats)
	if len(got) != 3 {
		t.Fatalf("3 ta javobsiz kutilgan: %d", len(got))
	}
	// Eng yangisi birinchi bo'lishi kerak.
	want := []int64{3, 4, 2}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("%d-o'rinda %d kutilgan, %d keldi", i, id, got[i].ID)
		}
	}
	// Javob berilgani (unseen=0) ro'yxatga tushmasin.
	for _, c := range got {
		if c.ID == 1 {
			t.Error("javob berilgan suhbat ro'yxatga tushdi")
		}
	}
}

// TestLastWordOurs - oxirgi so'z biz tomondan bo'lsa zanjir yurmasligi
// kerak (mijoz javob kutmayapti).
func TestUnansweredIDsWhenAgentLast(t *testing.T) {
	msgs := []Message{
		{ID: 1, SenderType: "client", Message: "DG60607041 что с этим заказом."},
		{ID: 2, SenderType: "agent", Message: "Ваш заказ в филиале Чирчик."},
	}
	if got := UnansweredClientIDs(msgs); len(got) != 0 {
		t.Errorf("javobsiz xabar bo'lmasligi kerak: %v", got)
	}
	if msgs[len(msgs)-1].FromClient() {
		t.Error("oxirgi xabar agentniki bo'lishi kerak edi")
	}
}
