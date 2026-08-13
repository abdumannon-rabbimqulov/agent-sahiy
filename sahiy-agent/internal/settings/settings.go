// Package settings — ish vaqtida o'zgartiriladigan sozlamalar (dashboarddan).
//
// .env dagi qiymat faqat BIRINCHI ishga tushishda boshlang'ich qiymat bo'lib
// oladi; shundan keyin dashboarddagi tugma ustun turadi va agentni qayta
// ishga tushirish shart emas.
package settings

import (
	"strconv"
	"sync"

	"gorm.io/gorm"

	"sahiy-agent/internal/db"
)

// Kalitlar (kv jadvalida shu nom bilan saqlanadi).
const (
	// AIEnabled — false bo'lsa agent AI'ga umuman murojaat qilmaydi:
	// token sarflanmaydi, xabarlar javobsiz kutib turadi.
	AIEnabled = "ai_enabled"
	// AutoReply — true bo'lsa AI javobi mijozga darhol yuboriladi;
	// false bo'lsa javob dashboardda tekshirish uchun turadi.
	AutoReply = "auto_reply"
)

// Store — sozlamalar do'koni. Qiymatlar xotirada keshlanadi (har tsiklda
// bazaga bormaslik uchun), yozilganda kesh yangilanadi.
type Store struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]bool
}

// New yangi Store.
func New(gdb *gorm.DB) *Store {
	return &Store{db: gdb, cache: map[string]bool{}}
}

// Init kalit bazada bo'lmasa .env dagi boshlang'ich qiymat bilan yozadi.
// Bor bo'lsa tegmaydi — dashboarddagi tanlov saqlanib qoladi.
func (s *Store) Init(key string, def bool) error {
	v, err := db.GetSetting(s.db, key)
	if err != nil {
		return err
	}
	if v == "" {
		if err := s.Set(key, def); err != nil {
			return err
		}
		return nil
	}
	s.mu.Lock()
	s.cache[key] = v == "true"
	s.mu.Unlock()
	return nil
}

// Bool qiymatni qaytaradi (bazada bo'lmasa yoki xato bo'lsa — def).
func (s *Store) Bool(key string, def bool) bool {
	s.mu.RLock()
	v, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		return v
	}
	raw, err := db.GetSetting(s.db, key)
	if err != nil || raw == "" {
		return def
	}
	val := raw == "true"
	s.mu.Lock()
	s.cache[key] = val
	s.mu.Unlock()
	return val
}

// Set qiymatni bazaga yozadi va keshni yangilaydi.
func (s *Store) Set(key string, v bool) error {
	if err := db.SetSetting(s.db, key, strconv.FormatBool(v)); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = v
	s.mu.Unlock()
	return nil
}

// All — dashboard uchun barcha sozlamalar.
func (s *Store) All() map[string]bool {
	return map[string]bool{
		AIEnabled: s.Bool(AIEnabled, true),
		AutoReply: s.Bool(AutoReply, false),
	}
}
