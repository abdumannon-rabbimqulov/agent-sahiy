// Package prompts — promptlarni Postgres'dan o'qiydi va xotirada saqlaydi.
//
// Ish tamoyili: barcha promptlar bitta map'da yotadi, o'qish atomic (mutex
// yo'q — o'qish juda tez-tez bo'ladi, yozish esa kamdan-kam). Yangilanish
// butun map'ni ALMASHTIRISH orqali bo'ladi, shuning uchun o'qiyotgan
// goroutine hech qachon yarim yangilangan holatni ko'rmaydi.
//
// Ishonchlilik: baza yiqilsa yoki bo'sh natija qaytsa kesh ALMASHTIRILMAYDI —
// agent eski promptlar bilan ishlashda davom etadi.
package prompts

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// pollInterval — bazadagi o'zgarishni tekshirish oralig'i. Dashboarddan
// saqlanganda kesh darhol yangilanadi (hook orqali); bu esa boshqa nusxa
// yoki bazadan to'g'ridan-to'g'ri o'zgartirish uchun zaxira yo'l.
const pollInterval = 60 * time.Second

// Store — promptlar keshi.
type Store struct {
	m  atomic.Pointer[map[string]string]
	db *gorm.DB

	// Oxirgi ko'rilgan holat — o'zgarishni shu ikki son bilan aniqlaymiz.
	lastVersion atomic.Int64
	lastCount   atomic.Int64
}

// New yangi Store. Promptlarning YAGONA manbai — Postgres; kodda yoki
// faylda zaxira matn yo'q.
func New(db *gorm.DB) *Store {
	s := &Store{db: db}
	empty := map[string]string{}
	s.m.Store(&empty)
	return s
}

// Get promptni qaytaradi. Bazada bo'lmasa — bo'sh satr.
func (s *Store) Get(key string) string {
	if m := s.m.Load(); m != nil {
		return strings.TrimSpace((*m)[key])
	}
	return ""
}

// Missing — berilgan kalitlardan qaysilari keshda yo'q (yoki bo'sh).
// Ishga tushishda majburiy promptlar borligini tekshirish uchun.
func (s *Store) Missing(keys []string) []string {
	var out []string
	for _, k := range keys {
		if s.Get(k) == "" {
			out = append(out, k)
		}
	}
	return out
}

// Keys berilgan prefiks bilan boshlanadigan kalitlarni qaytaradi (tartiblangan).
// Masalan Keys("cat:") — barcha kategoriya promptlari.
func (s *Store) Keys(prefix string) []string {
	m := s.m.Load()
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

// Len — keshdagi promptlar soni.
func (s *Store) Len() int {
	if m := s.m.Load(); m != nil {
		return len(*m)
	}
	return 0
}

// Reload bazadan barcha yoqilgan promptlarni o'qiydi va keshni almashtiradi.
//
// Xato bo'lsa yoki natija BO'SH bo'lsa kesh saqlanib qoladi — bu ataylab:
// baza vaqtincha yiqilsa agent javob berishda davom etishi kerak.
func (s *Store) Reload() error {
	var rows []models.Prompt
	if err := s.db.Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return fmt.Errorf("promptlarni o'qish: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("bazada yoqilgan prompt yo'q — eski kesh saqlanib qoldi")
	}

	next := make(map[string]string, len(rows))
	var maxVer int64
	for _, r := range rows {
		next[r.Key] = r.Content
		if int64(r.Version) > maxVer {
			maxVer = int64(r.Version)
		}
	}
	s.m.Store(&next)
	s.lastVersion.Store(maxVer)
	s.lastCount.Store(int64(len(rows)))
	return nil
}

// Watch fon rejimida bazani kuzatadi: har pollInterval'da versiya va
// promptlar sonini tekshiradi, o'zgargan bo'lsa Reload qiladi.
//
// Butun jadvalni emas, ikkita sonni o'qiydi — arzon so'rov.
func (s *Store) Watch(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkOnce()
		}
	}
}

// checkOnce — bitta tekshirish tsikli (test qulay bo'lsin uchun alohida).
func (s *Store) checkOnce() {
	var stat struct {
		MaxVersion int64
		Cnt        int64
	}
	err := s.db.Model(&models.Prompt{}).
		Select("COALESCE(MAX(version), 0) AS max_version, COUNT(*) AS cnt").
		Where("enabled = ?", true).Scan(&stat).Error
	if err != nil {
		// Baza vaqtincha yo'q — jim o'tamiz, eski kesh ishlaydi.
		return
	}
	if stat.MaxVersion == s.lastVersion.Load() && stat.Cnt == s.lastCount.Load() {
		return // o'zgarish yo'q
	}
	if err := s.Reload(); err != nil {
		fmt.Fprintln(os.Stderr, "prompt keshini yangilash:", err)
	}
}

// Set promptni saqlaydi (yangi bo'lsa qo'shadi). Versiya oshishi va tarixga
// yozilishi models.Prompt hook'lari ichida bo'ladi, kesh esa AfterSave orqali
// darhol yangilanadi.
func (s *Store) Set(key, content string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("kalit bo'sh")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("prompt matni bo'sh bo'lmasligi kerak")
	}

	var cur []models.Prompt
	if err := s.db.Where("key = ?", key).Limit(1).Find(&cur).Error; err != nil {
		return err
	}
	if len(cur) == 0 {
		return s.db.Create(&models.Prompt{Key: key, Content: content, Version: 1, Enabled: true}).Error
	}
	p := cur[0]
	p.Content = content
	// Save → BeforeUpdate (versiya + tarix) → AfterSave (kesh).
	return s.db.Save(&p).Error
}

// SetEnabled promptni yoqadi/o'chiradi.
func (s *Store) SetEnabled(key string, enabled bool) error {
	var cur []models.Prompt
	if err := s.db.Where("key = ?", key).Limit(1).Find(&cur).Error; err != nil {
		return err
	}
	if len(cur) == 0 {
		return fmt.Errorf("prompt topilmadi: %s", key)
	}
	p := cur[0]
	p.Enabled = enabled
	return s.db.Save(&p).Error
}

// Delete promptni va uning butun tarixini o'chiradi. Majburiy promptlarni
// (models.RequiredPrompts) o'chirib bo'lmaydi — ularsiz agent ishlamaydi.
func (s *Store) Delete(key string) error {
	if slices.Contains(models.RequiredPrompts, key) {
		return fmt.Errorf("%s — majburiy prompt, o'chirib bo'lmaydi", key)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key = ?", key).Delete(&models.PromptHistory{}).Error; err != nil {
			return err
		}
		res := tx.Where("key = ?", key).Delete(&models.Prompt{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("prompt topilmadi: %s", key)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.Reload() // Delete hook'i tranzaksiya ichida ishlamaydi
}

// Rename prompt kalitini o'zgartiradi ("order" → "cat:order"): prompt ham,
// uning tarix yozuvlari ham ko'chiriladi. Yangi kalit band bo'lsa xato.
func (s *Store) Rename(oldKey, newKey string) error {
	oldKey, newKey = strings.TrimSpace(oldKey), strings.TrimSpace(newKey)
	if newKey == "" {
		return fmt.Errorf("yangi kalit bo'sh")
	}
	if oldKey == newKey {
		return nil
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var busy int64
		if err := tx.Model(&models.Prompt{}).Where("key = ?", newKey).Count(&busy).Error; err != nil {
			return err
		}
		if busy > 0 {
			return fmt.Errorf("%s kaliti allaqachon band", newKey)
		}
		// Kalit — birlamchi kalit, shuning uchun to'g'ridan-to'g'ri UPDATE.
		res := tx.Model(&models.Prompt{}).Where("key = ?", oldKey).
			UpdateColumn("key", newKey)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("prompt topilmadi: %s", oldKey)
		}
		return tx.Model(&models.PromptHistory{}).Where("key = ?", oldKey).
			UpdateColumn("key", newKey).Error
	})
	if err != nil {
		return err
	}
	return s.Reload()
}

// All — dashboard uchun barcha promptlar (o'chirilganlari ham).
func (s *Store) All() ([]models.Prompt, error) {
	var out []models.Prompt
	err := s.db.Order("key asc").Find(&out).Error
	return out, err
}

// History — bitta promptning eski versiyalari (eng yangisi birinchi).
func (s *Store) History(key string) ([]models.PromptHistory, error) {
	var out []models.PromptHistory
	err := s.db.Where("key = ?", key).Order("version desc").Limit(50).Find(&out).Error
	return out, err
}

// Rollback promptni eski versiyaga qaytaradi. Eski matn YANGI versiya bo'lib
// yoziladi (tarix o'chmaydi — orqaga qaytishni ham qaytarish mumkin).
func (s *Store) Rollback(key string, version int) error {
	var h []models.PromptHistory
	if err := s.db.Where("key = ? AND version = ?", key, version).
		Limit(1).Find(&h).Error; err != nil {
		return err
	}
	if len(h) == 0 {
		return fmt.Errorf("%s uchun %d-versiya topilmadi", key, version)
	}
	return s.Set(key, h[0].Content)
}
