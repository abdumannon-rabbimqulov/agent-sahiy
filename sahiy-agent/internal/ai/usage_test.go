package ai

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// testPrompts — soxta prompt manbai (map).
type testPrompts map[string]string

func (p testPrompts) Get(k string) string { return p[k] }
func (p testPrompts) Keys(prefix string) []string {
	var out []string
	for k := range p {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// fakeBackend — har chaqiruvda belgilangan usage qaytaradi.
type fakeBackend struct {
	u    Usage
	err  error
	seen int
}

func (f *fakeBackend) Name() string { return "fake model-x" }
func (f *fakeBackend) Ready() bool  { return true }
func (f *fakeBackend) Generate(ctx context.Context, system, user string, opt GenOptions) (string, Usage, error) {
	f.seen++
	if f.err != nil {
		return "", Usage{}, f.err
	}
	return "1", f.u, nil
}

func TestMeterUchtaChaqiruvniYigadi(t *testing.T) {
	be := &fakeBackend{u: Usage{Model: "model-x", PromptTokens: 100, CachedTokens: 20, CompletionTokens: 30, Calls: 1}}
	c := New(be, testPrompts{"classify": "router {{CATEGORIES}}", "cat:yetkazish": "..."})

	m := &Meter{}
	ctx := WithMeter(context.Background(), m)
	if _, err := c.Classify(ctx, "client: salom"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ask(ctx, Request{Transcript: "client: salom"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Summarize(ctx, "client: salom", ""); err != nil {
		t.Fatal(err)
	}

	got := m.Usage()
	if got.Calls != 3 || got.PromptTokens != 300 || got.CompletionTokens != 90 || got.CachedTokens != 60 {
		t.Errorf("yig'indi = %+v", got)
	}
	if got.Model != "model-x" {
		t.Errorf("model = %q", got.Model)
	}
	if got.Total() != 390 {
		t.Errorf("Total() = %d, kutilgan 390", got.Total())
	}
}

func TestMetersizChaqiruvIshlaydi(t *testing.T) {
	be := &fakeBackend{u: Usage{PromptTokens: 10, Calls: 1}}
	c := New(be, testPrompts{})
	// ctx'da Meter yo'q — panika bo'lmasligi kerak.
	if _, err := c.Ask(context.Background(), Request{Transcript: "salom"}); err != nil {
		t.Fatal(err)
	}
}

func TestXatoHisobgaOlinmaydi(t *testing.T) {
	be := &fakeBackend{err: context.DeadlineExceeded}
	c := New(be, testPrompts{})
	m := &Meter{}
	ctx := WithMeter(context.Background(), m)
	if _, err := c.Ask(ctx, Request{Transcript: "salom"}); err == nil {
		t.Fatal("xato kutilgan edi")
	}
	if u := m.Usage(); u.Calls != 0 || u.PromptTokens != 0 {
		t.Errorf("muvaffaqiyatsiz so'rov hisobga olinmasligi kerak: %+v", u)
	}
}
