// Package models — barcha GORM modellari (SQLAlchemy'dagi models.py kabi).
package models

import "time"

// Category — agent javob berishda ishlatadigan bilim bo'limi.
// Masalan id=1 "Yetkazib berish": narx, muddat, punktlar haqida ma'lumot.
type Category struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:200;not null" json:"name"`
	Description string    `gorm:"size:500" json:"description"` // AI qachon tanlashini bilishi uchun qisqa izoh
	Content     string    `gorm:"type:text;not null" json:"content"`
	Active      bool      `gorm:"not null;default:true;index" json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Interaction — AI (yoki xodim) bir marta kim bilan qanday gaplashgani.
type Interaction struct {
	ID             uint      `gorm:"primaryKey" json:"-"`
	CreatedAt      time.Time `gorm:"column:created;index" json:"time"`
	ConversationID int64     `gorm:"index" json:"conversation_id"`
	ClientID       int64     `gorm:"index" json:"client_id"`
	ClientName     string    `gorm:"size:200" json:"client_name"`
	Title          string    `gorm:"size:500" json:"title"`
	ClientMessage  string    `gorm:"type:text" json:"client_message"`
	AIReply        string    `gorm:"type:text" json:"ai_reply"`
	Sent           bool      `gorm:"not null;default:false" json:"sent"`
	CategoryID     *uint     `gorm:"index" json:"category_id,omitempty"`
	Category       *Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category,omitempty"`
}

// Escalation — xodimlar guruhiga yuborilgan bitta murojaat.
type Escalation struct {
	TgMessageID    int64     `gorm:"primaryKey;autoIncrement:false" json:"tg_message_id"`
	ConversationID int64     `gorm:"index" json:"conversation_id"`
	ClientName     string    `gorm:"size:200" json:"client_name"`
	Question       string    `gorm:"type:text" json:"question"`
	Answer         string    `gorm:"type:text" json:"answer"`
	Resolved       bool      `gorm:"not null;default:false" json:"resolved"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationState — har bir suhbat bo'yicha oxirgi ko'rilgan mijoz xabari.
// Shu tufayli eski suhbatga kelgan yangi xabar ham seziladi.
type ConversationState struct {
	ConversationID int64     `gorm:"primaryKey;autoIncrement:false" json:"conversation_id"`
	LastMessageID  int64     `json:"last_message_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Setting — oddiy kalit-qiymat sozlamalari (eski kv jadvali o'rniga).
type Setting struct {
	Key   string `gorm:"primaryKey;size:100" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

// TableName — eski "kv" jadvali saqlanib qoladi (ma'lumot ko'chirilmasin).
func (Setting) TableName() string { return "kv" }
