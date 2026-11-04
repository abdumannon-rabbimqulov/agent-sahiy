package support

import (
	"strings"
	"testing"
)

func pickedUpRow() DeliveryOrder {
	return DeliveryOrder{
		ExpressNum:  "YT1",
		UserID:      7,
		Delivered:   true,
		DeliveredAt: "2026-10-01T10:00:00Z",
		Status:      7,
		ExpressLine: "Auto cargo-Pickup",
		City:        "Jizzax",
		BranchName:  "SAHIY JIZZAX",
	}
}

func clientMsgs(texts ...string) []Message {
	var out []Message
	for _, t := range texts {
		out = append(out, Message{SenderType: "client", Message: t})
	}
	return out
}

// Olingan tovar: ketma-ketlik muammo chiqarmaydi va xodimga qator yozmaydi.
func TestReceivedOrderIsNotAProblem(t *testing.T) {
	c := checked(shippedOrder(StatusWaiting, 40, 50), []DeliveryOrder{pickedUpRow()})
	if c.Problem || c.StaffText != "" || c.Step != StepReceived {
		t.Fatalf("olingan tovar muammo bo'lmasligi kerak: %+v", c)
	}
}

// Filial boshqa viloyatda bo'lsa ham, tovar qo'lida: guruhga chiqmaydi.
func TestReceivedOrderSkipsRegionCheck(t *testing.T) {
	o := shippedOrder(StatusWaiting, 40, 50)
	o.Province = "Toshkent shahri"
	row := pickedUpRow()
	if c := checked(o, []DeliveryOrder{row}); c.Problem || c.StaffText != "" {
		t.Fatalf("viloyat olingan tovarda solishtirilmasligi kerak: %+v", c)
	}
	row.City = "Toshkent shahri"
	if brief, bad := BriefDelivery([]DeliveryOrder{row}); len(bad) != 0 || len(brief.PickedUp) != 1 {
		t.Fatalf("olingan tovar mismatch bermasligi kerak: %+v %+v", brief, bad)
	}
}

// Egasi mos kelmasa — tovar olingan bo'lsa ham xodimga chiqadi.
func TestReceivedOrderOwnerMismatchStillAlerts(t *testing.T) {
	row := pickedUpRow()
	row.UserID = 99
	c := checked(shippedOrder(StatusWaiting, 40, 50), []DeliveryOrder{row})
	if !c.Problem || c.Step != StepOwner {
		t.Fatalf("begona posilka muammo bo'lishi kerak: %+v", c)
	}
}

func TestPickedUpNoteAsksHowItArrived(t *testing.T) {
	if !strings.Contains(pickedUpNote, "yaxshi keldimi") {
		t.Fatalf("izoh mijozdan so'rashi kerak: %s", pickedUpNote)
	}
}

// Kuryer yetkazgan (status 7): "tekshirish kerak" emas va guruhga ogohlantirish
// chiqmaydi, muddat qancha o'tgan bo'lsa ham. Izohda filial yo'q.
func TestCourierStatus7IsReceived(t *testing.T) {
	row := pickedUpRow()
	row.ExpressLine = "Auto cargo-Delivery"
	row.City = "Toshkent shahri"
	row.BranchName = "SHOTA"
	brief, bad := BriefDelivery([]DeliveryOrder{row})
	if len(brief.Handed) != 1 || len(brief.NeedCheck) != 0 || len(brief.PickedUp) != 0 ||
		len(brief.Alerts) != 0 || len(bad) != 0 || brief.Empty {
		t.Fatalf("mijozga yetkazilgan bo'lishi kerak edi: %+v %+v", brief, bad)
	}
	if got := brief.Handed[0]; got.Branch != "" || got.Izoh != courierReceivedNote {
		t.Fatalf("kuryer izohi noto'g'ri: %+v", got)
	}
	if !strings.Contains(courierReceivedNote, "yaxshi keldimi") {
		t.Fatalf("izoh mijozdan so'rashi kerak: %s", courierReceivedNote)
	}
	if got := receivedTracks(brief, []OrderCheck{{Step: StepReceived}}); len(got) != 1 {
		t.Fatalf("trek qaytishi kerak edi: %v", got)
	}
}

// Kuryer yo'q viloyatda turi yozilmagan status 7 — filialdan olingan.
func TestStatus7UnknownLineInRegionIsPickedUp(t *testing.T) {
	row := pickedUpRow()
	row.ExpressLine = ""
	brief, _ := BriefDelivery([]DeliveryOrder{row})
	if len(brief.PickedUp) != 1 || len(brief.Handed) != 0 {
		t.Fatalf("olib ketilgan bo'lishi kerak edi: %+v", brief)
	}
}

// Status 7 bo'lmagan `delivered=true` — kuryerga berilgan xolos, tekshiruv
// avvalgidek qoladi.
func TestDeliveredWithoutStatus7IsNotReceived(t *testing.T) {
	row := pickedUpRow()
	row.Status = 8
	row.ExpressLine = "Auto cargo-Delivery"
	row.City = "Toshkent shahri"
	row.BranchName = "SHOTA"
	brief, _ := BriefDelivery([]DeliveryOrder{row})
	if len(brief.Handed) != 0 || len(brief.NeedCheck) != 1 || len(brief.Alerts) != 1 {
		t.Fatalf("tekshirish kerak bo'lishi kerak edi: %+v", brief)
	}
}

func TestReceivedTracksOnlyWhenEverythingReceived(t *testing.T) {
	done := DeliveryBrief{PickedUp: []PickupDone{{ExpressNum: "YT1"}}}
	if got := receivedTracks(done, []OrderCheck{{Step: StepReceived}}); len(got) != 1 {
		t.Fatalf("trek qaytishi kerak edi: %v", got)
	}
	if got := receivedTracks(done, nil); len(got) != 1 {
		t.Fatalf("faqat yetkazma so'ralgan holat: %v", got)
	}
	if got := receivedTracks(done, []OrderCheck{{Step: StepReceived}, {Step: StepLate}}); got != nil {
		t.Fatalf("yetmagan buyurtma bor — nil kutilgandi: %v", got)
	}
	mixed := done
	mixed.Pending = []PendingPickup{{ExpressNum: "YT2"}}
	if got := receivedTracks(mixed, nil); got != nil {
		t.Fatalf("topshirilmagan posilka bor — nil kutilgandi: %v", got)
	}
	if got := receivedTracks(DeliveryBrief{}, nil); got != nil {
		t.Fatalf("bo'sh ro'yxat: %v", got)
	}
}

func TestMentionsDefect(t *testing.T) {
	for _, s := range []string{
		"DG123 tovar singan keldi", "ekrani ishlamayapti", "Noto'g'ri tovar keldi",
		"товар сломан", "нотўғри келди", "yaxshi emas", "men olmadim",
	} {
		if !MentionsDefect(s) {
			t.Errorf("shikoyat ushlanmadi: %q", s)
		}
	}
	for _, s := range []string{"DG123", "DG123 qayerda?", "yaxshi keldi rahmat", "buyurtmam qachon keladi"} {
		if MentionsDefect(s) {
			t.Errorf("shikoyat emas edi: %q", s)
		}
	}
}

func TestReceivedHelpMuted(t *testing.T) {
	for _, s := range []string{"DG60597226 qayerda", "yaxshi keldi", "ha", "хорошо", "muammo yo'q"} {
		if !receivedHelpMuted(clientMsgs(s), false) {
			t.Errorf("guruhga chiqmasligi kerak edi: %q", s)
		}
	}
	// Ro'yxatga sig'magan javob: shikoyat bo'lishi mumkin, yashirilmaydi.
	if receivedHelpMuted(clientMsgs("rangi men tanlagandek emas"), false) {
		t.Error("noma'lum javob yashirilmasligi kerak")
	}
	if !receivedHelpMuted(clientMsgs("[rasm yuborildi]"), true) {
		t.Error("rasmdan o'qilgan raqam ham raqam yozilgani bilan teng")
	}
}

func TestDefectAlertNamesTrackAndMessage(t *testing.T) {
	got := defectAlert([]string{"YT1"}, clientMsgs("salom", "tovar singan keldi"))
	if !strings.Contains(got, "YT1") || !strings.Contains(got, "tovar singan keldi") || strings.Contains(got, "salom") {
		t.Fatalf("alert noto'g'ri: %s", got)
	}
}
