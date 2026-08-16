package prompts

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// Seed FAQAT kategoriyalar bilan ishlaydi: slug'i yo'q kategoriyalarga slug
// yozadi va har bir kategoriya matnini "cat:<slug>" prompti sifatida bazaga
// ko'chiradi.
//
// base / summarize kabi promptlar SEED QILINMAYDI — ular
// butunlay Postgres'da yashaydi va dashboarddan (/prompts) yoziladi.
// Kod ichida birorta prompt matni saqlanmaydi.
func (s *Store) Seed() error {
	if err := s.seedCategorySlugs(); err != nil {
		return err
	}

	var cats []models.Category
	if err := s.db.Find(&cats).Error; err != nil {
		return fmt.Errorf("kategoriyalarni o'qish: %w", err)
	}

	var added int
	for _, c := range cats {
		if c.Slug == "" || strings.TrimSpace(c.Content) == "" {
			continue
		}
		key := models.CatKey(c.Slug)
		var cur []models.Prompt
		if err := s.db.Where("key = ?", key).Limit(1).Find(&cur).Error; err != nil {
			return err
		}
		if len(cur) > 0 {
			continue // mavjud yozuvga tegilmaydi — dashboarddagi tahrir saqlanadi
		}
		if err := s.db.Create(&models.Prompt{
			Key: key, Content: c.Content, Version: 1, Enabled: c.Active,
		}).Error; err != nil {
			return fmt.Errorf("kategoriya prompti (%s): %w", key, err)
		}
		added++
	}
	if added > 0 {
		log.Printf("✓ %d ta kategoriya prompti bazaga qo'shildi", added)
	}
	return nil
}

// seedCategorySlugs slug'i yo'q kategoriyalarga nom asosida slug yozadi.
// Bir xil slug chiqsa oxiriga id qo'shiladi.
func (s *Store) seedCategorySlugs() error {
	var cats []models.Category
	if err := s.db.Where("slug IS NULL OR slug = ''").Find(&cats).Error; err != nil {
		return fmt.Errorf("slug'siz kategoriyalar: %w", err)
	}
	for _, c := range cats {
		slug := models.Slugify(c.Name)
		if slug == "" {
			slug = fmt.Sprintf("kategoriya-%d", c.ID)
		}
		var busy int64
		if err := s.db.Model(&models.Category{}).
			Where("slug = ? AND id <> ?", slug, c.ID).Count(&busy).Error; err != nil {
			return err
		}
		if busy > 0 {
			slug = fmt.Sprintf("%s-%d", slug, c.ID)
		}
		if err := s.db.Model(&models.Category{}).Where("id = ?", c.ID).
			Update("slug", slug).Error; err != nil {
			return fmt.Errorf("slug yozish (%d): %w", c.ID, err)
		}
	}
	return nil
}

// SyncCategory kategoriya qo'shilganda/tahrirlanganda uning promptini
// yangilaydi ("cat:<slug>").
func SyncCategory(db *gorm.DB, c *models.Category) error {
	if c.Slug == "" {
		c.Slug = models.Slugify(c.Name)
	}
	if c.Slug == "" || strings.TrimSpace(c.Content) == "" {
		return nil
	}
	key := models.CatKey(c.Slug)

	var cur []models.Prompt
	if err := db.Where("key = ?", key).Limit(1).Find(&cur).Error; err != nil {
		return err
	}
	if len(cur) == 0 {
		return db.Create(&models.Prompt{
			Key: key, Content: c.Content, Version: 1, Enabled: c.Active,
		}).Error
	}
	p := cur[0]
	p.Content, p.Enabled = c.Content, c.Active
	return db.Save(&p).Error
}
