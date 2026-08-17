package prompts_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	if err := gdb.Where("1 = 1").Delete(&models.PromptBackup{}).Error; err != nil {
		t.Fatalf("nusxalar jadvalini tozalash: %v", err)
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

// TestBackupRestore — tahrir qaytarib olinadimi. Versiyalash olib
// tashlangandan keyin "Saqlash" qaytarib bo'lmaydigan amal edi; shu
// yo'l aynan o'sha holatni yopadi.
func TestBackupRestore(t *testing.T) {
	svc := newService(t)
	if _, err := svc.Create("cat:test", "birinchi matn", true); err != nil {
		t.Fatalf("yaratilmadi: %v", err)
	}

	second := "ikkinchi matn"
	if _, err := svc.Update("cat:test", prompts.Update{Content: &second}); err != nil {
		t.Fatalf("yangilanmadi: %v", err)
	}

	backups, err := svc.Backups("cat:test")
	if err != nil {
		t.Fatalf("nusxalar o'qilmadi: %v", err)
	}
	if len(backups) != 1 || backups[0].Content != "birinchi matn" {
		t.Fatalf("kutilgan 1 ta nusxa (\"birinchi matn\"), olindi: %+v", backups)
	}

	p, err := svc.Restore("cat:test", backups[0].ID)
	if err != nil {
		t.Fatalf("tiklanmadi: %v", err)
	}
	if p.Content != "birinchi matn" {
		t.Errorf("tiklangan matn: %q", p.Content)
	}
	// Tiklashning o'zi ham nusxaga tushadi — orqaga qaytish yo'li ochiq.
	if backups, _ = svc.Backups("cat:test"); len(backups) != 2 {
		t.Errorf("tiklashdan keyin 2 ta nusxa kutilgan, bor: %d", len(backups))
	}
	if svc.Get("cat:test") != "birinchi matn" {
		t.Errorf("kesh yangilanmadi: %q", svc.Get("cat:test"))
	}
}

// TestBackupLimit — nusxalar cheksiz to'planib ketmasligi kerak.
func TestBackupLimit(t *testing.T) {
	svc := newService(t)
	if _, err := svc.Create("cat:limit", "0", true); err != nil {
		t.Fatalf("yaratilmadi: %v", err)
	}
	for i := 1; i <= 8; i++ {
		text := strings.Repeat("x", i)
		if _, err := svc.Update("cat:limit", prompts.Update{Content: &text}); err != nil {
			t.Fatalf("yangilanmadi: %v", err)
		}
	}
	backups, err := svc.Backups("cat:limit")
	if err != nil {
		t.Fatalf("nusxalar: %v", err)
	}
	if len(backups) != 5 {
		t.Errorf("5 ta nusxa kutilgan, bor: %d", len(backups))
	}
	// Eng yangisi birinchi turadi.
	if backups[0].Content != strings.Repeat("x", 7) {
		t.Errorf("tartib buzilgan, birinchi nusxa: %q", backups[0].Content)
	}
}

// TestRestoreWrongKey — boshqa promptning nusxasini tiklab bo'lmaydi.
func TestRestoreWrongKey(t *testing.T) {
	svc := newService(t)
	if _, err := svc.Create("cat:a", "a-matn", true); err != nil {
		t.Fatalf("yaratilmadi: %v", err)
	}
	if _, err := svc.Create("cat:b", "b-matn", true); err != nil {
		t.Fatalf("yaratilmadi: %v", err)
	}
	next := "a-matn-2"
	if _, err := svc.Update("cat:a", prompts.Update{Content: &next}); err != nil {
		t.Fatalf("yangilanmadi: %v", err)
	}
	backups, _ := svc.Backups("cat:a")
	if len(backups) == 0 {
		t.Fatal("nusxa yaratilmadi")
	}
	if _, err := svc.Restore("cat:b", backups[0].ID); !errors.Is(err, prompts.ErrInvalid) {
		t.Errorf("ErrInvalid kutilgan, olindi: %v", err)
	}
}

// TestWarn — placeholder ogohlantirishlari. Saqlashni to'xtatmaydi,
// lekin "{{ORDERS}} yozilmagan" holatni sezdirib turadi.
func TestWarn(t *testing.T) {
	svc := prompts.NewService(nil, nil)
	svc.SetMeta(prompts.Meta{
		Placeholders: map[string][]string{"block:order": {"{{ORDERS}}"}},
		Known:        []string{"{{DATE}}", "{{ORDERS}}", "{{CATEGORY}}"},
	})

	if w := svc.Warn("block:order", "Buyurtmalar: {{ORDERS}}"); len(w) != 0 {
		t.Errorf("ogohlantirish kutilmagan: %v", w)
	}
	if w := svc.Warn("block:order", "Buyurtmalar ro'yxati"); len(w) != 1 {
		t.Errorf("{{ORDERS}} yo'qligi aytilishi kerak, olindi: %v", w)
	}
	if w := svc.Warn("base", "Bugun {{SANA}}"); len(w) != 1 ||
		!strings.Contains(w[0], "{{SANA}}") {
		t.Errorf("noma'lum belgi aytilishi kerak, olindi: %v", w)
	}
	if w := svc.Warn("base", "Bugun {{DATE}}"); len(w) != 0 {
		t.Errorf("tanish belgi ogohlantirmasligi kerak: %v", w)
	}
}

// TestHandlerExtras — yangi endpointlar: meta, nusxalar, tiklash, sinov.
func TestHandlerExtras(t *testing.T) {
	svc := newService(t)
	svc.SetMeta(prompts.Meta{
		Required:     []string{"base"},
		Optional:     []string{"block:order"},
		Placeholders: map[string][]string{"block:order": {"{{ORDERS}}"}},
		Known:        []string{"{{ORDERS}}"},
	})

	h := prompts.NewHandler(svc)
	// Sinov funksiyasi o'rniga soxta: haqiqiy model kerak emas.
	h.SetTry(func(_ context.Context, req prompts.TryRequest) (any, error) {
		return map[string]string{"key": req.Key, "content": req.Content}, nil
	})
	mux := http.NewServeMux()
	h.Register(mux)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// meta
	rec := do("GET", "/api/prompts-meta", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "{{ORDERS}}") {
		t.Fatalf("meta: %d — %s", rec.Code, rec.Body)
	}

	// Kutilgan placeholder yozilmasa — saqlanadi, lekin ogohlantiriladi.
	rec = do("POST", "/api/prompts", `{"key":"block:order","content":"Buyurtmalar"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("yaratish: %d — %s", rec.Code, rec.Body)
	}
	var created struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("javob JSON emas: %v", err)
	}
	if len(created.Warnings) != 1 {
		t.Errorf("{{ORDERS}} yo'qligi haqida ogohlantirish kutilgan: %+v", created.Warnings)
	}

	// Tahrir → nusxa paydo bo'ladi.
	if rec = do("PUT", "/api/prompts/block:order",
		`{"content":"Buyurtmalar: {{ORDERS}}"}`); rec.Code != http.StatusOK {
		t.Fatalf("tahrir: %d — %s", rec.Code, rec.Body)
	}
	rec = do("GET", "/api/prompt-backups/block:order", "")
	var backups []models.PromptBackup
	if err := json.Unmarshal(rec.Body.Bytes(), &backups); err != nil {
		t.Fatalf("nusxalar JSON emas: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("1 ta nusxa kutilgan: %+v", backups)
	}

	// Tiklash.
	body := `{"id":` + strconv.FormatUint(uint64(backups[0].ID), 10) + `}`
	if rec = do("POST", "/api/prompt-restore/block:order", body); rec.Code != http.StatusOK {
		t.Fatalf("tiklash: %d — %s", rec.Code, rec.Body)
	}
	if got := svc.Get("block:order"); got != "Buyurtmalar" {
		t.Errorf("tiklangandan keyin kesh: %q", got)
	}

	// Sinov: bo'sh murojaat — 400; to'ldirilgani — 200.
	if rec = do("POST", "/api/prompt-try/base", `{"content":"a","transcript":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bo'sh sinov: %d", rec.Code)
	}
	if rec = do("POST", "/api/prompt-try/base",
		`{"content":"a","transcript":"salom"}`); rec.Code != http.StatusOK {
		t.Errorf("sinov: %d — %s", rec.Code, rec.Body)
	}

	// Sinov ulanmagan bo'lsa — 503.
	bare := http.NewServeMux()
	prompts.NewHandler(svc).Register(bare)
	req := httptest.NewRequest("POST", "/api/prompt-try/base", strings.NewReader(`{"content":"a","transcript":"b"}`))
	rec = httptest.NewRecorder()
	bare.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("ulanmagan sinov: %d", rec.Code)
	}
}
