package support

import (
	"encoding/json"
	"testing"
	"time"
)

func at(id int64, sender, text string, ago time.Duration) Message {
	return Message{
		ID: id, SenderType: sender, Message: text,
		Content:   json.RawMessage(`"text"`),
		CreatedAt: time.Now().Add(-ago).Format("2006-01-02 15:04:05"),
	}
}

func ids(msgs []Message) []int64 {
	out := make([]int64, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

func eq(a []int64, b ...int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestWindowFaqatYangilariVaKontekst(t *testing.T) {
	var msgs []Message
	for i := int64(1); i <= 20; i++ {
		msgs = append(msgs, at(i, "client", "x", time.Minute))
	}
	// 17 tagacha javob berilgan → yangilari: 18,19,20 + 2 ta kontekst (16,17)
	got := ids(Window(msgs, 17, 2, 20, 0))
	if !eq(got, 16, 17, 18, 19, 20) {
		t.Errorf("oyna = %v", got)
	}
}

func TestWindowBirinchiKorish(t *testing.T) {
	var msgs []Message
	for i := int64(1); i <= 20; i++ {
		msgs = append(msgs, at(i, "client", "x", time.Minute))
	}
	// afterID=0 → oxirgi before+1 ta
	if got := ids(Window(msgs, 0, 2, 20, 0)); !eq(got, 18, 19, 20) {
		t.Errorf("oyna = %v", got)
	}
}

func TestWindowEskiXabarniTashlaydi(t *testing.T) {
	msgs := []Message{
		at(1, "client", "eski", 50*time.Hour),
		at(2, "agent", "eski javob", 49*time.Hour),
		at(3, "client", "yangi", time.Minute),
	}
	got := ids(Window(msgs, 2, 5, 20, 24*time.Hour))
	if !eq(got, 3) {
		t.Errorf("eski xabarlar tashlanishi kerak, keldi %v", got)
	}
}

func TestWindowMaxCheklovi(t *testing.T) {
	var msgs []Message
	for i := int64(1); i <= 30; i++ {
		msgs = append(msgs, at(i, "client", "x", time.Minute))
	}
	if got := Window(msgs, 0, 50, 10, 0); len(got) != 10 {
		t.Errorf("max=10 kutilgan, keldi %d", len(got))
	}
}

func TestStale(t *testing.T) {
	msgs := []Message{
		at(1, "client", "eski", 50*time.Hour),
		at(2, "agent", "javob", time.Minute), // agent xabari hisobga olinmaydi
	}
	if stale, _ := Stale(msgs, 24*time.Hour); !stale {
		t.Error("2 kunlik mijoz xabari stale bo'lishi kerak")
	}
	if stale, _ := Stale(msgs, 0); stale {
		t.Error("maxAge=0 da tekshiruv o'chiq bo'lishi kerak")
	}
	fresh := []Message{at(1, "client", "yangi", time.Minute)}
	if stale, _ := Stale(fresh, 24*time.Hour); stale {
		t.Error("yangi xabar stale emas")
	}
}

func TestStaleSanaNomalum(t *testing.T) {
	// created_at bo'sh bo'lsa — xabar hech qachon eski deb hisoblanmaydi.
	msgs := []Message{msg(1, "client", "text", "salom")}
	if stale, _ := Stale(msgs, time.Hour); stale {
		t.Error("sana noma'lum bo'lsa stale bo'lmasligi kerak")
	}
}

func TestImageURLs(t *testing.T) {
	msgs := []Message{
		msg(1, "client", "text", "salom"),
		msg(2, "client", "image", "https://storage.abusahiy.uz/a.jpg"),
		msg(3, "agent", "image", "https://storage.abusahiy.uz/agent.jpg"), // agent — hisobga olinmaydi
		msg(4, "client", "image", "https://storage.abusahiy.uz/b.jpg"),
	}
	got := ImageURLs(msgs, 3)
	if len(got) != 2 || got[0] != "https://storage.abusahiy.uz/b.jpg" {
		t.Errorf("havolalar = %v", got)
	}
	if len(ImageURLs(msgs[:1], 3)) != 0 {
		t.Error("rasm bo'lmasa bo'sh bo'lishi kerak")
	}
}

func TestLastClientMessageRasm(t *testing.T) {
	msgs := []Message{
		msg(1, "client", "text", "salom"),
		msg(2, "client", "image", "https://storage.abusahiy.uz/a.jpg"),
	}
	id, text := LastClientMessage(msgs)
	if id != 2 || text != "[rasm]" {
		t.Errorf("id=%d text=%q — rasm URL o'rniga [rasm] kutilgan", id, text)
	}
}
