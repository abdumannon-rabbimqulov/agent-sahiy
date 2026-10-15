package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Test chat endpointining tekshiruv yo'li: zanjir ishga tushishidan
// OLDIN rad etilishi kerak bo'lgan so'rovlar. Bu yerda tashqi tizimga
// ham, modelga ham chiqilmaydi.
func TestAgentSandboxHandlerRejectsBadBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"JSON emas", `salom`, "body JSON emas"},
		{"messages yo'q", `{"client_id":1}`, "messages majburiy"},
		{"messages bo'sh", `{"client_id":1,"messages":[]}`, "messages majburiy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/agent/sandbox", strings.NewReader(c.body))
			w := httptest.NewRecorder()
			agentSandboxHandler(w, r)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("400 kutilgandi, %d keldi: %s", w.Code, w.Body.String())
			}
			var got struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("javob JSON emas: %v", err)
			}
			if got.Error != c.want {
				t.Errorf("xato matni %q, kutilgani %q", got.Error, c.want)
			}
		})
	}
}
