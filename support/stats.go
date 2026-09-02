package support

import (
	"time"

	"gorm.io/gorm"
)

// Stats - admin panel bosh sahifasi uchun umumiy hisob.
type Stats struct {
	// Murojaatlar
	Total       int64 `json:"total"`        // jami interaksiya
	Sent        int64 `json:"sent"`         // avtomatik yuborilgan
	Approved    int64 `json:"approved"`     // admin tasdiqlagan
	Pending     int64 `json:"pending"`      // tasdiq kutmoqda
	Rejected    int64 `json:"rejected"`     // rad etilgan
	Failed      int64 `json:"failed"`       // xato
	AIResolved  int64 `json:"ai_resolved"`  // AI o'zi hal qilgan (help'siz yuborilgan)
	NeededStaff int64 `json:"needed_staff"` // xodim aralashuvi kerak bo'lgan (help bor)

	// Bugungi kesim. Tasdiqlash/yuborish `sent_at` bo'yicha sanaladi,
	// `created_at` bo'yicha emas: kecha kelgan murojaatni bugun
	// tasdiqlash mumkin — u bugungi ish hisoblanadi.
	TotalToday    int64 `json:"total_today"`    // bugun kelgan murojaat
	SentToday     int64 `json:"sent_today"`     // bugun avtomatik yuborilgan
	ApprovedToday int64 `json:"approved_today"` // bugun admin tasdiqlab yuborgan
	RejectedToday int64 `json:"rejected_today"` // bugun rad etilgan

	// Mijozlar
	UniqueClients int64 `json:"unique_clients"`
	UniqueChats   int64 `json:"unique_chats"`

	// Tokenlar va xarajat
	PromptTokens     int64   `json:"prompt_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Calls            int64   `json:"calls"`
	CostTotal        float64 `json:"cost_total"`
	CostToday        float64 `json:"cost_today"`
	CostMonth        float64 `json:"cost_month"`
	TokensToday      int64   `json:"tokens_today"`

	// Muammoli buyurtmalar (kunlik hisobot uchun)
	IssuesOpen          int64   `json:"issues_open"`
	IssuesOpenedToday   int64   `json:"issues_opened_today"`
	IssuesResolvedToday int64   `json:"issues_resolved_today"`
	IssuesAvgHours      float64 `json:"issues_avg_hours"`
}

// IssueStats - muammolar bo'yicha umumiy hisob.
type IssueStats struct {
	Open          int64   `json:"issues_open"`
	OpenedToday   int64   `json:"issues_opened_today"`
	ResolvedToday int64   `json:"issues_resolved_today"`
	AvgHours      float64 `json:"issues_avg_hours"`
}

// GetIssueStats - muammolar bo'yicha raqamlar.
func GetIssueStats(db *gorm.DB) (IssueStats, error) {
	var s IssueStats
	err := db.Model(&OrderIssue{}).Select(`
		COUNT(*) FILTER (WHERE state = 'open') AS open,
		COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now())) AS opened_today,
		COUNT(*) FILTER (WHERE resolved_at >= date_trunc('day', now())) AS resolved_today,
		COALESCE(AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 3600)
			FILTER (WHERE resolved_at IS NOT NULL), 0) AS avg_hours
	`).Scan(&s).Error
	return s, err
}

// IssueDailyStat - kunlik hisobot qatori.
type IssueDailyStat struct {
	Day      time.Time `json:"day"`
	Opened   int64     `json:"opened"`
	Resolved int64     `json:"resolved"`
}

// IssueDailyStats - oxirgi `days` kunda ochilgan va yopilgan muammolar.
func IssueDailyStats(db *gorm.DB, days int) ([]IssueDailyStat, error) {
	if days < 1 || days > 365 {
		days = 30
	}
	var out []IssueDailyStat
	err := db.Raw(`
		SELECT d.day,
		       COALESCE(o.cnt, 0) AS opened,
		       COALESCE(r.cnt, 0) AS resolved
		FROM generate_series(date_trunc('day', now()) - make_interval(days => ?),
		                     date_trunc('day', now()), '1 day') AS d(day)
		LEFT JOIN (SELECT date_trunc('day', created_at) AS day, COUNT(*) AS cnt
		             FROM order_issues GROUP BY 1) o ON o.day = d.day
		LEFT JOIN (SELECT date_trunc('day', resolved_at) AS day, COUNT(*) AS cnt
		             FROM order_issues WHERE resolved_at IS NOT NULL GROUP BY 1) r ON r.day = d.day
		ORDER BY d.day DESC`, days-1).Scan(&out).Error
	return out, err
}

// GetStats umumiy hisobni bitta so'rovda yig'adi.
func GetStats(db *gorm.DB) (Stats, error) {
	var s Stats

	row := db.Model(&Interaction{}).Select(`
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE status = 'sent')     AS sent,
		COUNT(*) FILTER (WHERE status = 'approved') AS approved,
		COUNT(*) FILTER (WHERE status = 'pending')  AS pending,
		COUNT(*) FILTER (WHERE status = 'rejected') AS rejected,
		COUNT(*) FILTER (WHERE status = 'failed')   AS failed,
		COUNT(*) FILTER (WHERE status IN ('sent','approved') AND (help_text = '' OR help_text IS NULL)) AS ai_resolved,
		COUNT(*) FILTER (WHERE help_text <> '')     AS needed_staff,
		COUNT(*) FILTER (WHERE created_at >= date_trunc('day', now()))                              AS total_today,
		COUNT(*) FILTER (WHERE status = 'sent'     AND sent_at    >= date_trunc('day', now()))      AS sent_today,
		COUNT(*) FILTER (WHERE status = 'approved' AND sent_at    >= date_trunc('day', now()))      AS approved_today,
		COUNT(*) FILTER (WHERE status = 'rejected' AND updated_at >= date_trunc('day', now()))      AS rejected_today,
		COUNT(DISTINCT client_id)       AS unique_clients,
		COUNT(DISTINCT conversation_id) AS unique_chats,
		COALESCE(SUM(prompt_tokens),0)     AS prompt_tokens,
		COALESCE(SUM(cached_tokens),0)     AS cached_tokens,
		COALESCE(SUM(completion_tokens),0) AS completion_tokens,
		COALESCE(SUM(calls),0)             AS calls,
		COALESCE(SUM(cost_usd),0)          AS cost_total,
		COALESCE(SUM(cost_usd) FILTER (WHERE created_at >= date_trunc('day', now())),0)   AS cost_today,
		COALESCE(SUM(cost_usd) FILTER (WHERE created_at >= date_trunc('month', now())),0) AS cost_month,
		COALESCE(SUM(prompt_tokens + completion_tokens) FILTER (WHERE created_at >= date_trunc('day', now())),0) AS tokens_today
	`)
	if err := row.Scan(&s).Error; err != nil {
		return s, err
	}
	s.TotalTokens = s.PromptTokens + s.CompletionTokens

	is, err := GetIssueStats(db)
	if err != nil {
		return s, err
	}
	s.IssuesOpen = is.Open
	s.IssuesOpenedToday = is.OpenedToday
	s.IssuesResolvedToday = is.ResolvedToday
	s.IssuesAvgHours = is.AvgHours
	return s, nil
}

// DailyStat - bir kunlik kesim (grafik uchun).
type DailyStat struct {
	Day        time.Time `json:"day"`
	Total      int64     `json:"total"`
	Sent       int64     `json:"sent"`
	Pending    int64     `json:"pending"`
	Failed     int64     `json:"failed"`
	Tokens     int64     `json:"tokens"`
	Cost       float64   `json:"cost"`
	AIResolved int64     `json:"ai_resolved"`
}

// DailyStats oxirgi `days` kunlik kesim (yangisidan eskisiga).
func DailyStats(db *gorm.DB, days int) ([]DailyStat, error) {
	if days < 1 || days > 365 {
		days = 30
	}
	var out []DailyStat
	err := db.Model(&Interaction{}).
		Select(`
			date_trunc('day', created_at) AS day,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status IN ('sent','approved')) AS sent,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'failed')  AS failed,
			COUNT(*) FILTER (WHERE status IN ('sent','approved') AND (help_text = '' OR help_text IS NULL)) AS ai_resolved,
			COALESCE(SUM(prompt_tokens + completion_tokens),0) AS tokens,
			COALESCE(SUM(cost_usd),0) AS cost`).
		Where("created_at >= now() - make_interval(days => ?)", days).
		Group("day").Order("day desc").Scan(&out).Error
	return out, err
}

// ClientStat - bitta mijoz kesimi.
type ClientStat struct {
	ClientID   int64     `json:"client_id"`
	Total      int64     `json:"total"`
	AIResolved int64     `json:"ai_resolved"`
	NeededHelp int64     `json:"needed_help"`
	Pending    int64     `json:"pending"`
	Tokens     int64     `json:"tokens"`
	Cost       float64   `json:"cost"`
	LastAt     time.Time `json:"last_at"`
}

// ClientStats mijozlar kesimi (eng ko'p murojaat qilgani birinchi).
func ClientStats(db *gorm.DB, days, limit int) ([]ClientStat, error) {
	if days < 1 || days > 365 {
		days = 30
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var out []ClientStat
	err := db.Model(&Interaction{}).
		Select(`
			client_id,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status IN ('sent','approved') AND (help_text = '' OR help_text IS NULL)) AS ai_resolved,
			COUNT(*) FILTER (WHERE help_text <> '') AS needed_help,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COALESCE(SUM(prompt_tokens + completion_tokens),0) AS tokens,
			COALESCE(SUM(cost_usd),0) AS cost,
			MAX(created_at) AS last_at`).
		Where("created_at >= now() - make_interval(days => ?)", days).
		Group("client_id").Order("total desc").Limit(limit).Scan(&out).Error
	return out, err
}
