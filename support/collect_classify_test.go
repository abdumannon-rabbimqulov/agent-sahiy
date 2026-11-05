package support

import (
	"testing"
	"time"
)

var collectNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)

func daysAgo(n int) string {
	return collectNow.Add(-time.Duration(n)*24*time.Hour - time.Hour).Format(adminkaTimeLayout)
}

func findingKeys(v CollectVerdict) map[string]bool {
	out := map[string]bool{}
	for _, f := range v.Findings {
		out[f.Type+"/"+f.Bucket] = true
	}
	return out
}

func TestClassifyAgeBoundaries(t *testing.T) {
	cases := []struct {
		name string
		o    AdminkaOrder
		want string // bo'sh - hech qanday tur yo'q
	}{
		{"status3 5 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(5)}, ""},
		{"status3 sanasiz", AdminkaOrder{Status: 3}, ""},
		{"status3 6 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(6)}, "sotib_olingan/5"},
		{"status3 20 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(20)}, "sotib_olingan/5"},
		{"status3 21 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(21)}, "sotib_olingan/20"},
		{"status3 30 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(30)}, "sotib_olingan/20"},
		{"status3 31 kun", AdminkaOrder{Status: 3, CreatedAt: daysAgo(31)}, "sotib_olingan/30"},
		{"status12 yangi", AdminkaOrder{Status: 12, CreatedAt: daysAgo(1)}, "status_12/"},
		{"status12 sanasiz", AdminkaOrder{Status: 12}, "status_12/"},
		{"yolda 20", AdminkaOrder{Status: 7, ShippedAt: daysAgo(20), CreatedAt: daysAgo(20)}, ""},
		{"yolda 21", AdminkaOrder{Status: 7, ShippedAt: daysAgo(21), CreatedAt: daysAgo(21)}, "yolda/20"},
		{"yolda 31", AdminkaOrder{Status: 7, ShippedAt: daysAgo(31), CreatedAt: daysAgo(10)}, "yolda/30"},
		{"status6 25", AdminkaOrder{Status: 6, CreatedAt: daysAgo(25)}, ""},
		{"status6 26 sanasi yaratilgandan", AdminkaOrder{Status: 6, CreatedAt: daysAgo(26)}, "status_6_kelmagan/"},
		{"boshqa 25", AdminkaOrder{Status: 4, CreatedAt: daysAgo(25)}, ""},
		{"boshqa 26", AdminkaOrder{Status: 4, CreatedAt: daysAgo(26)}, "boshqa_kelmagan/"},
		{"taqiqlangan boshqaga kirmaydi", AdminkaOrder{Status: 10, CreatedAt: daysAgo(60)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.o.PayStatus = 1
			got := findingKeys(ClassifyOrder(CollectInput{Order: c.o, DeliveryOK: true}, collectNow))
			if c.want == "" && len(got) != 0 {
				t.Fatalf("tur kutilmagan edi: %v", got)
			}
			if c.want != "" && !got[c.want] {
				t.Fatalf("%s kutilgan edi: %v", c.want, got)
			}
		})
	}
}

func TestClassifyStatus6AlsoOnWay(t *testing.T) {
	o := AdminkaOrder{Status: 6, PayStatus: 1, ShippedAt: daysAgo(31), CreatedAt: daysAgo(40), ExpressNum: "JT1"}
	got := findingKeys(ClassifyOrder(CollectInput{Order: o, DeliveryOK: true}, collectNow))
	if !got["yolda/30"] || !got["status_6_kelmagan/"] {
		t.Fatalf("ikkala tur kutilgan edi: %v", got)
	}
}

func TestClassifyUnpaidSkipped(t *testing.T) {
	v := ClassifyOrder(CollectInput{Order: AdminkaOrder{Status: 3, CreatedAt: daysAgo(40)}}, collectNow)
	if len(v.Findings) != 0 {
		t.Fatalf("to'lanmagan turga kirmasligi kerak: %v", v.Findings)
	}
}

func TestClassifyDeliveryErrorNotVerified(t *testing.T) {
	o := AdminkaOrder{Status: 6, PayStatus: 1, ShippedAt: daysAgo(40), ExpressNum: "JT1"}
	v := ClassifyOrder(CollectInput{Order: o, DeliveryOK: false}, collectNow)
	if v.Verified || len(v.Findings) != 0 {
		t.Fatalf("yetkazma xatosida xulosa chiqmasligi kerak: %+v", v)
	}
}

func TestClassifyStatus3ArrivedIncluded(t *testing.T) {
	o := AdminkaOrder{Status: 3, PayStatus: 1, CreatedAt: daysAgo(40), ExpressNum: "JT1", UserID: 5}
	rows := []DeliveryOrder{{ExpressNum: "JT1", UserID: 5, Delivered: true, Status: 7, CreatedAt: daysAgo(2)}}
	got := findingKeys(ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: true}, collectNow))
	if !got["sotib_olingan/30"] {
		t.Fatalf("status 3 hammasi kirishi kerak: %v", got)
	}
}

func TestClassifyArrivedNotLate(t *testing.T) {
	o := AdminkaOrder{Status: 6, PayStatus: 1, ShippedAt: daysAgo(40), CreatedAt: daysAgo(40), ExpressNum: "JT1", UserID: 5}
	rows := []DeliveryOrder{{ExpressNum: "JT1", UserID: 5, Delivered: true, Status: 7, CreatedAt: daysAgo(2)}}
	v := ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: true}, collectNow)
	if !v.Arrived || !v.Received || len(v.Findings) != 0 {
		t.Fatalf("mijoz olgan tovar muammo emas: %+v", v)
	}
}

func TestClassifyOwnerMismatch(t *testing.T) {
	o := AdminkaOrder{Status: 6, PayStatus: 1, ExpressNum: "JT1", UserID: 5}
	rows := []DeliveryOrder{{ExpressNum: "JT1", UserID: 9, CreatedAt: daysAgo(1)}}
	got := findingKeys(ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: true}, collectNow))
	if !got["xato_kelgan/"] {
		t.Fatalf("xato_kelgan kutilgan edi: %v", got)
	}
}

func TestClassifyCourier(t *testing.T) {
	base := AdminkaOrder{Status: 6, PayStatus: 1, ExpressNum: "JT1", UserID: 5, Province: "Toshkent shahri"}
	row := func(branch, city string, days int, delivered bool) []DeliveryOrder {
		return []DeliveryOrder{{ExpressNum: "JT1", UserID: 5, BranchName: branch, City: city,
			CreatedAt: daysAgo(days), Delivered: delivered, ExpressLine: "delivery"}}
	}
	check := func(name string, rows []DeliveryOrder, o AdminkaOrder, want bool) {
		t.Helper()
		got := findingKeys(ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: true}, collectNow))
		if got["kuryer_olmagan/"] != want {
			t.Errorf("%s: kuryer_olmagan=%v kutilgan edi: %v", name, want, got)
		}
	}
	check("Toshkent 4 kun", row("SHOTA", "Toshkent shahri", 4, false), base, true)
	check("Toshkent 3 kun", row("SHOTA", "Toshkent shahri", 3, false), base, false)
	check("kuryerga berilgan", row("SHOTA", "Toshkent shahri", 9, true), base, false)

	jizzax := base
	jizzax.Province = "Jizzax viloyati"
	check("viloyatda kuryer yo'q", row("SAHIY JIZZAX", "Jizzax viloyati", 9, false), jizzax, false)
}

func TestClassifyAddress(t *testing.T) {
	o := AdminkaOrder{Status: 6, PayStatus: 1, ExpressNum: "JT1", UserID: 5, Province: "Jizzax viloyati"}
	rows := []DeliveryOrder{{ExpressNum: "JT1", UserID: 5, BranchName: "SAHIY GULISTION", CreatedAt: daysAgo(1)}}
	got := findingKeys(ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: true}, collectNow))
	if !got["manzilga_yetmagan/"] {
		t.Fatalf("manzilga_yetmagan kutilgan edi: %v", got)
	}
}

func TestCollectGap(t *testing.T) {
	now := collectNow
	dl := now.Add(115 * time.Minute)
	min := 500 * time.Millisecond
	if g := collectGap(dl, now, 1000, min, 1); g != 6900*time.Millisecond {
		t.Fatalf("1000 suhbatda 6.9s kutilgan edi: %v", g)
	}
	if g := collectGap(dl, now, 1000, min, 2); g != 13800*time.Millisecond {
		t.Fatalf("backoff ikki baravar oshirishi kerak: %v", g)
	}
	if g := collectGap(now.Add(-time.Minute), now, 10, min, 1); g != min {
		t.Fatalf("muddat o'tganda eng kam oraliq: %v", g)
	}
}

func TestCollectPending(t *testing.T) {
	chats := []Chat{
		{ID: 1, MsCreatedAt: "b", OperatorUnseenCount: 1},
		{ID: 2, MsCreatedAt: "a", OperatorUnseenCount: 1},
		{ID: 3, MsCreatedAt: "c", OperatorUnseenCount: 1},
		{ID: 4, MsCreatedAt: "d", OperatorUnseenCount: 1},
		{ID: 5, MsCreatedAt: "e", OperatorUnseenCount: 0},
		{ID: 6, MsCreatedAt: "f", OperatorUnseenCount: 1},
	}
	seen := map[int64]CollectChat{
		1: {LastMessageAt: "b", State: CollectStateDone},
		3: {LastMessageAt: "old", State: CollectStateDone},
		4: {LastMessageAt: "d", State: CollectStatePartial, Attempts: 1},
		6: {LastMessageAt: "f", State: CollectStateError, Attempts: collectMaxAttempts},
	}
	got := collectPending(chats, seen)
	want := []int64{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("tartib: %v", got)
		}
	}
}

func TestApplyAIResultIgnoresBad(t *testing.T) {
	orders := []collectedOrder{{row: CollectOrder{OrderSN: "DG1"}}}
	rec := CollectChat{}
	applyAIResult(`{"xulosalar":[{"order_sn":"DG1","tur":"xato_kelgan","izoh":"boshqa tovar"},{"order_sn":null,"tur":"nomalum","izoh":"x"}]}`, &rec, orders)
	if orders[0].row.AIType != "xato_kelgan" || rec.AIType != "" {
		t.Fatalf("%+v %+v", orders[0].row, rec)
	}
	applyAIResult(`bu json emas`, &rec, orders)
	if rec.AIType != "" {
		t.Fatalf("JSON bo'lmasa o'zgarmasligi kerak")
	}
}

func TestApplyAIResultInherits(t *testing.T) {
	orders := []collectedOrder{{row: CollectOrder{OrderSN: "DG1"}}, {row: CollectOrder{OrderSN: "DG2"}}}
	rec := CollectChat{}
	applyAIResult(`{"xulosalar":[{"order_sn":null,"tur":"kelmagan","izoh":"kelmadi"},{"order_sn":"DG2","tur":"yolda","izoh":"y"}]}`, &rec, orders)
	if orders[0].row.AIType != "kelmagan" || orders[1].row.AIType != "yolda" {
		t.Fatalf("%+v", orders)
	}
}

func TestCollectModeGuard(t *testing.T) {
	settingsCache.Lock()
	old, oldAt := settingsCache.val, settingsCache.at
	settingsCache.val = map[string]string{SettingCollectMode: "true"}
	settingsCache.at = time.Now()
	settingsCache.Unlock()
	defer func() {
		settingsCache.Lock()
		settingsCache.val, settingsCache.at = old, oldAt
		settingsCache.Unlock()
	}()

	if !blockedByCollectMode(&Interaction{Source: SourceAgent}) {
		t.Fatal("AI javobi to'silishi kerak")
	}
	if blockedByCollectMode(&Interaction{Source: SourceTelegram}) {
		t.Fatal("xodim javobi o'tishi kerak")
	}
	if err := deliverChat(&Interaction{Source: SourceAgent, ChatReply: "salom", ConversationID: 1}, false); err != ErrCollectMode {
		t.Fatalf("deliverChat to'silmadi: %v", err)
	}
	if _, err := RunChain(t.Context(), 1, 1); err != ErrCollectMode {
		t.Fatalf("RunChain to'silmadi: %v", err)
	}
}
