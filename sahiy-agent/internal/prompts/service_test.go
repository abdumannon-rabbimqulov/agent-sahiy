package prompts_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"sahiy-agent/internal/db"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/prompts"
)

// newService — test uchun toza bazadagi service. TEST_DATABASE_URL
// berilmagan bo'lsa test o'tkazib yuboriladi (CI'da baza bo'lmasligi mumkin).
func newService(t *testing.T) *prompts.Service {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL berilmagan — integratsiya testi o'tkazib yuborildi")
	}
	gdb, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("bazaga ulanmadi: %v", err)
	}
	if err := gdb.Where("1 = 1").Delete(&models.Prompt{}).Error; err != nil {
		t.Fatalf("jadvalni tozalash: %v", err)
	}
	svc := prompts.NewService(prompts.NewRepository(gdb), []string{"base"})
	if _, err := svc.Create("base", "asosiy prompt", true); err != nil {
		t.Fatalf("base yaratilmadi: %v", err)
	}
	return svc
}

func TestServiceCRUD(t *testing.T) {
	svc := newService(t)

	// Yaratish + kesh darhol yangilanadi.
	if _, err := svc.Create("cat:test", "salom", true); err != nil {
		t.Fatalf("yaratilmadi: %v", err)
	}
	if got := svc.Get("cat:test"); got != "salom" {
		t.Fatalf("kesh yangilanmadi: %q", got)
	}

	// Band kalit — ErrConflict.
	if _, err := svc.Create("cat:test", "yana", true); !errors.Is(err, prompts.ErrConflict) {
		t.Fatalf("ErrConflict kutilgandi, keldi: %v", err)
	}
	// Bo'sh matn — ErrInvalid.
	if _, err := svc.Create("cat:bosh", "  ", true); !errors.Is(err, prompts.ErrInvalid) {
		t.Fatalf("ErrInvalid kutilgandi, keldi: %v", err)
	}

	// Matnni yangilash.
	content := "yangi matn"
	if _, err := svc.Update("cat:test", prompts.Update{Content: &content}); err != nil {
		t.Fatalf("yangilanmadi: %v", err)
	}
	if got := svc.Get("cat:test"); got != content {
		t.Fatalf("kesh eski matnda qoldi: %q", got)
	}

	// Kalitni o'zgartirish.
	newKey := "cat:test2"
	if _, err := svc.Update("cat:test", prompts.Update{NewKey: &newKey}); err != nil {
		t.Fatalf("kalit o'zgarmadi: %v", err)
	}
	if svc.Get("cat:test") != "" || svc.Get(newKey) != content {
		t.Fatal("kalit o'zgargandan keyin kesh noto'g'ri")
	}

	// O'chirib qo'yish — kesh faqat yoqilganlardan yig'iladi.
	off := false
	if _, err := svc.Update(newKey, prompts.Update{Enabled: &off}); err != nil {
		t.Fatalf("o'chirib qo'yilmadi: %v", err)
	}
	if svc.Get(newKey) != "" {
		t.Fatal("o'chiq prompt keshda qoldi")
	}

	// Majburiy promptga tegib bo'lmaydi.
	if err := svc.Delete("base"); !errors.Is(err, prompts.ErrConflict) {
		t.Fatalf("majburiy prompt o'chirildi: %v", err)
	}
	if _, err := svc.Update("base", prompts.Update{Enabled: &off}); !errors.Is(err, prompts.ErrConflict) {
		t.Fatalf("majburiy prompt o'chirib qo'yildi: %v", err)
	}

	// O'chirish.
	if err := svc.Delete(newKey); err != nil {
		t.Fatalf("o'chirilmadi: %v", err)
	}
	if _, err := svc.ByKey(newKey); !errors.Is(err, prompts.ErrNotFound) {
		t.Fatalf("ErrNotFound kutilgandi, keldi: %v", err)
	}
}

func TestHandlerStatusCodes(t *testing.T) {
	svc := newService(t)
	mux := http.NewServeMux()
	prompts.NewHandler(svc).Register(mux)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := do("POST", "/api/prompts", `{"key":"cat:x","content":"matn"}`); rec.Code != http.StatusCreated {
		t.Fatalf("yaratish: %d — %s", rec.Code, rec.Body)
	}
	if rec := do("POST", "/api/prompts", `{"key":"cat:x","content":"matn"}`); rec.Code != http.StatusConflict {
		t.Fatalf("takroriy kalit: %d", rec.Code)
	}
	if rec := do("POST", "/api/prompts", `{"key":"cat:y","content":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bo'sh matn: %d", rec.Code)
	}
	if rec := do("GET", "/api/prompts/cat:x", ""); rec.Code != http.StatusOK {
		t.Fatalf("o'qish: %d", rec.Code)
	}
	if rec := do("PATCH", "/api/prompts/cat:x", `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d — %s", rec.Code, rec.Body)
	}
	if rec := do("DELETE", "/api/prompts/base", ""); rec.Code != http.StatusConflict {
		t.Fatalf("majburiy promptni o'chirish: %d", rec.Code)
	}
	if rec := do("DELETE", "/api/prompts/cat:x", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("o'chirish: %d", rec.Code)
	}
	if rec := do("GET", "/api/prompts/cat:x", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("topilmadi: %d", rec.Code)
	}

	rec := do("GET", "/api/prompts", "")
	var list []models.Prompt
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("ro'yxat JSON emas: %v", err)
	}
	if len(list) != 1 || list[0].Key != "base" {
		t.Fatalf("ro'yxat kutilmagan: %+v", list)
	}
}
