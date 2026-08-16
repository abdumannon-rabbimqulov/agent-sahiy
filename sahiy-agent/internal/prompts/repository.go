package prompts

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// Fingerprint — jadvalning "hozirgi holati" (o'zgarganini arzon aniqlash
// uchun): yoqilgan promptlar soni va eng oxirgi tahrir vaqti.
type Fingerprint struct {
	Count     int64
	UpdatedAt time.Time
}

// Repository — promptlarni saqlash qatlami. Faqat SQL: bu yerda hech qanday
// tekshiruv yoki biznes qoidasi yo'q (ular Service'da).
type Repository interface {
	// List — barcha promptlar (o'chirib qo'yilganlari ham), kalit bo'yicha.
	List() ([]models.Prompt, error)
	// Enabled — faqat yoqilgan promptlar (kesh shulardan yig'iladi).
	Enabled() ([]models.Prompt, error)
	// ByKey — bitta prompt; topilmasa ErrNotFound.
	ByKey(key string) (models.Prompt, error)
	// Exists — shunday kalit bormi.
	Exists(key string) (bool, error)
	// Create — yangi yozuv (kalit band bo'lsa xato).
	Create(p *models.Prompt) error
	// Update — mavjud yozuvni yangilaydi (ID bo'yicha).
	Update(p *models.Prompt) error
	// DeleteByKey — o'chirish; topilmasa ErrNotFound.
	DeleteByKey(key string) error
	// Fingerprint — yoqilgan promptlar soni va oxirgi tahrir vaqti.
	Fingerprint() (Fingerprint, error)
}

// gormRepo — Postgres (GORM) ustidagi Repository.
type gormRepo struct {
	db *gorm.DB
}

// NewRepository — GORM asosidagi repozitoriy.
func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) List() ([]models.Prompt, error) {
	var out []models.Prompt
	if err := r.db.Order("key asc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("promptlar ro'yxati: %w", err)
	}
	return out, nil
}

func (r *gormRepo) Enabled() ([]models.Prompt, error) {
	var out []models.Prompt
	if err := r.db.Where("enabled = ?", true).Order("key asc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("yoqilgan promptlar: %w", err)
	}
	return out, nil
}

// ByKey — "topilmadi" oddiy holat, shuning uchun First emas Find: GORM
// logga ortiqcha ogohlantirish yozmaydi.
func (r *gormRepo) ByKey(key string) (models.Prompt, error) {
	var rows []models.Prompt
	if err := r.db.Where("key = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return models.Prompt{}, fmt.Errorf("promptni o'qish: %w", err)
	}
	if len(rows) == 0 {
		return models.Prompt{}, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return rows[0], nil
}

func (r *gormRepo) Exists(key string) (bool, error) {
	var n int64
	if err := r.db.Model(&models.Prompt{}).Where("key = ?", key).Count(&n).Error; err != nil {
		return false, fmt.Errorf("kalitni tekshirish: %w", err)
	}
	return n > 0, nil
}

func (r *gormRepo) Create(p *models.Prompt) error {
	if err := r.db.Create(p).Error; err != nil {
		return fmt.Errorf("promptni yaratish: %w", err)
	}
	return nil
}

func (r *gormRepo) Update(p *models.Prompt) error {
	if err := r.db.Save(p).Error; err != nil {
		return fmt.Errorf("promptni saqlash: %w", err)
	}
	return nil
}

func (r *gormRepo) DeleteByKey(key string) error {
	res := r.db.Where("key = ?", key).Delete(&models.Prompt{})
	if res.Error != nil {
		return fmt.Errorf("promptni o'chirish: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return nil
}

// Fingerprint — butun jadvalni emas, ikkita qiymatni o'qiydi (arzon so'rov).
func (r *gormRepo) Fingerprint() (Fingerprint, error) {
	var row struct {
		Cnt       int64
		UpdatedAt *time.Time
	}
	err := r.db.Model(&models.Prompt{}).
		Select("COUNT(*) AS cnt, MAX(updated_at) AS updated_at").
		Where("enabled = ?", true).Scan(&row).Error
	if err != nil {
		return Fingerprint{}, fmt.Errorf("promptlar holati: %w", err)
	}
	fp := Fingerprint{Count: row.Cnt}
	if row.UpdatedAt != nil {
		fp.UpdatedAt = *row.UpdatedAt
	}
	return fp, nil
}
