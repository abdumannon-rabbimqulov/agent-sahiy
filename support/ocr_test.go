package support

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// imageServer - testdata dagi rasmni tarqatadigan soxta server.
// Havola rasm ko'rinishida bo'lishi kerak (isImageLink).
func imageServer(t *testing.T, file string) *httptest.Server {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatalf("testdata: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(raw)
	}))
}

// TestReadImageOCR - rasm MODELSIZ, tesseract bilan o'qiladi.
func TestReadImageOCR(t *testing.T) {
	if !OCRAvailable() {
		t.Skip("tesseract o'rnatilmagan")
	}
	srv := imageServer(t, "buyurtma.jpg")
	defer srv.Close()

	got, err := ReadImageOCR(context.Background(), srv.URL+"/rasm.jpg")
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if len(got.OrderSN) != 1 || got.OrderSN[0] != "DG60607041" {
		t.Errorf("buyurtma raqami: %v (matn: %q)", got.OrderSN, got.Text)
	}
	if len(got.Express) != 1 || got.Express[0] != "JT3172404674793" {
		t.Errorf("trek raqami: %v (matn: %q)", got.Express, got.Text)
	}
	if !strings.HasPrefix(got.Model, "tesseract") {
		t.Errorf("model nomi: %q", got.Model)
	}
}

// TestOCRFirst - zanjir avval OCR'ni sinaydi: OCR topsa, MODELGA
// umuman bormaydi (token sarflanmaydi).
func TestOCRFirst(t *testing.T) {
	if !OCRAvailable() {
		t.Skip("tesseract o'rnatilmagan")
	}
	srv := imageServer(t, "buyurtma.jpg")
	defer srv.Close()

	// Model chaqirilsa test yiqiladi.
	var visionCalls int
	groq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visionCalls++
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer groq.Close()
	os.Setenv("GROQ_API_KEY", "test")
	os.Setenv("GROQ_BASE_URL", groq.URL)
	defer func() { os.Unsetenv("GROQ_API_KEY"); os.Unsetenv("GROQ_BASE_URL") }()

	msgs := []Message{{ID: 1, SenderType: "client", Message: srv.URL + "/rasm.jpg"}}
	got, usage, err := ReadNumbersFromMessages(context.Background(), msgs)
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if visionCalls != 0 {
		t.Errorf("OCR topgan bo'lsa ham model %d marta chaqirildi", visionCalls)
	}
	if usage.Calls != 0 || usage.PromptTokens != 0 {
		t.Errorf("token sarflandi: %+v", usage)
	}
	if len(got.OrderSN) != 1 || got.OrderSN[0] != "DG60607041" {
		t.Errorf("raqam topilmadi: %+v", got)
	}
}

// TestOCRDisabled - OCR_ENABLED=false bo'lsa rasm to'g'ridan-to'g'ri
// modelga ketadi.
func TestOCRDisabled(t *testing.T) {
	srv := imageServer(t, "buyurtma.jpg")
	defer srv.Close()

	var visionCalls int
	groq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visionCalls++
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"order_sn\":[\"DG1\"],\"express_num\":[],\"matn\":\"\"}"}}]}`))
	}))
	defer groq.Close()
	os.Setenv("GROQ_API_KEY", "test")
	os.Setenv("GROQ_BASE_URL", groq.URL)
	os.Setenv("OCR_ENABLED", "false")
	defer func() {
		os.Unsetenv("GROQ_API_KEY")
		os.Unsetenv("GROQ_BASE_URL")
		os.Unsetenv("OCR_ENABLED")
	}()

	msgs := []Message{{ID: 1, SenderType: "client", Message: srv.URL + "/rasm.jpg"}}
	if _, _, err := ReadNumbersFromMessages(context.Background(), msgs); err != nil {
		t.Fatalf("xato: %v", err)
	}
	if visionCalls != 1 {
		t.Errorf("model %d marta chaqirildi, 1 ta kutilgandi", visionCalls)
	}
}
