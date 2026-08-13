//go:build costcheck

// Vaqtinchalik tekshiruv: xarajat kesimlarining SQL'ini haqiqiy Postgres'da
// sinaydi. Ishga tushirish:
//
//	COSTCHECK_DSN=... go test -tags costcheck -run TestKesimlar -v .
package main

import (
	"os"
	"testing"
	"time"

	"sahiy-agent/internal/db"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/store"
)

func TestKesimlar(t *testing.T) {
	dsn := os.Getenv("COSTCHECK_DSN")
	if dsn == "" {
		t.Skip("COSTCHECK_DSN yo'q")
	}
	gdb, err := db.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(gdb)

	escID := int64(777)
	now := time.Now()
	rows := []models.Interaction{
		// Mijoz 1, suhbat 100 — ikki javob (biri eskalatsiya)
		{CreatedAt: now.Add(-2 * time.Hour), ConversationID: 100, ClientID: 1, ClientName: "Ali", Title: "Buyurtma",
			Status: models.StatusAISent, Model: "gpt-4o-mini", PromptTokens: 1000, CachedTokens: 200, CompletionTokens: 100, AICalls: 2, CostUSD: 0.0002},
		{CreatedAt: now.Add(-1 * time.Hour), ConversationID: 100, ClientID: 1, ClientName: "Ali Valiyev", Title: "Buyurtma",
			Status: models.StatusPending, EscalationID: &escID, Model: "gpt-4o-mini", PromptTokens: 2000, CompletionTokens: 300, AICalls: 3, CostUSD: 0.0005},
		// Mijoz 1, boshqa suhbat
		{CreatedAt: now.Add(-30 * time.Minute), ConversationID: 101, ClientID: 1, ClientName: "Ali Valiyev",
			Status: models.StatusAISent, Model: "gpt-4o-mini", PromptTokens: 500, CompletionTokens: 50, AICalls: 2, CostUSD: 0.0001},
		// Mijoz 2
		{CreatedAt: now.Add(-10 * time.Minute), ConversationID: 200, ClientID: 2, ClientName: "Vali",
			Status: models.StatusStaffSent, Model: "gpt-4o-mini", PromptTokens: 300, CompletionTokens: 30, AICalls: 1, CostUSD: 0.00007},
		// AI tegmagan yozuv — hech qaysi kesimga kirmasligi kerak
		{CreatedAt: now.Add(-5 * time.Minute), ConversationID: 300, ClientID: 3, ClientName: "Hasan",
			Status: models.StatusFailed, AICalls: 0, CostUSD: 0},
	}
	for i := range rows {
		if err := st.Append(&rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	const wantCost = 0.0002 + 0.0005 + 0.0001 + 0.00007
	near := func(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }

	// --- kunlik ---
	daily, err := st.Daily(30)
	if err != nil {
		t.Fatal("Daily:", err)
	}
	var dayCost float64
	for _, d := range daily {
		dayCost += d.CostUSD
	}
	t.Logf("kunlik: %+v", daily)

	// --- mijozlar ---
	clients, err := st.ByClient(30, 50, "cost")
	if err != nil {
		t.Fatal("ByClient:", err)
	}
	var cliCost float64
	for _, c := range clients {
		cliCost += c.CostUSD
		t.Logf("mijoz %d %q: suhbat=%d javob=%d token=%d/%d $%.6f oxirgi=%s",
			c.ClientID, c.ClientName, c.Conversations, c.Replies, c.PromptTokens, c.CompletionTokens, c.CostUSD, c.LastAt.Format("15:04"))
	}
	if len(clients) != 2 {
		t.Errorf("2 ta mijoz kutilgan (AI tegmagani chiqmasin), keldi %d", len(clients))
	}
	if clients[0].ClientID != 1 || clients[0].Conversations != 2 || clients[0].Replies != 3 {
		t.Errorf("eng qimmat mijoz noto'g'ri: %+v", clients[0])
	}
	if clients[0].ClientName != "Ali Valiyev" {
		t.Errorf("eng oxirgi ism olinishi kerak, keldi %q", clients[0].ClientName)
	}

	// --- muammolar ---
	convs, err := st.ByConversation(30, 50, "cost")
	if err != nil {
		t.Fatal("ByConversation:", err)
	}
	var convCost float64
	for _, c := range convs {
		convCost += c.CostUSD
		t.Logf("suhbat #%d %q: javob=%d token=%d/%d $%.6f holat=%s eskalatsiya=%v",
			c.ConversationID, c.ClientName, c.Replies, c.PromptTokens, c.CompletionTokens, c.CostUSD, c.Status, c.Escalated)
	}
	if len(convs) != 3 {
		t.Errorf("3 ta suhbat kutilgan, keldi %d", len(convs))
	}
	top := convs[0]
	if top.ConversationID != 100 || top.Replies != 2 || !top.Escalated || top.Status != models.StatusPending {
		t.Errorf("eng qimmat suhbat noto'g'ri: %+v", top)
	}
	if top.PromptTokens != 3000 || top.CompletionTokens != 400 {
		t.Errorf("suhbat tokenlari noto'g'ri: %d / %d", top.PromptTokens, top.CompletionTokens)
	}

	// --- asosiy invariant: uchala kesim yig'indisi bir xil ---
	if !near(dayCost, wantCost) || !near(cliCost, wantCost) || !near(convCost, wantCost) {
		t.Errorf("yig'indilar mos emas: kunlik=%.8f mijoz=%.8f muammo=%.8f (kutilgan %.8f)",
			dayCost, cliCost, convCost, wantCost)
	}

	// --- saralash: last ---
	byLast, err := st.ByConversation(30, 50, "last")
	if err != nil {
		t.Fatal(err)
	}
	if byLast[0].ConversationID != 200 {
		t.Errorf("sort=last da eng yangi suhbat (#200) tepada bo'lishi kerak, keldi #%d", byLast[0].ConversationID)
	}

	// --- davr cheklovi ---
	if all, err := st.ByClient(0, 50, "cost"); err != nil || len(all) != 2 {
		t.Errorf("days=0 (hammasi): %d ta, err=%v", len(all), err)
	}
	// Statistika ham shu summani ko'rsatsin.
	if s, err := st.Stats(); err != nil || !near(s.CostToday, wantCost) {
		t.Errorf("Stats.CostToday = %.8f, kutilgan %.8f (err=%v)", s.CostToday, wantCost, err)
	}
}
