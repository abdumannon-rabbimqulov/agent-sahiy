// Package models — barcha GORM modellari (SQLAlchemy'dagi models.py kabi).
package models

import "time"

// Category — agent javob berishda ishlatadigan bilim bo'limi.
// Masalan id=1 "Yetkazib berish": narx, muddat, punktlar haqida ma'lumot.
type Category struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:200;not null" json:"name"`
	// Slug — prompt kaliti uchun ("Yetkazib berish" → "yetkazib-berish").
	// Kategoriya bilimlari prompts jadvalida "cat:<slug>" kaliti bilan yotadi.
	Slug        string    `gorm:"size:120;uniqueIndex" json:"slug"`
	Description string    `gorm:"size:500" json:"description"` // AI qachon tanlashini bilishi uchun qisqa izoh
	Content     string    `gorm:"type:text;not null" json:"content"`
	Active      bool      `gorm:"not null;default:true;index" json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Muammo holati (status) — Interaction va Escalation uchun bir xil.
// Dashboard ham, Telegram xabari ham shu qiymatlarga tayanadi.
const (
	// StatusAISent — AI o'zi xulosa chiqarib javob berdi va mijozga yubordi.
	StatusAISent = "ai_sent"
	// StatusAIDraft — AI javob yozdi, lekin yuborilmadi (AUTO_REPLY=false).
	StatusAIDraft = "ai_draft"
	// StatusPending — AI hal qila olmadi, xodimlar guruhiga yuborildi,
	// javob kutilmoqda (muammo hal jarayonida).
	StatusPending = "pending"
	// StatusStaffSent — xodim guruhda javob berdi, javob mijozga yuborildi.
	StatusStaffSent = "staff_sent"
	// StatusFailed — yuborib bo'lmadi (kanal yo'q yoki xato).
	StatusFailed = "failed"
	// StatusRejected — admin AI javobini ko'rib chiqib rad etdi (yuborilmadi).
	StatusRejected = "rejected"
	// StatusUnclear — AI murojaatni hech qaysi kategoriyaga qo'sha olmadi
	// (base prompti {"category":false} qaytardi). Mijozga hech narsa
	// yuborilmagan — admin dashboarddan suhbatni o'zi ko'rib chiqadi.
	StatusUnclear = "unclear"
)

// StatusLabel — statusning o'zbekcha nomi (dashboard va loglar uchun).
func StatusLabel(status string) string {
	switch status {
	case StatusAISent:
		return "AI hal qildi"
	case StatusAIDraft:
		return "AI javobi (yuborilmadi)"
	case StatusPending:
		return "Jarayonda — xodim javobi kutilmoqda"
	case StatusStaffSent:
		return "Xodim hal qildi"
	case StatusFailed:
		return "Xatolik"
	case StatusRejected:
		return "Admin rad etdi"
	case StatusUnclear:
		return "AI tushunmadi — ko'rib chiqing"
	}
	return status
}

// Interaction — AI (yoki xodim) bir marta kim bilan qanday gaplashgani.
type Interaction struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time `gorm:"column:created;index" json:"time"`
	ConversationID int64     `gorm:"index" json:"conversation_id"`
	ClientID       int64     `gorm:"index" json:"client_id"`
	ClientName     string    `gorm:"size:200" json:"client_name"`
	Title          string    `gorm:"size:500" json:"title"`
	ClientMessage  string    `gorm:"type:text" json:"client_message"`
	AIReply        string    `gorm:"type:text" json:"ai_reply"`
	Sent           bool      `gorm:"not null;default:false" json:"sent"`
	// Steps — agent shu javobga qanday ketma-ketlikda kelgani
	// (nechta xabar o'qidi, qaysi kategoriya, qaysi buyurtmani topdi...).
	Steps string `gorm:"type:text" json:"steps"`
	// Rasm tahlil qilinmaydi (token tejash) — faqat borligi belgilanadi va
	// dashboardda "rasm bor" deb ko'rsatiladi.
	ImageCount int    `gorm:"not null;default:0" json:"image_count"`
	ImageURLs  string `gorm:"type:text" json:"image_urls"` // qator bilan ajratilgan asl havolalar

	// --- AI xarajati: provayder qaytargan aniq token soni va uning narxi ---
	Model            string  `gorm:"size:60" json:"model"`
	PromptTokens     int     `gorm:"not null;default:0" json:"prompt_tokens"`
	CachedTokens     int     `gorm:"not null;default:0" json:"cached_tokens"`
	CompletionTokens int     `gorm:"not null;default:0" json:"completion_tokens"`
	AICalls          int     `gorm:"not null;default:0" json:"ai_calls"`
	CostUSD          float64 `gorm:"type:numeric(12,6);not null;default:0" json:"cost_usd"`

	// Status — muammo holati (pending → staff_sent, yoki darhol ai_sent).
	Status string `gorm:"size:20;index;not null;default:'ai_sent'" json:"status"`
	// HandledBy — kim hal qilgani: "AI" yoki "Xodim: Ali".
	HandledBy string `gorm:"size:120" json:"handled_by"`
	// EscalationID — eskalatsiya bo'lsa, Telegram xabari id'si. Xodim javob
	// berganda aynan shu yozuv topilib, statusi yangilanadi.
	EscalationID *int64     `gorm:"index" json:"escalation_id,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`

	CategoryID *uint     `gorm:"index" json:"category_id,omitempty"`
	Category   *Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category,omitempty"`
}

// Escalation — xodimlar guruhiga yuborilgan bitta muammo va uning holati.
type Escalation struct {
	TgMessageID    int64  `gorm:"primaryKey;autoIncrement:false" json:"tg_message_id"`
	ConversationID int64  `gorm:"index" json:"conversation_id"`
	ClientID       int64  `gorm:"index" json:"client_id"`
	ClientName     string `gorm:"size:200" json:"client_name"`
	Question       string `gorm:"type:text" json:"question"`
	// Summary — AI chiqargan xulosa (xodim guruhda shuni o'qiydi).
	Summary string `gorm:"type:text" json:"summary"`
	// Level — shoshilinchlik darajasi: yuqori | o'rta | past.
	Level string `gorm:"size:20" json:"level"`

	Status     string     `gorm:"size:20;index;not null;default:'pending'" json:"status"`
	Answer     string     `gorm:"type:text" json:"answer"`
	AnsweredBy string     `gorm:"size:120" json:"answered_by"`
	AnsweredAt *time.Time `json:"answered_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Resolved — muammo hal qilinganmi.
func (e Escalation) Resolved() bool { return e.Status == StatusStaffSent }

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
