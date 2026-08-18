package models

import "time"

// Prompt — bazada saqlanadigan prompt matni. Dashboarddan tahrirlanadi va
// o'zgarish darhol kuchga kiradi (agent qayta ishga tushirilmaydi).
//
// Kalitlar ro'yxati kodda emas, ai paketida (ai.PromptBase, ai.BlockOrder...)
// — bu yerda faqat saqlash shakli.
type Prompt struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:100;uniqueIndex;not null" json:"key"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Enabled   bool      `gorm:"not null;default:true;index" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// PromptBackup — prompt matnining tahrirdan OLDINGI nusxasi. To'liq
// versiyalash emas: har kalit uchun oxirgi bir nechta nusxa saqlanadi va
// dashboarddan tiklanadi. Sabab — "Saqlash" tugmasi aks holda qaytarib
// bo'lmaydigan amal bo'lib qoladi.
type PromptBackup struct {
	ID      uint      `gorm:"primaryKey" json:"id"`
	Key     string    `gorm:"size:100;index;not null" json:"key"`
	Content string    `gorm:"type:text;not null" json:"content"`
	SavedAt time.Time `gorm:"autoCreateTime;index" json:"saved_at"`
}
