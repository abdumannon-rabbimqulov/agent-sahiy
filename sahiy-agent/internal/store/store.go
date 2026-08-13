// Package store — suhbatlar tarixi (GORM).
package store

import (
	"fmt"
	"time"

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

// Get bitta yozuvni id bo'yicha qaytaradi.
func (s *Store) Get(id uint) (*models.Interaction, error) {
	var in models.Interaction
	if err := s.db.First(&in, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &in, nil
}

// MarkSent admin tekshirib tasdiqlagan javobni "yuborildi" deb belgilaydi.
func (s *Store) MarkSent(id uint, by string) error {
	now := time.Now()
	return s.db.Model(&models.Interaction{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":      models.StatusAISent,
			"sent":        true,
			"handled_by":  by,
			"resolved_at": now,
		}).Error
}

// MarkRejected admin rad etgan javobni belgilaydi (mijozga yuborilmaydi).
func (s *Store) MarkRejected(id uint, by string) error {
	now := time.Now()
	return s.db.Model(&models.Interaction{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":      models.StatusRejected,
			"sent":        false,
			"handled_by":  by,
			"resolved_at": now,
		}).Error
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
	// Muammo holatlari bo'yicha.
	AIResolved    int `json:"AIResolved"`    // AI o'zi hal qilgan
	Pending       int `json:"Pending"`       // xodim javobi kutilmoqda
	StaffResolved int `json:"StaffResolved"` // xodim hal qilgan
	// AI xarajati (tokenlar provayder qaytargan aniq sonlar).
	TokensTotal int64   `json:"TokensTotal"`
	CostTotal   float64 `json:"CostTotal"`
	CostToday   float64 `json:"CostToday"`
	CostMonth   float64 `json:"CostMonth"`
}

// Stats statistikani hisoblaydi.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	err := s.db.Model(&models.Interaction{}).
		Select(`COUNT(*) AS total_replies,
		        COUNT(*) FILTER (WHERE sent) AS sent_replies,
		        COUNT(DISTINCT client_id) FILTER (WHERE client_id <> 0) AS unique_clients,
		        COUNT(DISTINCT conversation_id) AS unique_chats,
		        COUNT(*) FILTER (WHERE status = ?) AS ai_resolved,
		        COUNT(*) FILTER (WHERE status = ?) AS pending,
		        COUNT(*) FILTER (WHERE status = ?) AS staff_resolved,
		        COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens_total,
		        COALESCE(SUM(cost_usd), 0) AS cost_total,
		        COALESCE(SUM(cost_usd) FILTER (WHERE created >= date_trunc('day', now())), 0) AS cost_today,
		        COALESCE(SUM(cost_usd) FILTER (WHERE created >= date_trunc('month', now())), 0) AS cost_month`,
			models.StatusAISent, models.StatusPending, models.StatusStaffSent).
		Scan(&st).Error
	return st, err
}

// DailyCost — bir kunning (model bo'yicha) xarajati.
type DailyCost struct {
	Day              string  `json:"day"` // YYYY-MM-DD
	Model            string  `json:"model"`
	Replies          int     `json:"replies"`
	AICalls          int     `json:"ai_calls"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// sinceDays davr boshlanishini qaytaradi: bugundan orqaga days kun (bugun ham
// kiradi), kun boshidan. days <= 0 bo'lsa ok=false — davr cheklanmaydi
// (butun tarix).
//
// Kesim sanasi Go tomonda hisoblanadi — Postgres'da parametr turini aniqlash
// muammosi bo'lmasin (make_interval(days => $1) xato beradi).
func sinceDays(days int) (time.Time, bool) {
	if days <= 0 {
		return time.Time{}, false
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -(days - 1)), true
}

// withPeriod so'rovga davr shartini qo'shadi (davr cheklanmagan bo'lsa —
// tegmaydi) va AI so'rovi bo'lmagan yozuvlarni chiqarib tashlaydi.
func withPeriod(q *gorm.DB, days int) *gorm.DB {
	if since, ok := sinceDays(days); ok {
		q = q.Where("created >= ?", since)
	}
	return q.Where("ai_calls > 0")
}

// orderBy — kesim jadvallari uchun saralash: "last" bo'lsa oxirgi faollik
// bo'yicha, aks holda xarajat bo'yicha (qimmatlari tepada).
func orderBy(sort string) string {
	if sort == "last" {
		return "MAX(created) DESC"
	}
	return "SUM(cost_usd) DESC, MAX(created) DESC"
}

// capLimit — qatorlar soni chegarasi.
func capLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return min(limit, 500)
}

// Daily oxirgi n kunlik xarajatni kun va model bo'yicha guruhlab qaytaradi
// (eng yangi kun birinchi). days <= 0 — butun tarix.
// AI so'rovi bo'lmagan yozuvlar hisobga olinmaydi.
func (s *Store) Daily(days int) ([]DailyCost, error) {
	var out []DailyCost
	q := withPeriod(s.db.Model(&models.Interaction{}), days)
	err := q.
		Select(`to_char(date_trunc('day', created), 'YYYY-MM-DD') AS day,
		        COALESCE(NULLIF(model, ''), '—') AS model,
		        COUNT(*) AS replies,
		        COALESCE(SUM(ai_calls), 0) AS ai_calls,
		        COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		        COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		        COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		        COALESCE(SUM(cost_usd), 0) AS cost_usd`).
		Group("1, 2").Order("1 desc, 2").
		Scan(&out).Error
	return out, err
}

// ClientCost — bitta mijozga ketgan AI xarajati (barcha suhbatlari bo'yicha).
type ClientCost struct {
	ClientID         int64     `json:"client_id"`
	ClientName       string    `json:"client_name"`
	Conversations    int       `json:"conversations"`
	Replies          int       `json:"replies"`
	AICalls          int       `json:"ai_calls"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CachedTokens     int64     `json:"cached_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	LastAt           time.Time `json:"last_at"`
}

// ByClient xarajatni mijozlar kesimida qaytaradi.
// sort: "last" — oxirgi faollik bo'yicha, aks holda xarajat bo'yicha.
func (s *Store) ByClient(days, limit int, sort string) ([]ClientCost, error) {
	var out []ClientCost
	q := withPeriod(s.db.Model(&models.Interaction{}), days)
	err := q.Select(`client_id,
		        COALESCE(NULLIF((array_agg(client_name ORDER BY created DESC))[1], ''), '—') AS client_name,
		        COUNT(DISTINCT conversation_id) AS conversations,
		        COUNT(*) AS replies,
		        COALESCE(SUM(ai_calls), 0) AS ai_calls,
		        COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		        COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		        COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		        COALESCE(SUM(cost_usd), 0) AS cost_usd,
		        MAX(created) AS last_at`).
		Group("client_id").Order(orderBy(sort)).Limit(capLimit(limit)).
		Scan(&out).Error
	return out, err
}

// ConversationCost — bitta muammoning (suhbatning) to'liq tannarxi: undagi
// barcha javoblar — AI o'zi hal qilgani ham, xodimga chiqqani ham — jamlanadi.
type ConversationCost struct {
	ConversationID   int64     `json:"conversation_id"`
	ClientID         int64     `json:"client_id"`
	ClientName       string    `json:"client_name"`
	Title            string    `json:"title"`
	Replies          int       `json:"replies"`
	AICalls          int       `json:"ai_calls"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CachedTokens     int64     `json:"cached_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	Status           string    `json:"status"`    // oxirgi holat
	Escalated        bool      `json:"escalated"` // xodimlar guruhiga chiqqanmi
	FirstAt          time.Time `json:"first_at"`
	LastAt           time.Time `json:"last_at"`
}

// ByConversation xarajatni muammolar (suhbatlar) kesimida qaytaradi.
func (s *Store) ByConversation(days, limit int, sort string) ([]ConversationCost, error) {
	var out []ConversationCost
	q := withPeriod(s.db.Model(&models.Interaction{}), days)
	err := q.Select(`conversation_id,
		        MAX(client_id) AS client_id,
		        COALESCE(NULLIF((array_agg(client_name ORDER BY created DESC))[1], ''), '—') AS client_name,
		        COALESCE((array_agg(title ORDER BY created DESC))[1], '') AS title,
		        COUNT(*) AS replies,
		        COALESCE(SUM(ai_calls), 0) AS ai_calls,
		        COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		        COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		        COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		        COALESCE(SUM(cost_usd), 0) AS cost_usd,
		        (array_agg(status ORDER BY created DESC))[1] AS status,
		        bool_or(escalation_id IS NOT NULL) AS escalated,
		        MIN(created) AS first_at,
		        MAX(created) AS last_at`).
		Group("conversation_id").Order(orderBy(sort)).Limit(capLimit(limit)).
		Scan(&out).Error
	return out, err
}

// MonthCost — shu oyning boshidan beri sarflangan summa (byudjet nazorati).
func (s *Store) MonthCost() (float64, error) {
	var v float64
	err := s.db.Model(&models.Interaction{}).
		Where("created >= date_trunc('month', now())").
		Select("COALESCE(SUM(cost_usd), 0)").Scan(&v).Error
	return v, err
}

// String qisqacha statistika.
func (st Stats) String() string {
	return fmt.Sprintf("odamlar: %d | suhbatlar: %d | javoblar: %d (yuborilgan: %d) | AI hal qildi: %d | jarayonda: %d | xodim hal qildi: %d | 💰 bugun $%.4f · oy $%.4f (%d token)",
		st.UniqueClients, st.UniqueChats, st.TotalReplies, st.SentReplies,
		st.AIResolved, st.Pending, st.StaffResolved,
		st.CostToday, st.CostMonth, st.TokensTotal)
}

// ResolveEscalation xodim javob berganda eskalatsiya yozuvini yangilaydi:
// status "jarayonda" dan "xodim hal qildi" ga o'tadi va javob matni yoziladi.
// Yangi qator qo'shilmaydi — dashboardda bitta muammo bitta qator bo'lib qoladi.
func (s *Store) ResolveEscalation(tgMessageID int64, answer, by string) (int64, error) {
	now := time.Now()
	res := s.db.Model(&models.Interaction{}).
		Where("escalation_id = ?", tgMessageID).
		Updates(map[string]any{
			"status":      models.StatusStaffSent,
			"handled_by":  by,
			"ai_reply":    answer,
			"sent":        true,
			"resolved_at": now,
		})
	return res.RowsAffected, res.Error
}
