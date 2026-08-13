// Package category — agent bilim bo'limlari (kategoriyalar) CRUD'i.
package category

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
	"sahiy-agent/internal/prompts"
)

// Store — kategoriyalar do'koni.
type Store struct {
	db *gorm.DB
}

// New yangi Store.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// List kategoriyalarni qaytaradi. activeOnly=true bo'lsa faqat yoqilganlari.
func (s *Store) List(activeOnly bool) ([]models.Category, error) {
	q := s.db.Order("id asc")
	if activeOnly {
		q = q.Where("active")
	}
	var out []models.Category
	err := q.Find(&out).Error
	return out, err
}

// Get bitta kategoriyani id bo'yicha oladi.
func (s *Store) Get(id uint) (*models.Category, error) {
	var c models.Category
	if err := s.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// BySlug kategoriyani slug bo'yicha topadi (router shu kalitni qaytaradi).
func (s *Store) BySlug(slug string) (*models.Category, error) {
	var out []models.Category
	if err := s.db.Where("slug = ?", slug).Limit(1).Find(&out).Error; err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("kategoriya topilmadi: %s", slug)
	}
	return &out[0], nil
}

// Create yangi kategoriya qo'shadi va uning promptini ("cat:<slug>") yozadi.
func (s *Store) Create(c *models.Category) error {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("name va content bo'sh bo'lmasligi kerak")
	}
	if c.Slug == "" {
		c.Slug = s.freeSlug(models.Slugify(c.Name), 0)
	}
	if err := s.db.Create(c).Error; err != nil {
		return err
	}
	return prompts.SyncCategory(s.db, c)
}

// freeSlug band bo'lmagan slug qaytaradi (bir xil nomlar uchun).
func (s *Store) freeSlug(slug string, skipID uint) string {
	if slug == "" {
		slug = "kategoriya"
	}
	base := slug
	for i := 2; i < 100; i++ {
		var n int64
		s.db.Model(&models.Category{}).Where("slug = ? AND id <> ?", slug, skipID).Count(&n)
		if n == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return slug
}

// Update mavjud kategoriyani yangilaydi.
func (s *Store) Update(id uint, c *models.Category) error {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("name va content bo'sh bo'lmasligi kerak")
	}
	cur, err := s.Get(id)
	if err != nil {
		return err
	}
	slug := cur.Slug
	if slug == "" {
		slug = s.freeSlug(models.Slugify(c.Name), id)
	}
	if err := s.db.Model(&models.Category{}).Where("id = ?", id).
		Updates(map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"content":     c.Content,
			"active":      c.Active,
			"slug":        slug,
		}).Error; err != nil {
		return err
	}
	// Kategoriya matni — bu ayni paytda "cat:<slug>" prompti.
	c.Slug = slug
	return prompts.SyncCategory(s.db, c)
}

// Delete kategoriyani o'chiradi.
func (s *Store) Delete(id uint) error {
	return s.db.Delete(&models.Category{}, id).Error
}

// Catalog — AI ga beriladigan qisqa ro'yxat (faqat aktiv kategoriyalar):
//
//	1 — Yetkazib berish: punktlar, muddat, narx
//	2 — To'lov: karta, naqd
//
// Aktiv kategoriya bo'lmasa bo'sh satr qaytadi.
func (s *Store) Catalog() (string, error) {
	cats, err := s.List(true)
	if err != nil || len(cats) == 0 {
		return "", err
	}
	var b strings.Builder
	for _, c := range cats {
		fmt.Fprintf(&b, "%d — %s", c.ID, c.Name)
		if c.Description != "" {
			fmt.Fprintf(&b, ": %s", c.Description)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
