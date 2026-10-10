package support

import "testing"

func TestDetectLang(t *testing.T) {
	cases := []struct {
		name   string
		msgs   []Message
		lang   string
		script string
	}{
		{
			name: "rus mijoz, oxirida bizning o'zbekcha xabarimiz",
			msgs: []Message{
				{SenderType: "client", Message: "могу видео снять или фото"},
				{SenderType: "agent", Message: "Salom, iltimos, mahsulot rasmini yuboring."},
			},
			lang: LangRus,
		},
		{
			name:   "o'zbekcha lotin",
			msgs:   []Message{{SenderType: "client", Message: "Buyurtmam qachon keladi?"}},
			lang:   LangUzb,
			script: ScriptLat,
		},
		{
			name:   "o'zbekcha kirill",
			msgs:   []Message{{SenderType: "client", Message: "Буюртмам қачон келади?"}},
			lang:   LangUzb,
			script: ScriptCyr,
		},
		{
			name: "faqat rasm — matn yo'q",
			msgs: []Message{{SenderType: "client", Message: "https://cdn.example.com/a-image_picker_1.png"}},
			lang: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectLang(c.msgs)
			if got.Lang != c.lang || got.Script != c.script {
				t.Fatalf("detectLang = %q/%q, kerak %q/%q", got.Lang, got.Script, c.lang, c.script)
			}
		})
	}
}

func TestLangJSON(t *testing.T) {
	if got := langJSON(false, true, ""); got != `{"rus":true,"uzb":false}` {
		t.Errorf("rus: %s", got)
	}
	if got := langJSON(true, false, ScriptCyr); got != `{"alifbo":"kirill","rus":false,"uzb":true}` {
		t.Errorf("uzb kirill: %s", got)
	}
}
