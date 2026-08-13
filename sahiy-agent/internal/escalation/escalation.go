// Package escalation — xodimlar guruhiga yuborilgan murojaatlar (GORM).
package escalation

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sahiy-agent/internal/models"
)

// Store — murojaatlar do'koni.
type Store struct {
	db *gorm.DB
}

// New yangi Store.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Add yangi murojaatni qo'shadi (mavjud bo'lsa tegmaydi).
func (s *Store) Add(it *models.Escalation) error {
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(it).Error
}

// Get reply qilingan tg message_id bo'yicha murojaatni topadi.
func (s *Store) Get(tgMessageID int64) (*models.Escalation, bool) {
	var it models.Escalation
	if err := s.db.First(&it, "tg_message_id = ?", tgMessageID).Error; err != nil {
		return nil, false
	}
	return &it, true
}

// Answer xodim javobini saqlaydi va muammoni hal qilingan deb belgilaydi.
func (s *Store) Answer(tgMessageID int64, answer, by string) error {
	now := time.Now()
	return s.db.Model(&models.Escalation{}).
		Where("tg_message_id = ?", tgMessageID).
		Updates(map[string]any{
			"status":      models.StatusStaffSent,
			"answer":      answer,
			"answered_by": by,
			"answered_at": now,
		}).Error
}

// Pending — hali javob kutayotgan muammolar (eng yangisi birinchi).
func (s *Store) Pending(limit int) ([]models.Escalation, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []models.Escalation
	err := s.db.Where("status = ?", models.StatusPending).
		Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// Recent — oxirgi murojaatlar (holatidan qat'i nazar).
func (s *Store) Recent(limit int) ([]models.Escalation, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []models.Escalation
	err := s.db.Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// CountPending — jarayondagi muammolar soni.
func (s *Store) CountPending() (int64, error) {
	var n int64
	err := s.db.Model(&models.Escalation{}).
		Where("status = ?", models.StatusPending).Count(&n).Error
	return n, err
}
