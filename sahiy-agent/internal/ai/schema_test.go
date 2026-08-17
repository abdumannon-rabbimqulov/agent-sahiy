package ai

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestSchemasMatchStructs — sxemadagi maydonlar struct'nikiga aynan mos
// bo'lishi kerak. Maydon nomi bir joyda o'zgarib, ikkinchisida qolib
// ketsa model kutilmagan shakl qaytaradi va parse jim ravishda bo'sh
// natija beradi — shuning uchun bu test bor.
func TestSchemasMatchStructs(t *testing.T) {
	cases := []struct {
		name   string
		schema json.RawMessage
		typ    any
	}{
		{"DecisionSchema", DecisionSchema, Decision{}},
		{"OrderReplySchema", OrderReplySchema, OrderReply{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
				Additional *bool                      `json:"additionalProperties"`
			}
			if err := json.Unmarshal(c.schema, &s); err != nil {
				t.Fatalf("sxema JSON emas: %v", err)
			}
			if s.Additional == nil || *s.Additional {
				t.Error("additionalProperties: false bo'lishi kerak")
			}

			want := jsonFields(c.typ)
			got := keys(s.Properties)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("maydonlar mos emas:\nstruct: %v\nsxema:  %v", want, got)
			}
			req := append([]string(nil), s.Required...)
			sort.Strings(req)
			if !reflect.DeepEqual(want, req) {
				t.Errorf("required to'liq emas:\nkerak: %v\nbor:   %v", want, req)
			}
		})
	}
}

// jsonFields — struct'ning json tegidagi nomlari (`-` bo'lganlari tashqari).
func jsonFields(v any) []string {
	rt := reflect.TypeOf(v)
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
