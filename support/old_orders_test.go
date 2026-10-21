package support

import (
	"testing"
	"time"
)

func TestOnlyRecentOrders(t *testing.T) {
	t.Setenv("OLD_ORDER_DAYS", "")
	ago := func(days int) string {
		return time.Now().AddDate(0, 0, -days).Format(adminkaTimeLayout)
	}
	rows := []AdminkaOrder{
		{OrderSN: "old3", Status: StatusPaid, PaidAt: ago(45)},
		{OrderSN: "old4", Status: StatusWaiting, PaidAt: ago(45)},
		{OrderSN: "old7", Status: StatusShipped, PaidAt: ago(45)},
		{OrderSN: "old12", Status: StatusAbnormal, PaidAt: ago(31)},
		{OrderSN: "new7", Status: StatusShipped, PaidAt: ago(10)},
		{OrderSN: "edge7", Status: StatusShipped, PaidAt: ago(30)},
		{OrderSN: "nodate", Status: StatusShipped},
	}
	got := map[string]bool{}
	for _, o := range onlyRecentOrders(rows) {
		got[o.OrderSN] = true
	}
	want := map[string]bool{"old3": true, "old4": true, "new7": true, "edge7": true, "nodate": true}
	for _, o := range rows {
		if got[o.OrderSN] != want[o.OrderSN] {
			t.Errorf("%s: qoldi=%v, kerak %v", o.OrderSN, got[o.OrderSN], want[o.OrderSN])
		}
	}
}
