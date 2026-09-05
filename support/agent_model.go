// Bazadagi agent modellari: mijoz murojaati va unga tayyorlangan javob
// (Interaction), zanjirning har bosqichi (AgentStep), suhbatning
// ishlanganlik holati va global sozlama yozuvi.
package support

import (
	"time"

	"gorm.io/gorm"
)

// Interaction manbalari.
const (
	SourceAgent    = "agent"    // AI zanjiri o'zi tayyorlagan
	SourceTelegram = "telegram" // xodimning guruhdagi javobidan
)

// Interaction statuslari.
const (
	StatusPending  = "pending"  // admin tasdig'ini kutmoqda
	StatusSent     = "sent"     // avtomatik yuborildi
	StatusApproved = "approved" // admin tasdiqlab yubordi
	StatusRejected = "rejected" // admin rad etdi
	StatusFailed   = "failed"   // zanjir yoki yuborish xatosi
)

// Interaction - bitta mijoz murojaati va unga AI tayyorlagan javob.
type Interaction struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	ConversationID int64  `gorm:"index;not null" json:"conversation_id"`
	ClientID       int64  `gorm:"index" json:"client_id"`
	ClientMessage  string `gorm:"type:text" json:"client_message"`

	// AI natijasi: mijozga (chat) va ichki guruhga (help) ketadigan matnlar.
	ChatReply string `gorm:"type:text" json:"chat_reply"`
	HelpText  string `gorm:"type:text" json:"help_text"`

	// MessageIDs - shu murojaatda javob berilayotgan mijoz xabarlari
	// ("1,2,3"). Javob mijozga yetib borgandan keyin shular o'qilgan
	// deb belgilanadi.
	MessageIDs string `gorm:"size:255" json:"message_ids,omitempty"`
	// ReadMarked - xabarlar o'qilgan deb belgilanganmi.
	ReadMarked bool `gorm:"not null;default:false" json:"read_marked"`
	// ChatResolved - javobdan keyin suhbat "hal qilindi" holatiga
	// o'tkazilganmi (support tizimida).
	ChatResolved bool `gorm:"not null;default:false" json:"chat_resolved"`
	// HelpSent - help matni Telegram guruhga yuborilganmi. help tasdiq
	// kutmaydi: xodimlar darhol xabardor bo'lishi kerak.
	HelpSent bool `gorm:"not null;default:false" json:"help_sent"`

	// Forced - qo'lda, tekshiruvsiz ishga tushirilganmi (oxirgi so'z
	// biz tomondan bo'lsa ham). Panelda ajratib ko'rsatiladi.
	Forced bool `gorm:"not null;default:false" json:"forced"`

	// Source - javob qayerdan paydo bo'lgan: "agent" (AI zanjiri) yoki
	// "telegram" (xodim guruhda reply qilgan, LLM uni mijoz tiliga
	// moslab yozgan).
	Source string `gorm:"size:16;index;not null;default:agent" json:"source"`

	Status     string `gorm:"size:16;index;not null;default:pending" json:"status"`
	HandledBy  string `gorm:"size:64" json:"handled_by,omitempty"` // tasdiqlagan admin logini
	StepsCount int    `json:"steps_count"`
	Error      string `gorm:"type:text" json:"error,omitempty"`

	// Token hisobi (zanjirdagi barcha so'rovlar yig'indisi).
	Model            string  `gorm:"size:64" json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Calls            int     `json:"calls"`
	CostUSD          float64 `json:"cost_usd"`

	SentAt    *time.Time `json:"sent_at,omitempty"`
	CreatedAt time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	Steps []AgentStep `gorm:"foreignKey:InteractionID" json:"steps,omitempty"`
}

// AgentStep - zanjirning bitta bosqichi (panelda "AI qanday o'yladi").
type AgentStep struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	InteractionID uint   `gorm:"index;not null" json:"interaction_id"`
	StepNo        int    `json:"step_no"`
	PromtID       uint   `json:"promt_id"`
	PromtTitle    string `gorm:"size:255" json:"promt_title"`

	RequestContext string `gorm:"type:text" json:"request_context"` // modelga ketgan user matni
	RawResponse    string `gorm:"type:text" json:"raw_response"`    // model qaytargan asl JSON

	PromptTokens     int   `json:"prompt_tokens"`
	CachedTokens     int   `json:"cached_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	DurationMS       int64 `json:"duration_ms"`

	CreatedAt time.Time `json:"created_at"`
}

// ConversationState - poller uchun: qaysi suhbat qayergacha ishlangan.
type ConversationState struct {
	ConversationID int64      `gorm:"primaryKey" json:"conversation_id"`
	ClientID       int64      `json:"client_id"`
	LastMessageID  int64      `json:"last_message_id"`
	LastMessageAt  string     `gorm:"size:64" json:"last_message_at"`
	LastHandledAt  *time.Time `json:"last_handled_at,omitempty"`
	Skip           bool       `gorm:"not null;default:false" json:"skip"` // qo'lda o'chirib qo'yilgan
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Setting - global sozlamalar (auto_reply, poll_enabled).
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"size:255;not null" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveInteraction interaksiyani bosqichlari bilan birga yozadi.
func SaveInteraction(db *gorm.DB, in *Interaction) error {
	return db.Session(&gorm.Session{FullSaveAssociations: true}).Create(in).Error
}

// GetInteraction id bo'yicha interaksiyani bosqichlari bilan qaytaradi.
func GetInteraction(db *gorm.DB, id uint) (*Interaction, error) {
	var in Interaction
	err := db.Preload("Steps", func(d *gorm.DB) *gorm.DB {
		return d.Order("step_no asc")
	}).First(&in, id).Error
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// ListInteractions ro'yxat (status bo'yicha filtr, sahifalash).
func ListInteractions(db *gorm.DB, status string, page, limit int) ([]Interaction, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := db.Model(&Interaction{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []Interaction
	err := q.Order("id desc").Offset((page - 1) * limit).Limit(limit).Find(&list).Error
	return list, total, err
}

// applyUsage - sarflangan tokenlar va hisoblangan narxni interaksiyaga
// ko'chiradi.
func (in *Interaction) applyUsage(u Usage) {
	in.Model = u.Model
	in.PromptTokens = u.PromptTokens
	in.CachedTokens = u.CachedTokens
	in.CompletionTokens = u.CompletionTokens
	in.Calls = u.Calls
	in.CostUSD = u.Cost()
}

// markSent - javob mijozga ketdi deb belgilaydi.
func (in *Interaction) markSent(by string) {
	in.Status = StatusSent
	in.HandledBy = by
	now := time.Now()
	in.SentAt = &now
}
