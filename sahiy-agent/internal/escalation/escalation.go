// Package escalation — xodimlar guruhiga yuborilgan murojaatlar (GORM).
package escalation

import (
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

// Resolve murojaatni hal qilingan deb belgilaydi.
func (s *Store) Resolve(tgMessageID int64, answer string) error {
	return s.db.Model(&models.Escalation{}).
		Where("tg_message_id = ?", tgMessageID).
		Updates(map[string]any{"resolved": true, "answer": answer}).Error
}
