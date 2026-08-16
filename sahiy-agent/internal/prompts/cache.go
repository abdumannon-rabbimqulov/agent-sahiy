package prompts

import (
	"sort"
	"strings"
	"sync/atomic"
)

// Cache — promptlarning xotiradagi nusxasi (kalit → matn).
//
// O'qish juda tez-tez, yozish esa kamdan-kam bo'ladi, shuning uchun mutex
// yo'q: yangilanish butun map'ni ALMASHTIRISH orqali bo'ladi va o'qiyotgan
// goroutine hech qachon yarim yangilangan holatni ko'rmaydi.
type Cache struct {
	m atomic.Pointer[map[string]string]
}

// NewCache — bo'sh kesh.
func NewCache() *Cache {
	c := &Cache{}
	empty := map[string]string{}
	c.m.Store(&empty)
	return c
}

// Replace butun keshni yangi map bilan almashtiradi (atomik).
func (c *Cache) Replace(next map[string]string) { c.m.Store(&next) }

// Get promptni qaytaradi. Keshda bo'lmasa — bo'sh satr.
func (c *Cache) Get(key string) string {
	if m := c.m.Load(); m != nil {
		return strings.TrimSpace((*m)[key])
	}
	return ""
}

// Keys berilgan prefiks bilan boshlanadigan kalitlar (tartiblangan).
// Masalan Keys("cat:") — barcha kategoriya promptlari.
func (c *Cache) Keys(prefix string) []string {
	m := c.m.Load()
	if m == nil {
		return nil
	}
	var out []string
	for k := range *m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Missing — berilgan kalitlardan qaysilari keshda yo'q (yoki bo'sh).
func (c *Cache) Missing(keys []string) []string {
	var out []string
	for _, k := range keys {
		if c.Get(k) == "" {
			out = append(out, k)
		}
	}
	return out
}

// Len — keshdagi promptlar soni.
func (c *Cache) Len() int {
	if m := c.m.Load(); m != nil {
		return len(*m)
	}
	return 0
}
