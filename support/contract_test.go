package support

import "testing"

func TestParseAgentJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want AgentJSON
	}{
		{"oddiy", `{"dashboard":true,"chat":"Salom","promt":4}`, AgentJSON{Dashboard: true, Chat: "Salom"}},
		{"ramka", "```json\n{\"adminka\":true,\"promt\":null}\n```", AgentJSON{Adminka: true}},
		{"matn ichida", `Mana javob: {"chat":"Xo'p {a}","promt":2} tamom`, AgentJSON{Chat: "Xo'p {a}"}},
	}
	for _, c := range cases {
		got, err := ParseAgentJSON(c.raw)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Dashboard != c.want.Dashboard || got.Adminka != c.want.Adminka || got.Chat != c.want.Chat {
			t.Errorf("%s: %+v", c.name, got)
		}
	}

	// promt qiymatlari
	a, _ := ParseAgentJSON(`{"promt":4}`)
	if id, ok := a.NextPromt(); !ok || id != 4 {
		t.Errorf("promt 4 kutilgan: %v %v", id, ok)
	}
	a, _ = ParseAgentJSON(`{"promt":null}`)
	if _, ok := a.NextPromt(); ok {
		t.Error("null da zanjir tugashi kerak")
	}

	if _, err := ParseAgentJSON("umuman JSON emas"); err == nil {
		t.Error("xato kutilgan edi")
	}
}
