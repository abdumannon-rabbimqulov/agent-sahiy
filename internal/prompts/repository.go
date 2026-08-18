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

	// SaveBackup — matnning tahrirdan oldingi nusxasini saqlaydi va shu
	// kalitning eskilarini (keepBackups dan ortig'ini) o'chiradi.
	SaveBackup(key, content string) error
	// Backups — kalitning saqlangan nusxalari, yangisidan eskisiga.
	Backups(key string) ([]models.PromptBackup, error)
	// BackupByID — bitta nusxa (tiklash uchun); topilmasa ErrNotFound.
	BackupByID(id uint) (models.PromptBackup, error)
}

// keepBackups — har kalit uchun saqlanadigan nusxalar soni. Maqsad —
// noto'g'ri tahrirni qaytarish, arxiv yuritish emas.
const keepBackups = 5

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

// DeleteByKey promptni va uning nusxalarini o'chiradi. Nusxalar qolsa
// shu nom bilan yangi prompt yaratilganda begona tarix ko'rinib qolardi.
func (r *gormRepo) DeleteByKey(key string) error {
	res := r.db.Where("key = ?", key).Delete(&models.Prompt{})
	if res.Error != nil {
		return fmt.Errorf("promptni o'chirish: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if err := r.db.Where("key = ?", key).Delete(&models.PromptBackup{}).Error; err != nil {
		return fmt.Errorf("prompt nusxalarini o'chirish: %w", err)
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

// SaveBackup nusxa qo'shadi va shu kalitning eng eski (keepBackups dan
// ortiq) yozuvlarini o'chiradi.
func (r *gormRepo) SaveBackup(key, content string) error {
	if content == "" {
		return nil // bo'sh matnni saqlashdan foyda yo'q
	}
	if err := r.db.Create(&models.PromptBackup{Key: key, Content: content}).Error; err != nil {
		return fmt.Errorf("prompt nusxasini saqlash: %w", err)
	}
	// Ortiqchasini o'chirish: eng yangi keepBackups tadan tashqarisi ketadi.
	sub := r.db.Model(&models.PromptBackup{}).Select("id").
		Where("key = ?", key).Order("id desc").Limit(keepBackups)
	if err := r.db.Where("key = ? AND id NOT IN (?)", key, sub).
		Delete(&models.PromptBackup{}).Error; err != nil {
		return fmt.Errorf("eski nusxalarni o'chirish: %w", err)
	}
	return nil
}

func (r *gormRepo) Backups(key string) ([]models.PromptBackup, error) {
	var out []models.PromptBackup
	if err := r.db.Where("key = ?", key).Order("id desc").
		Limit(keepBackups).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("prompt nusxalari: %w", err)
	}
	return out, nil
}

func (r *gormRepo) BackupByID(id uint) (models.PromptBackup, error) {
	var rows []models.PromptBackup
	if err := r.db.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return models.PromptBackup{}, fmt.Errorf("nusxani o'qish: %w", err)
	}
	if len(rows) == 0 {
		return models.PromptBackup{}, fmt.Errorf("%w: nusxa #%d", ErrNotFound, id)
	}
	return rows[0], nil
}
