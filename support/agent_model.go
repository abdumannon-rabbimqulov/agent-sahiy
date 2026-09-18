// Bazadagi agent modellari: mijoz murojaati va unga tayyorlangan javob
// (Interaction), zanjirning har bosqichi (AgentStep), suhbatning
// ishlanganlik holati va global sozlama yozuvi.
package support

import (
	"fmt"
	"log"
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

	// Overdue - "pending" holatida PendingOverdueHours dan ko'p vaqt
	// turgan (mijoz qayta yozmagan, admin ham tasdiqlamagan). Bazada
	// saqlanmaydi — ro'yxat chiqarilganda hisoblanadi.
	Overdue bool `gorm:"-" json:"overdue,omitempty"`
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

// PendingOverdueHours - shuncha soatdan ko'p "pending" turgan (admin
// tasdiqlamagan, mijoz ham qayta yozmagan) javob "yana ko'rib chiqish"
// uchun ro'yxat boshiga chiqariladi. .env: PENDING_OVERDUE_HOURS
// (standart 3).
const DefaultPendingOverdueHours = 3

func PendingOverdueHours() int { return envInt("PENDING_OVERDUE_HOURS", DefaultPendingOverdueHours) }

// DefaultStalePendingHours - shuncha soatdan ko'p tasdiqlanmagan
// "pending" javob endi dolzarb emas deb hisoblanadi va avtomatik bekor
// qilinadi. .env: STALE_PENDING_HOURS (0 — o'chirilgan).
const DefaultStalePendingHours = 24

func StalePendingHours() int { return envInt("STALE_PENDING_HOURS", DefaultStalePendingHours) }

// RejectStalePending - StalePendingHours dan ko'p vaqt tasdiqlanmagan
// "pending" javoblarni bekor qiladi. Mijoz uzoq vaqt kutgan, admin
// ulgurmagan javob endi dolzarb emas deb hisoblanadi (mijoz vaziyati
// o'zgargan yoki boshqa yo'l bilan javob olgan bo'lishi mumkin) —
// bunday yozuvlar navbatda cheksiz to'planib, yangi, hali dolzarb
// javoblarni ko'zdan yashirmasin.
//
// Eski javob bekor qilingani bilan mijoz javob olganicha yo'q —
// shuning uchun tegishli suhbatning ConversationState'i ham tozalanadi
// (qo'lda o'chirilgan — Skip — suhbatlar bundan mustasno): keyingi
// poller siklida bu suhbat "hali javob berilmagan" deb qayta ko'riladi
// va AGENT uni qaytadan o'rganib, yangi javob tayyorlaydi.
func RejectStalePending(db *gorm.DB) (int64, error) {
	hours := StalePendingHours()
	if hours <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	var stale []Interaction
	if err := db.Where("status = ? AND created_at < ?", StatusPending, cutoff).
		Find(&stale).Error; err != nil {
		return 0, err
	}
	if len(stale) == 0 {
		return 0, nil
	}

	ids := make([]uint, 0, len(stale))
	convIDs := make([]int64, 0, len(stale))
	seen := map[int64]bool{}
	for _, in := range stale {
		ids = append(ids, in.ID)
		if !seen[in.ConversationID] {
			seen[in.ConversationID] = true
			convIDs = append(convIDs, in.ConversationID)
		}
	}

	res := db.Model(&Interaction{}).Where("id IN ?", ids).Updates(map[string]any{
		"status": StatusRejected,
		"error":  fmt.Sprintf("%d soatdan ko'p tasdiqlanmadi — avtomatik eskirgan deb bekor qilindi", hours),
	})
	if res.Error != nil {
		return 0, res.Error
	}

	if err := db.Model(&ConversationState{}).
		Where("conversation_id IN ? AND skip = ?", convIDs, false).
		Updates(map[string]any{"last_message_id": 0, "last_message_at": ""}).Error; err != nil {
		log.Printf("eskirgan javoblar: suhbat holatini tozalash: %v", err)
	}

	return res.RowsAffected, nil
}

// ListInteractions ro'yxat (status bo'yicha filtr, sahifalash).
// Har doim eng yangisidan eskisiga qarab chiqadi ("id desc").
// "pending" uchun har bir yozuvga Overdue belgisi ham hisoblanadi
// (qarang: PendingOverdueHours) — uzoq kutganini panelda ajratib
// ko'rsatish uchun, tartibga tegmaydi.
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
	if err != nil {
		return nil, 0, err
	}
	if status == StatusPending {
		cutoff := time.Now().Add(-time.Duration(PendingOverdueHours()) * time.Hour)
		for i := range list {
			list[i].Overdue = list[i].CreatedAt.Before(cutoff)
		}
	}
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
