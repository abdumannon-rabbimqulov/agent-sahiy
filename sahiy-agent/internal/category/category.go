// Package category — agent bilim bo'limlari (kategoriyalar) CRUD'i.
package category

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
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

// Create yangi kategoriya qo'shadi.
func (s *Store) Create(c *models.Category) error {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("name va content bo'sh bo'lmasligi kerak")
	}
	return s.db.Create(c).Error
}

// Update mavjud kategoriyani yangilaydi.
func (s *Store) Update(id uint, c *models.Category) error {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("name va content bo'sh bo'lmasligi kerak")
	}
	return s.db.Model(&models.Category{}).Where("id = ?", id).
		Updates(map[string]any{
			"name":        c.Name,
			"description": c.Description,
			"content":     c.Content,
			"active":      c.Active,
		}).Error
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
