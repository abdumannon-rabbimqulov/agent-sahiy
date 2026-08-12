package support

import (
	"encoding/json"
	"testing"
)

func msg(id int64, sender, content, text string) Message {
	return Message{ID: id, SenderType: sender, Message: text,
		Content: json.RawMessage(`"` + content + `"`)}
}

func TestImageMessages(t *testing.T) {
	msgs := []Message{
		msg(1, "client", "text", "salom"),
		msg(2, "client", "image", "https://storage.abusahiy.uz/a.jpg"),
		msg(3, "agent", "image", "https://storage.abusahiy.uz/agent.jpg"), // agent — hisobga olinmaydi
		msg(4, "client", "image", "https://storage.abusahiy.uz/b.jpg"),
		msg(5, "client", "image", ""), // URL yo'q — tashlab ketiladi
	}

	got := ImageMessages(msgs, 0)
	if len(got) != 2 {
		t.Fatalf("2 ta rasm kutilgan, keldi %d", len(got))
	}
	if got[0].ID != 4 || got[1].ID != 2 {
		t.Errorf("eng yangisi birinchi bo'lishi kerak, keldi: %d, %d", got[0].ID, got[1].ID)
	}
	if l := ImageMessages(msgs, 1); len(l) != 1 || l[0].ID != 4 {
		t.Errorf("limit ishlamadi: %+v", l)
	}
}

func TestTranscriptRasmniURLsizYozadi(t *testing.T) {
	out, _ := TranscriptTail([]Message{
		msg(1, "client", "image", "https://storage.abusahiy.uz/a.jpg"),
	}, 0)
	if out != "client: [rasm]\n" {
		t.Errorf("transkript = %q", out)
	}
}
