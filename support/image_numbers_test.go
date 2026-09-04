package support

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func clientImg(id int64, url string) Message {
	return Message{ID: id, SenderType: "client", Message: url}
}

// TestClientImageLinks - faqat mijoz rasmlari, yangisidan eskisiga.
func TestClientImageLinks(t *testing.T) {
	msgs := []Message{
		clientImg(1, "https://cdn.example.com/chat-images/a.jpg"),
		{ID: 2, SenderType: "client", Message: "buyurtmam qani"},
		{ID: 3, SenderType: "agent", Message: "https://cdn.example.com/chat-images/xodim.png"},
		clientImg(4, "https://cdn.example.com/chat-images/b.png"),
	}
	got := ClientImageLinks(msgs)
	if len(got) != 2 {
		t.Fatalf("2 ta rasm kutildi: %v", got)
	}
	if !strings.HasSuffix(got[0], "b.png") {
		t.Errorf("eng oxirgi rasm birinchi bo'lishi kerak: %v", got)
	}
	if !HasClientImage(msgs) {
		t.Error("HasClientImage false qaytardi")
	}
	if HasClientImage(msgs[1:2]) {
		t.Error("matnli xabar rasm deb hisoblandi")
	}
}

// TestReadNumbersNoImages - rasm bo'lmasa ErrNoImages.
func TestReadNumbersNoImages(t *testing.T) {
	msgs := []Message{{ID: 1, SenderType: "client", Message: "salom"}}
	_, _, err := ReadNumbersFromMessages(context.Background(), msgs)
	if !errors.Is(err, ErrNoImages) {
		t.Fatalf("ErrNoImages kutildi: %v", err)
	}
}

// visionServer - Groq o'rniga javob beradigan soxta server.
func visionServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body visionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("so'rovni o'qish: %v", err)
		}
		// Rasm havolasi so'rovda bo'lishi shart.
		var hasImage bool
		for _, c := range body.Messages[0].Content {
			if c.Type == "image_url" && c.ImageURL != nil && c.ImageURL.URL != "" {
				hasImage = true
			}
		}
		if !hasImage {
			t.Error("so'rovda rasm havolasi yo'q")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "test-vision",
			"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
}

func withGroq(t *testing.T, url string) Groq {
	t.Helper()
	os.Setenv("GROQ_API_KEY", "test")
	os.Setenv("GROQ_BASE_URL", url)
	// Bu testlar ko'ruvchi MODEL yo'lini tekshiradi — OCR o'chiriladi,
	// aks holda soxta havolalarni yuklashga urinib vaqt ketadi.
	os.Setenv("OCR_ENABLED", "false")
	t.Cleanup(func() {
		os.Unsetenv("GROQ_API_KEY")
		os.Unsetenv("GROQ_BASE_URL")
		os.Unsetenv("OCR_ENABLED")
	})
	return GroqFromEnv()
}

// TestReadImageNumbers - rasmdan raqamlar o'qiladi.
func TestReadImageNumbers(t *testing.T) {
	srv := visionServer(t, `{"order_sn":["DG60607041"],"express_num":["JT3172404674793"],"matn":"Buyurtma DG60607041"}`)
	defer srv.Close()
	g := withGroq(t, srv.URL)

	got, u, err := ReadImageNumbers(context.Background(), g, "https://cdn.example.com/chat-images/a.jpg")
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if len(got.OrderSN) != 1 || got.OrderSN[0] != "DG60607041" {
		t.Errorf("buyurtma raqami: %v", got.OrderSN)
	}
	if len(got.Express) != 1 || got.Express[0] != "JT3172404674793" {
		t.Errorf("trek raqami: %v", got.Express)
	}
	if got.Empty() {
		t.Error("Empty() true qaytardi")
	}
	if u.Calls != 1 || u.PromptTokens != 10 {
		t.Errorf("usage yig'ilmadi: %+v", u)
	}
}

// TestReadImageNumbersEmpty - rasmda raqam bo'lmasa ErrNoNumbersInImage.
func TestReadImageNumbersEmpty(t *testing.T) {
	srv := visionServer(t, `{"order_sn":[],"express_num":[],"matn":"mushukcha rasmi"}`)
	defer srv.Close()
	withGroq(t, srv.URL)

	msgs := []Message{clientImg(1, "https://cdn.example.com/chat-images/cat.jpg")}
	got, _, err := ReadNumbersFromMessages(context.Background(), msgs)
	if !errors.Is(err, ErrNoNumbersInImage) {
		t.Fatalf("ErrNoNumbersInImage kutildi: %v", err)
	}
	if !got.Empty() || got.Images != 1 {
		t.Errorf("natija: %+v", got)
	}
}

// TestReadImageNumbersFromText - model ro'yxatni tashlab ketsa ham raqam
// rasmdagi matndan (kod bilan) topiladi.
func TestReadImageNumbersFromText(t *testing.T) {
	srv := visionServer(t, `{"order_sn":[],"express_num":[],"matn":"Заказ ДГ 60607041 трек JT3172404674793"}`)
	defer srv.Close()
	g := withGroq(t, srv.URL)

	got, _, err := ReadImageNumbers(context.Background(), g, "https://cdn.example.com/chat-images/a.jpg")
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if len(got.OrderSN) != 1 || got.OrderSN[0] != "DG60607041" {
		t.Errorf("kirillcha ДГ o'girilmadi: %v", got.OrderSN)
	}
	if len(got.Express) != 1 {
		t.Errorf("trek matndan olinmadi: %v", got.Express)
	}
}

// TestReadImageNumbersNotJSON - model JSON o'rniga matn qaytarsa ham
// raqamlar qutqariladi.
func TestReadImageNumbersNotJSON(t *testing.T) {
	srv := visionServer(t, `Rasmda DG60607041 raqami bor`)
	defer srv.Close()
	g := withGroq(t, srv.URL)

	got, _, err := ReadImageNumbers(context.Background(), g, "https://cdn.example.com/chat-images/a.jpg")
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if len(got.OrderSN) != 1 || got.OrderSN[0] != "DG60607041" {
		t.Errorf("xom matndan olinmadi: %+v", got)
	}
}

// TestReadNumbersStopsAtFirstHit - birinchi raqam topilgan rasmda to'xtaydi.
func TestReadNumbersStopsAtFirstHit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{
				"content": `{"order_sn":["DG60607041"],"express_num":[],"matn":""}`}}},
		})
	}))
	defer srv.Close()
	withGroq(t, srv.URL)

	msgs := []Message{
		clientImg(1, "https://cdn.example.com/chat-images/a.jpg"),
		clientImg(2, "https://cdn.example.com/chat-images/b.jpg"),
	}
	got, _, err := ReadNumbersFromMessages(context.Background(), msgs)
	if err != nil {
		t.Fatalf("xato: %v", err)
	}
	if calls != 1 {
		t.Errorf("%d marta so'rov ketdi, 1 ta kutildi", calls)
	}
	if got.Images != 1 {
		t.Errorf("ko'rilgan rasm: %d", got.Images)
	}
}
