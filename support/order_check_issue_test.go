package support

import (
	"strings"
	"testing"
	"time"
)

// Yo'l bo'yicha ochilgan muammo adminka statusidan kelib chiqmaydi:
// shuning uchun "holat o'zgardi" degan avtomatik yopish uni yopmasligi
// kerak (DropResolvedIssues va ReviewOpenIssues shu darvozaga qaraydi).
func TestShipmentReasonGate(t *testing.T) {
	for _, r := range []string{ReasonLateShipment, ReasonVeryLate, ReasonNoShipDate} {
		if !shipmentReason(r) {
			t.Fatalf("%q yo'l bo'yicha sabab bo'lishi kerak", r)
		}
	}
	for _, r := range []string{ReasonStuckStatus, ReasonOwner, ReasonRegion, ""} {
		if shipmentReason(r) {
			t.Fatalf("%q yo'l bo'yicha sabab EMAS", r)
		}
	}
}

// Baza yo'q bo'lsa muammo ochilmaydi va panika ham bo'lmaydi:
// zanjir baza bilan ishlamaydigan holatlarda ham yuradi.
func TestOpenLateShipmentIssuesWithoutDB(t *testing.T) {
	o := shippedOrder(StatusFinished, 25, 60)
	views := []OrderView{NewOrderView(o)}
	checks := CheckOrders(views, nil, true, true, nil)
	if got := OpenLateShipmentIssues(checks, views, 1, 1); got != nil {
		t.Fatalf("baza yo'q — yozuv ochilmasligi kerak: %v", got)
	}
}

// Guruh xabarida yo'lga chiqqan sana va sabab ko'rinadi: yo'l muddati
// bo'yicha ochilgan muammoda xodimga to'lov sanasidan ko'ra aynan shu
// kerak.
func TestIssuesTextShowsDispatchDateAndReason(t *testing.T) {
	shipped := time.Now().AddDate(0, 0, -25).Format(adminkaTimeLayout)
	is := &OrderIssue{
		OrderSN:     "DG777",
		Status:      StatusFinished,
		StatusLabel: StatusLabel(StatusFinished),
		PaidAt:      time.Now().AddDate(0, 0, -60).Format(adminkaTimeLayout),
		ShippedAt:   shipped,
		Reason:      ReasonLateShipment,
	}
	txt := issuesText([]*OrderIssue{is}, "")
	if !strings.Contains(txt, "Yo'lga chiqqan: "+sanaMatn(shipped)) {
		t.Fatalf("yo'lga chiqqan sana ko'rinishi kerak:\n%s", txt)
	}
	if !strings.Contains(txt, "Sabab: "+reasonText(ReasonLateShipment)) {
		t.Fatalf("sabab ko'rinishi kerak:\n%s", txt)
	}
}

// Sana yo'q bo'lsa qo'shimcha qator chiqmaydi — bo'sh "Yo'lga chiqqan:"
// xodimga hech narsa aytmaydi.
func TestIssuesTextWithoutDispatchDate(t *testing.T) {
	is := &OrderIssue{
		OrderSN:     "DG778",
		Status:      StatusPaid,
		StatusLabel: StatusLabel(StatusPaid),
		PaidAt:      time.Now().AddDate(0, 0, -5).Format(adminkaTimeLayout),
		Reason:      ReasonStuckStatus,
	}
	if txt := issuesText([]*OrderIssue{is}, ""); strings.Contains(txt, "Yo'lga chiqqan") {
		t.Fatalf("sana yo'q — qator chiqmasligi kerak:\n%s", txt)
	}
}
