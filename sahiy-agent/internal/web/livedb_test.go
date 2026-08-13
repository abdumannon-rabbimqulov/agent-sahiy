//go:build webcheck

// Haqiqiy Postgres'ga qarshi tirik sinov: AI ni o'chirish/yoqish va AI
// javobini tekshirib tasdiqlash. Ishga tushirish:
//
//	WEBCHECK_DSN='postgres://...' go test -tags webcheck -run TestBoshqaruv -v ./internal/web/
package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"sahiy-agent/internal/category"
	"sahiy-agent/internal/db"
	"sahiy-agent/internal/escalation"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/settings"
	"sahiy-agent/internal/store"
)

func TestBoshqaruv(t *testing.T) {
	dsn := os.Getenv("WEBCHECK_DSN")
	if dsn == "" {
		t.Skip("WEBCHECK_DSN yo'q")
	}
	gdb, err := db.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	st, set := store.New(gdb), settings.New(gdb)
	// Ma'lum holatdan boshlaymiz (test qayta ishga tushirilsa ham).
	if err := set.Set(settings.AIEnabled, true); err != nil {
		t.Fatal(err)
	}
	if err := set.Set(settings.AutoReply, false); err != nil {
		t.Fatal(err)
	}
	// Init mavjud qiymatni BEKOR QILMASLIGI kerak: .env faqat birinchi
	// ishga tushishda ta'sir qiladi, keyin dashboarddagi tanlov ustun.
	if err := set.Init(settings.AIEnabled, false); err != nil {
		t.Fatal(err)
	}
	if !set.Bool(settings.AIEnabled, false) {
		t.Fatal("Init dashboarddagi tanlovni bekor qildi")
	}

	// Yuborilgan javoblarni ushlab qolamiz (haqiqiy API'ga bormaydi).
	var sentTo int64
	var sentText string
	srv := New(st, category.New(gdb), escalation.New(gdb), set, Options{
		SendReply: func(conversationID int64, text string) error {
			sentTo, sentText = conversationID, text
			return nil
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	do := func(method, path, body string) (int, string) {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, strings.TrimSpace(string(out))
	}

	// --- sozlamalar ---
	code, body := do("GET", "/api/settings", "")
	t.Logf("GET /api/settings → %d %s", code, body)
	var cur map[string]bool
	if err := json.Unmarshal([]byte(body), &cur); err != nil || !cur["ai_enabled"] || cur["auto_reply"] {
		t.Fatalf("boshlang'ich sozlama: %s (err=%v)", body, err)
	}

	// AI ni o'chiramiz.
	if code, body = do("PUT", "/api/settings", `{"ai_enabled":false}`); code != 200 {
		t.Fatalf("PUT → %d %s", code, body)
	}
	if !set.Bool(settings.AIEnabled, true) == false {
		t.Error("ai_enabled false bo'lishi kerak")
	}
	t.Logf("AI o'chirildi → %s", body)
	if got := set.Bool(settings.AIEnabled, true); got {
		t.Error("kesh yangilanmadi — AI hali ham yoqilgan ko'rinadi")
	}

	// Noma'lum kalit rad etilishi kerak.
	if code, _ = do("PUT", "/api/settings", `{"drop_db":true}`); code != http.StatusBadRequest {
		t.Errorf("noma'lum kalit uchun 400 kutilgan, keldi %d", code)
	}

	// --- javobni tasdiqlash ---
	draft := &models.Interaction{
		ConversationID: 900001, ClientID: 55, ClientName: "Test",
		ClientMessage: "buyurtmam qayerda?",
		AIReply:       "Buyurtmangiz yo'lda, 2-3 kunda yetib boradi.",
		Status:        models.StatusAIDraft,
	}
	if err := st.Append(draft); err != nil {
		t.Fatal(err)
	}
	path := "/api/interactions/" + itoa(draft.ID)

	code, body = do("POST", path+"/send", "")
	if code != 200 {
		t.Fatalf("tasdiqlash → %d %s", code, body)
	}
	if sentTo != 900001 || sentText != draft.AIReply {
		t.Errorf("mijozga noto'g'ri yuborildi: #%d %q", sentTo, sentText)
	}
	after, _ := st.Get(draft.ID)
	if after.Status != models.StatusAISent || !after.Sent || after.HandledBy != "AI + admin tasdiqladi" {
		t.Errorf("holat yangilanmadi: %+v", after)
	}
	t.Logf("✓ tasdiqlandi → holat=%s, kim=%s", after.Status, after.HandledBy)

	// Ikkinchi marta yuborib bo'lmasligi kerak.
	if code, body = do("POST", path+"/send", ""); code != http.StatusConflict {
		t.Errorf("takroriy yuborish uchun 409 kutilgan, keldi %d %s", code, body)
	}

	// --- rad etish ---
	draft2 := &models.Interaction{
		ConversationID: 900002, ClientName: "Test2",
		AIReply: "Noto'g'ri javob", Status: models.StatusAIDraft,
	}
	if err := st.Append(draft2); err != nil {
		t.Fatal(err)
	}
	if code, body = do("POST", "/api/interactions/"+itoa(draft2.ID)+"/reject", ""); code != 200 {
		t.Fatalf("rad etish → %d %s", code, body)
	}
	rej, _ := st.Get(draft2.ID)
	if rej.Status != models.StatusRejected || rej.Sent {
		t.Errorf("rad etish holati: %+v", rej)
	}
	t.Logf("✓ rad etildi → holat=%s", rej.Status)

	// Topilmagan yozuv.
	if code, _ = do("POST", "/api/interactions/99999999/send", ""); code != http.StatusNotFound {
		t.Errorf("404 kutilgan, keldi %d", code)
	}
}

func itoa(v uint) string {
	return strings.TrimSpace(string(json.RawMessage(jsonNum(v))))
}

func jsonNum(v uint) []byte {
	b, _ := json.Marshal(v)
	return b
}
