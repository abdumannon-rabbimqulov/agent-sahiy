// Bazadagi agent modellari: mijoz murojaati va unga tayyorlangan javob
// (Interaction), zanjirning har bosqichi (AgentStep), suhbatning
// ishlanganlik holati va global sozlama yozuvi.
package support

import (
	"fmt"
	"log"
	"strconv"
	"strings"
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

	// ImageURL - mijozga javob bilan birga ketadigan rasm (xodim
	// guruhda javobiga rasm biriktirgan bo'lsa). Rasm MATNDAN OLDIN
	// yuboriladi — xodim guruhda ham shu tartibda yozadi: avval rasm,
	// keyin izoh. Havola support omboriga yuklangan bo'ladi
	// (storage.go), Telegram havolasi emas: uning ichida bot tokeni
	// turadi.
	ImageURL string `gorm:"size:512" json:"image_url,omitempty"`
	// ImageSent - rasm mijozga yuborilganmi. Matn yuborishda xato
	// bo'lsa qayta urinishda rasm IKKI marta ketib qolmasin.
	ImageSent bool `gorm:"not null;default:false" json:"image_sent"`

	// NumbersFromImage - javobdagi buyurtma/trek raqami mijoz yozgan
	// matndan emas, rasmdan (OCR) olinganmi. Dashboardda "Mijozga javob"
	// bo'limida belgi sifatida ko'rsatiladi — xodim javob qayerdan kelib
	// chiqqanini bilsin.
	NumbersFromImage bool `gorm:"not null;default:false" json:"numbers_from_image"`

	// ImageNoNumber - mijoz rasm yubordi, lekin OCR undan buyurtma/trek
	// raqamini topa olmadi (past sifat, notanish format va h.k.). Panelda
	// alohida belgi bilan ko'rsatiladi — xodim rasmni topilmadi holatida
	// ham bilib, o'zi tekshirib qo'ysin (aks holda "rasm yuborildi, hech
	// narsa ko'rinmadi" holati ko'zdan yashirin qolardi).
	ImageNoNumber bool `gorm:"not null;default:false" json:"image_no_number"`

	// MessageIDs - shu murojaatda javob berilayotgan mijoz xabarlari
	// ("1,2,3"). Javob mijozga yetib borgandan keyin shular o'qilgan
	// deb belgilanadi.
	MessageIDs string `gorm:"size:255" json:"message_ids,omitempty"`
	// ReadMarked - xabarlar o'qilgan deb belgilanganmi.
	ReadMarked bool `gorm:"not null;default:false" json:"read_marked"`
	// ChatResolved - javobdan keyin suhbat "hal qilindi" holatiga
	// o'tkazilganmi (support tizimida).
	ChatResolved bool `gorm:"not null;default:false" json:"chat_resolved"`
	// HelpSent - help matni Telegram guruhga yuborilganmi. Ikki marta
	// yuborilmasin uchun ham shu maydon tekshiriladi.
	HelpSent bool `gorm:"not null;default:false" json:"help_sent"`
	// HelpMessageID - guruhdagi help xabarining id'si. Xodim o'sha
	// xabarga reply qilsa, javob shu murojaat bo'yicha mijozga ketadi
	// (support/telegram_updates.go).
	HelpMessageID int64 `gorm:"index" json:"help_message_id,omitempty"`
	// HelpAnsweredAt, HelpAnsweredBy - guruhdagi yordam so'roviga
	// mutaxassis qachon va kim reply qilgani. Bo'sh bo'lsa so'rov hali
	// JAVOBSIZ — panelda "guruh javobi" hisoboti shu maydonga tayanadi.
	HelpAnsweredAt *time.Time `gorm:"index" json:"help_answered_at,omitempty"`
	HelpAnsweredBy string     `gorm:"size:64" json:"help_answered_by,omitempty"`

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

	// Alerts - KOD topgan holatlar (masalan posilka mijoz viloyatidan
	// boshqa filialda). Bazada saqlanmaydi — matni help ichiga
	// qo'shiladi. Shundaylar bo'lsa xabar guruhga "help guruhga
	// ketsinmi" sozlamasidan qat'i nazar yuboriladi: bu model fikri
	// emas, tekshirishni talab qiladigan aniq holat.
	Alerts []string `gorm:"-" json:"alerts,omitempty"`

	// NumberNote - xodim javobida yozgan buyurtma raqami guruh
	// xabaridagi buyurtmalarga tegishli emasligi haqida eslatma.
	// Bazada saqlanmaydi: guruhdagi tasdiq xabariga qo'shiladi, xodim
	// o'zining xatosini darhol ko'rsin (support/staff_reply.go).
	NumberNote string `gorm:"-" json:"-"`

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

// TelegramPost - xodimlar guruhiga BIZ yuborgan xabar va u qaysi
// suhbatga tegishli ekani.
//
// Nega kerak: xodim guruhdagi istalgan xabarimizga reply qilishi
// mumkin — "yordam kerak" matniga ham, muammo ro'yxatiga ham,
// eslatmaga ham, mijoz yuborgan RASMGA ham. Ilgari faqat ikkita yo'l
// tanilardi (order_issues.tg_message_id va interactions.help_message_id);
// rasm xabariga yozilgan javob esa hech qayerga bog'lanmay, jimgina
// yo'qolardi. Endi har bir xabar shu yerda qayd etiladi va javob
// baribir o'z suhbatini topadi.
type TelegramPost struct {
	MessageID      int64     `gorm:"primaryKey" json:"message_id"`
	ConversationID int64     `gorm:"index;not null" json:"conversation_id"`
	ClientID       int64     `json:"client_id"`
	InteractionID  uint      `json:"interaction_id,omitempty"`
	Kind           string    `gorm:"size:16" json:"kind"` // help | issue | remind | image
	CreatedAt      time.Time `json:"created_at"`
}

// RememberTelegramPost - guruhga ketgan xabarni qayd etadi. Xatolik
// zanjirni to'xtatmaydi: bu faqat javobni topishga yordam beradi.
func RememberTelegramPost(msgID, conversationID, clientID int64, kind string, interactionID uint) {
	if DB == nil || msgID == 0 || conversationID <= 0 {
		return
	}
	p := TelegramPost{
		MessageID: msgID, ConversationID: conversationID,
		ClientID: clientID, InteractionID: interactionID, Kind: kind,
	}
	if err := DB.Save(&p).Error; err != nil {
		log.Printf("telegram: xabar %d qayd etilmadi: %v", msgID, err)
	}
}

// FindTelegramPost - guruhdagi xabar qaysi suhbatga tegishli.
func FindTelegramPost(msgID int64) *TelegramPost {
	if DB == nil || msgID == 0 {
		return nil
	}
	var p TelegramPost
	if err := DB.First(&p, "message_id = ?", msgID).Error; err != nil {
		return nil
	}
	return &p
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

// ListInteractions ro'yxat (status va id bo'yicha filtr, sahifalash).
// Har doim eng yangisidan eskisiga qarab chiqadi ("id desc").
// "pending" uchun har bir yozuvga Overdue belgisi ham hisoblanadi
// (qarang: PendingOverdueHours) — uzoq kutganini panelda ajratib
// ko'rsatish uchun, tartibga tegmaydi.
//
// `search` — id bo'yicha qidiruv. Panelda odam qo'liga tushadigan uch xil
// raqam bor va ular bir-biriga o'xshaydi, shuning uchun qaysi biri
// yozilgani so'ralmaydi — UCHALASI ham tekshiriladi: murojaat id'si,
// suhbat id'si (#62139) va mijoz id'si (8485098). Raqam bo'lmasa qidiruv
// e'tiborga olinmaydi.
func ListInteractions(db *gorm.DB, status, search string, page, limit int) ([]Interaction, int64, error) {
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
	if id, ok := searchID(search); ok {
		q = q.Where("id = ? OR conversation_id = ? OR client_id = ?", id, id, id)
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

// searchID - qidiruv matnidan id ajratadi. Panelda raqam turli
// ko'rinishda yoziladi: "#62139", "8485098", bo'shliq bilan. Shulardan
// raqam bo'lmagan hamma belgi tashlanadi. Raqam qolmasa — qidiruv yo'q.
func searchID(s string) (int64, bool) {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(b.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
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
