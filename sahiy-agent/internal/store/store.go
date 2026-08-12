// Package store — suhbatlar tarixi (GORM).
package store

import (
	"fmt"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// Store — tarix do'koni.
type Store struct {
	db *gorm.DB
}

// New yangi Store.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Append bitta yozuvni qo'shadi.
func (s *Store) Append(in *models.Interaction) error {
	return s.db.Omit("Category").Create(in).Error
}

// Recent oxirgi n yozuvni (eng yangisi birinchi) qaytaradi.
func (s *Store) Recent(n int) ([]models.Interaction, error) {
	if n <= 0 {
		n = 100
	}
	var out []models.Interaction
	err := s.db.Preload("Category").Order("id desc").Limit(n).Find(&out).Error
	return out, err
}

// Stats — umumiy statistika.
type Stats struct {
	TotalReplies  int `json:"TotalReplies"`
	SentReplies   int `json:"SentReplies"`
	UniqueClients int `json:"UniqueClients"`
	UniqueChats   int `json:"UniqueChats"`
}

// Stats statistikani hisoblaydi.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	err := s.db.Model(&models.Interaction{}).
		Select(`COUNT(*) AS total_replies,
		        COUNT(*) FILTER (WHERE sent) AS sent_replies,
		        COUNT(DISTINCT client_id) FILTER (WHERE client_id <> 0) AS unique_clients,
		        COUNT(DISTINCT conversation_id) AS unique_chats`).
		Scan(&st).Error
	return st, err
}

// String qisqacha statistika.
func (st Stats) String() string {
	return fmt.Sprintf("odamlar: %d | suhbatlar: %d | javoblar: %d (yuborilgan: %d)",
		st.UniqueClients, st.UniqueChats, st.TotalReplies, st.SentReplies)
}

// --- chat rasmlari ---

// SaveImage rasm yozuvini qo'shadi yoki yangilaydi.
func (s *Store) SaveImage(img *models.ChatImage) error {
	return s.db.Save(img).Error
}

// GetImage xabar id bo'yicha saqlangan rasmni qaytaradi.
// Topilmasa (nil, false) — bu xato emas.
func (s *Store) GetImage(messageID int64) (*models.ChatImage, bool) {
	var img models.ChatImage
	if err := s.db.First(&img, "message_id = ?", messageID).Error; err != nil {
		return nil, false
	}
	return &img, true
}

// RecentImages oxirgi n ta rasmni qaytaradi (eng yangisi birinchi).
func (s *Store) RecentImages(n int) ([]models.ChatImage, error) {
	if n <= 0 {
		n = 200
	}
	var out []models.ChatImage
	err := s.db.Order("created_at desc").Limit(n).Find(&out).Error
	return out, err
}
