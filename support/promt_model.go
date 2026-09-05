// Promtlar: baza modeli va CRUD amallari. Promt matnini admin panelning
// /prompts bo'limida yozadi, kod uni faqat o'qiydi.
package support

import (
	"time"

	"gorm.io/gorm"
)

// Promt - promt modeli.
type Promt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	Promt     string    `gorm:"type:text;not null" json:"promt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListPromts barcha promtlarni yangisidan boshlab qaytaradi.
func ListPromts(db *gorm.DB) ([]Promt, error) {
	var list []Promt
	err := db.Order("id desc").Find(&list).Error
	return list, err
}

// GetPromt id bo'yicha bitta promtni qaytaradi.
func GetPromt(db *gorm.DB, id uint) (*Promt, error) {
	var p Promt
	if err := db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePromt yangi promt yaratadi.
func CreatePromt(db *gorm.DB, p *Promt) error { return db.Create(p).Error }

// UpdatePromt mavjud promtni yangilaydi (bo'sh maydonlar tegilmaydi).
func UpdatePromt(db *gorm.DB, id uint, title, promt string) (*Promt, error) {
	p, err := GetPromt(db, id)
	if err != nil {
		return nil, err
	}
	if title != "" {
		p.Title = title
	}
	if promt != "" {
		p.Promt = promt
	}
	if err := db.Save(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// DeletePromt promtni o'chiradi.
func DeletePromt(db *gorm.DB, id uint) error {
	res := db.Delete(&Promt{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
