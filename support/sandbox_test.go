package support

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

// Sandbox'ning asosiy va'dasi: hech qanday iz qoldirmaslik. Shuning
// uchun testlar baza ULANMAGAN holda (DB == nil) yuriladi — agar
// zanjir yo'lida biror yozuv qolgan bo'lsa, test panika bilan tushadi.

func TestSandboxMessagesTrimsAndTags(t *testing.T) {
	got := sandboxMessages([]SandboxMessage{
		{Type: "client", Message: "  salom  "},
		{Type: "agent", Message: ""}, // bo'sh — tashlanadi
		{Type: "agent", Message: "Assalomu alaykum"},
		{Type: "boshqa", Message: "noma'lum rol"}, // client emas — agent
		{Type: "client", Message: "SN123"},
	})
	if len(got) != 4 {
		t.Fatalf("4 ta xabar kutilgandi, %d keldi", len(got))
	}
	if got[0].Message != "salom" {
		t.Errorf("bo'shliq kesilmadi: %q", got[0].Message)
	}
	if !got[0].FromClient() || got[1].FromClient() {
		t.Errorf("rollar noto'g'ri: %+v", got)
	}
	if !got[2].FromAgent() {
		t.Errorf("noma'lum rol agent bo'lishi kerak: %+v", got[2])
	}
	if got[0].ID == got[1].ID {
		t.Errorf("id'lar takrorlandi: %+v", got)
	}
}

// HistoryLimit() dan ortig'i kesiladi — haqiqiy zanjir ham shunchasini
// ko'radi, aks holda test natijasi haqiqiydan farq qilardi.
func TestSandboxMessagesRespectsHistoryLimit(t *testing.T) {
	var in []SandboxMessage
	for i := 0; i < HistoryLimit()+5; i++ {
		in = append(in, SandboxMessage{Type: "client", Message: "xabar"})
	}
	if got := len(sandboxMessages(in)); got != HistoryLimit() {
		t.Fatalf("%d kutilgandi, %d keldi", HistoryLimit(), got)
	}
}

func TestRunChainSandboxNeedsClientLastWord(t *testing.T) {
	ctx := context.Background()
	cases := map[string][]SandboxMessage{
		"bo'sh ro'yxat": nil,
		"faqat bo'sh matn": {
			{Type: "client", Message: "   "},
		},
		"oxirgi so'z bizda": {
			{Type: "client", Message: "salom"},
			{Type: "agent", Message: "Assalomu alaykum"},
		},
	}
	for name, msgs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := RunChainSandbox(ctx, 1, 2, msgs); !errors.Is(err, ErrSandboxNoMessages) {
				t.Fatalf("ErrSandboxNoMessages kutilgandi, keldi: %v", err)
			}
		})
	}
}

// detectIssues baza nil bo'lganda ro'yxatni baribir boyitadi, lekin
// hech qanday muammo yozuvi ochmaydi — sandbox aynan shunga tayanadi.
// (OpenLateShipmentIssues'ning nil-baza yo'li order_check_issue_test.go
// da tekshirilgan.)
func TestDetectIssuesWithoutDBOpensNothing(t *testing.T) {
	orders := []AdminkaOrder{{OrderSN: "SN1", Status: 3}, {OrderSN: "SN2", Status: 7}}
	views, fresh := detectIssues(nil, orders, 10, 20)
	if len(views) != len(orders) {
		t.Fatalf("%d ko'rinish kutilgandi, %d keldi", len(orders), len(views))
	}
	if len(fresh) != 0 {
		t.Fatalf("sandbox'da muammo ochilmasligi kerak, %d ta ochildi", len(fresh))
	}
	for _, v := range views {
		if v.InReview {
			t.Errorf("%s: InReview yoqilgan — yozuv ochilgan bo'lishi mumkin", v.OrderSN)
		}
	}
}

// setAgentEnabled - sozlamalar keshini to'g'ridan-to'g'ri yozadi
// (baza testda ulanmagan). Tozalash funksiyasi qaytadi.
func setAgentEnabled(t *testing.T, on bool) {
	t.Helper()
	settingsCache.Lock()
	settingsCache.val = map[string]string{SettingAgentEnabled: strconv.FormatBool(on)}
	settingsCache.at = time.Now()
	settingsCache.Unlock()
	t.Cleanup(func() {
		settingsCache.Lock()
		settingsCache.val, settingsCache.at = map[string]string{}, time.Time{}
		settingsCache.Unlock()
	})
}

// Test chat `agent_enabled` o'chirilganda ham ishlaydi: u mijozga hech
// narsa yubormaydi va bazaga yozmaydi, shuning uchun o'chirilgan agent
// unga to'sqinlik qilmasligi kerak. Ilgari panelda "AI agent
// o'chirilgan" degan xato chiqib, test chat umuman ishlamay qolardi.
func TestRunChainSandboxIgnoresAgentDisabled(t *testing.T) {
	setAgentEnabled(t, false)

	// Zanjir modelga chiqib ketmasin: kontekst oldindan bekor qilinadi.
	// Tekshiruv shoxi undan OLDIN turadi, shuning uchun eski xatti-harakat
	// bo'lganda bu yerda ErrAgentDisabled qaytardi.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RunChainSandbox(ctx, 1, 0, []SandboxMessage{
		{Type: "client", Message: "salom"},
	})
	if errors.Is(err, ErrAgentDisabled) {
		t.Fatal("test chat o'chirilgan agent sababli to'xtadi")
	}
}

// Oddiy (sandbox emas) zanjir esa eskicha: o'chirilgan bo'lsa ishlamaydi.
func TestRunChainRespectsAgentDisabled(t *testing.T) {
	setAgentEnabled(t, false)

	_, err := runChain(context.Background(), 1, 0, runOpts{})
	if !errors.Is(err, ErrAgentDisabled) {
		t.Fatalf("ErrAgentDisabled kutilgandi, keldi: %v", err)
	}
}
