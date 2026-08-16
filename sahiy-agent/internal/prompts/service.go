package prompts

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"sahiy-agent/internal/models"
)

// pollInterval — bazadagi o'zgarishni tekshirish oralig'i. Dashboarddan
// saqlanganda kesh darhol yangilanadi; bu esa boshqa nusxa yoki bazadan
// to'g'ridan-to'g'ri o'zgartirish uchun zaxira yo'l.
const pollInterval = 60 * time.Second

// maxContentBytes — bitta prompt matnining chegarasi.
const maxContentBytes = 1 << 20 // 1 MB

// Service — promptlarning biznes qatlami: tekshiruvlar, himoyalangan
// kalitlar va keshni yangilab turish. Agent (ai.Prompts) shu turdan o'qiydi.
type Service struct {
	repo  Repository
	cache *Cache
	// protected — o'chirib yoki o'chirib qo'yib bo'lmaydigan kalitlar
	// (ai.RequiredKeys). Ularsiz agent umuman ishlamaydi.
	protected []string

	mu   sync.Mutex  // last'ni himoyalaydi (faqat Watch va Reload tegadi)
	last Fingerprint // oxirgi ko'rilgan holat
}

// NewService yangi service. protected — majburiy promptlar ro'yxati.
func NewService(repo Repository, protected []string) *Service {
	return &Service{repo: repo, cache: NewCache(), protected: protected}
}

// --- o'qish (agent uchun; ai.Prompts interfeysi) ---

// Get — kalit bo'yicha prompt matni (topilmasa bo'sh satr).
func (s *Service) Get(key string) string { return s.cache.Get(key) }

// Keys — prefiks bilan boshlanadigan kalitlar (masalan "cat:").
func (s *Service) Keys(prefix string) []string { return s.cache.Keys(prefix) }

// Len — keshdagi promptlar soni.
func (s *Service) Len() int { return s.cache.Len() }

// Missing — berilgan kalitlardan qaysilari keshda yo'q.
func (s *Service) Missing(keys []string) []string { return s.cache.Missing(keys) }

// --- kesh ---

// Reload bazadan barcha yoqilgan promptlarni o'qiydi va keshni almashtiradi.
//
// Xato bo'lsa yoki natija BO'SH bo'lsa kesh saqlanib qoladi — bu ataylab:
// baza vaqtincha yiqilsa agent javob berishda davom etishi kerak.
func (s *Service) Reload() error {
	rows, err := s.repo.Enabled()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("bazada yoqilgan prompt yo'q — eski kesh saqlanib qoldi")
	}

	next := make(map[string]string, len(rows))
	for _, r := range rows {
		next[r.Key] = r.Content
	}
	s.cache.Replace(next)

	if fp, err := s.repo.Fingerprint(); err == nil {
		s.mu.Lock()
		s.last = fp
		s.mu.Unlock()
	}
	return nil
}

// Watch fon rejimida bazani kuzatadi: har pollInterval'da promptlar soni va
// oxirgi tahrir vaqtini tekshiradi, o'zgargan bo'lsa Reload qiladi.
func (s *Service) Watch(ctx context.Context) {
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
func (s *Service) checkOnce() {
	fp, err := s.repo.Fingerprint()
	if err != nil {
		return // baza vaqtincha yo'q — jim o'tamiz, eski kesh ishlaydi
	}
	s.mu.Lock()
	same := fp == s.last
	s.mu.Unlock()
	if same {
		return
	}
	if err := s.Reload(); err != nil {
		fmt.Fprintln(os.Stderr, "prompt keshini yangilash:", err)
	}
}

// --- CRUD ---

// List — dashboard uchun barcha promptlar (o'chirib qo'yilganlari ham).
func (s *Service) List() ([]models.Prompt, error) { return s.repo.List() }

// ByKey — bitta prompt (topilmasa ErrNotFound).
func (s *Service) ByKey(key string) (models.Prompt, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return models.Prompt{}, fmt.Errorf("%w: kalit bo'sh", ErrInvalid)
	}
	return s.repo.ByKey(key)
}

// Create yangi prompt qo'shadi. Kalit band bo'lsa ErrConflict.
func (s *Service) Create(key, content string, enabled bool) (models.Prompt, error) {
	key, content = strings.TrimSpace(key), strings.TrimSpace(content)
	if err := validate(key, content); err != nil {
		return models.Prompt{}, err
	}
	busy, err := s.repo.Exists(key)
	if err != nil {
		return models.Prompt{}, err
	}
	if busy {
		return models.Prompt{}, fmt.Errorf("%w: %s kaliti allaqachon band", ErrConflict, key)
	}

	p := models.Prompt{Key: key, Content: content, Enabled: enabled}
	if err := s.repo.Create(&p); err != nil {
		return models.Prompt{}, err
	}
	return p, s.refresh()
}

// Update — promptni o'zgartirish. Faqat berilgan (nil bo'lmagan) maydonlar
// yangilanadi, shuning uchun to'liq (PUT) va qisman (PATCH) tahrir uchun
// bitta yo'l yetarli. NewKey berilsa — kalit o'zgaradi (rename).
type Update struct {
	NewKey  *string
	Content *string
	Enabled *bool
}

// Update promptni yangilaydi va yangilangan yozuvni qaytaradi.
func (s *Service) Update(key string, upd Update) (models.Prompt, error) {
	p, err := s.ByKey(key)
	if err != nil {
		return models.Prompt{}, err
	}

	if upd.NewKey != nil {
		newKey := strings.TrimSpace(*upd.NewKey)
		if newKey != p.Key {
			if err := s.checkRename(p.Key, newKey); err != nil {
				return models.Prompt{}, err
			}
			p.Key = newKey
		}
	}
	if upd.Content != nil {
		content := strings.TrimSpace(*upd.Content)
		if err := validate(p.Key, content); err != nil {
			return models.Prompt{}, err
		}
		p.Content = content
	}
	if upd.Enabled != nil {
		// Majburiy promptni o'chirib qo'yish — agentni ishdan to'xtatish
		// bilan barobar, shuning uchun taqiqlanadi.
		if !*upd.Enabled && s.isProtected(p.Key) {
			return models.Prompt{}, fmt.Errorf(
				"%w: %s — majburiy prompt, o'chirib qo'yib bo'lmaydi", ErrConflict, p.Key)
		}
		p.Enabled = *upd.Enabled
	}

	if err := s.repo.Update(&p); err != nil {
		return models.Prompt{}, err
	}
	return p, s.refresh()
}

// Delete promptni o'chiradi. Majburiy promptlarni o'chirib bo'lmaydi.
func (s *Service) Delete(key string) error {
	key = strings.TrimSpace(key)
	if s.isProtected(key) {
		return fmt.Errorf("%w: %s — majburiy prompt, o'chirib bo'lmaydi", ErrConflict, key)
	}
	if err := s.repo.DeleteByKey(key); err != nil {
		return err
	}
	return s.refresh()
}

// --- ichki yordamchilar ---

// checkRename — yangi kalit yaroqli va bo'sh ekanini tekshiradi.
func (s *Service) checkRename(oldKey, newKey string) error {
	if newKey == "" {
		return fmt.Errorf("%w: yangi kalit bo'sh", ErrInvalid)
	}
	if s.isProtected(oldKey) {
		return fmt.Errorf("%w: %s — majburiy prompt, nomini o'zgartirib bo'lmaydi",
			ErrConflict, oldKey)
	}
	busy, err := s.repo.Exists(newKey)
	if err != nil {
		return err
	}
	if busy {
		return fmt.Errorf("%w: %s kaliti allaqachon band", ErrConflict, newKey)
	}
	return nil
}

// isProtected — kalit majburiy promptlar ro'yxatidami.
func (s *Service) isProtected(key string) bool { return slices.Contains(s.protected, key) }

// refresh — yozuvdan keyin keshni yangilaydi. Kesh yangilanmasa ham yozuv
// bazada saqlangan, shuning uchun xato faqat ogohlantirish sifatida qaytadi.
func (s *Service) refresh() error {
	if err := s.Reload(); err != nil {
		fmt.Fprintln(os.Stderr, "prompt keshini yangilash:", err)
	}
	return nil
}

// validate — kalit va matn uchun umumiy tekshiruv.
func validate(key, content string) error {
	switch {
	case key == "":
		return fmt.Errorf("%w: kalit bo'sh", ErrInvalid)
	case len(key) > 100:
		return fmt.Errorf("%w: kalit 100 belgidan uzun", ErrInvalid)
	case content == "":
		return fmt.Errorf("%w: prompt matni bo'sh bo'lmasligi kerak", ErrInvalid)
	case len(content) > maxContentBytes:
		return fmt.Errorf("%w: prompt matni juda uzun (1 MB dan katta)", ErrInvalid)
	}
	return nil
}
