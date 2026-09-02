// Muammoli buyurtmalar: aniqlash, saqlash va hal bo'lishini kuzatish.
//
// "Muammoli" qarorini KOD chiqaradi, model emas: sana ayirmasini modelga
// ishonib bo'lmaydi, u o'zidan kun to'qiydi.
package support

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Adminka status kodlari (ma'nosi ma'lum bo'lganlari).
const (
	StatusPaid     = 3 // sotib olingan, to'langan
	StatusWaiting  = 4 // kiritish uchun kutilmoqda
	StatusFinished = 6 // yakunlangan
)

// DefaultProblemDays - shu kundan ko'p turgan buyurtma muammoli hisoblanadi.
const DefaultProblemDays = 3

// DefaultRemindHours - hal bo'lmagan muammo qaytadan eslatiladigan oraliq.
const DefaultRemindHours = 24

// adminkaTimeLayout - adminka qaytaradigan sana ko'rinishi.
const adminkaTimeLayout = "2006-01-02 15:04:05"

// Muammo holatlari.
const (
	IssueOpen     = "open"
	IssueResolved = "resolved"
)

// Qanday yo'l bilan hal bo'lgani.
const (
	ResolvedViaTelegram = "telegram" // xodim guruhda reply qildi
	ResolvedViaChat     = "chat"     // xodim mijozga support chatda javob berdi
	ResolvedViaAuto     = "auto"     // adminkada status o'zgardi
	ResolvedViaPanel    = "panel"    // admin paneldan yopdi
)

// OrderIssue - qotib qolgan buyurtma va uning yechimi.
type OrderIssue struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	OrderSN        string `gorm:"size:64;index;not null" json:"order_sn"`
	ClientID       int64  `gorm:"index" json:"client_id"`
	ConversationID int64  `json:"conversation_id"`

	Status        int    `json:"status"`
	StatusLabel   string `gorm:"size:64" json:"status_label"`
	DaysSincePaid int    `json:"days_since_paid"`
	PackageName   string `gorm:"size:255" json:"package_name,omitempty"`
	PaidAt        string `gorm:"size:32" json:"paid_at,omitempty"`

	State string `gorm:"size:16;index;not null;default:open" json:"state"`

	// Telegram: qaysi xabarga reply qilinsa shu muammo yopiladi.
	TgMessageID    int64      `gorm:"index" json:"tg_message_id,omitempty"`
	NotifyCount    int        `json:"notify_count"`
	LastNotifiedAt *time.Time `json:"last_notified_at,omitempty"`

	Resolution  string     `gorm:"type:text" json:"resolution,omitempty"`
	ResolvedBy  string     `gorm:"size:64" json:"resolved_by,omitempty"`
	ResolvedVia string     `gorm:"size:16" json:"resolved_via,omitempty"`
	ResolvedAt  *time.Time `gorm:"index" json:"resolved_at,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StatusLabel - status raqamining o'zbekcha nomi.
func StatusLabel(status int) string {
	switch status {
	case StatusPaid:
		return "sotib olingan, to'langan"
	case StatusWaiting:
		return "kiritish uchun kutilmoqda"
	case StatusFinished:
		return "yakunlangan"
	}
	return fmt.Sprintf("holat %d", status)
}

// ProblemDays - .env dagi PROBLEM_DAYS (default 3).
func ProblemDays() int { return envInt("PROBLEM_DAYS", DefaultProblemDays) }

// RemindHours - .env dagi ISSUE_REMIND_HOURS (default 24).
func RemindHours() int {
	v := envStr("ISSUE_REMIND_HOURS", "")
	if v == "" {
		return DefaultRemindHours
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return DefaultRemindHours
	}
	return n
}

// ProblemStatuses - kuzatiladigan statuslar (.env: PROBLEM_STATUSES="3,4").
func ProblemStatuses() []int {
	v := envStr("PROBLEM_STATUSES", "")
	if v == "" {
		return []int{StatusPaid, StatusWaiting}
	}
	var out []int
	for _, p := range strings.Split(v, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return []int{StatusPaid, StatusWaiting}
	}
	return out
}

// parseAdminkaTime adminka sanasini o'qiydi ("2026-08-01 15:35:00").
func parseAdminkaTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(adminkaTimeLayout, s, time.Local)
	if err != nil {
		// Ba'zan ISO ko'rinishida kelishi mumkin.
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}

// IsPaid - buyurtma to'langanmi (pay_status: 1 — to'langan, 0 — yo'q).
func IsPaid(o AdminkaOrder) bool { return o.PayStatus == 1 }

// PaidTime - to'lov vaqti. Adminka `paid_at` bermasa buyurtma yaratilgan
// vaqtga qaytiladi (eski yozuvlarda `paid_at` bo'sh bo'lishi mumkin).
func PaidTime(o AdminkaOrder) (time.Time, bool) {
	if t, ok := parseAdminkaTime(o.PaidAt); ok {
		return t, true
	}
	return parseAdminkaTime(o.CreatedAt)
}

// DaysSincePaid - to'lov qilingan vaqtdan bugungacha o'tgan kun.
// Sana o'qilmasa -1.
func DaysSincePaid(o AdminkaOrder) int {
	t, ok := PaidTime(o)
	if !ok {
		return -1
	}
	return int(time.Since(t).Hours() / 24)
}

// IsProblem - buyurtma muammolimi: TO'LANGAN, kuzatiladigan statusda va
// to'lovdan beri PROBLEM_DAYS dan ko'p vaqt o'tgan.
//
// To'lanmagan buyurtma (pay_status = 0) kutib turishi normal — u xodimlar
// aralashuvini talab qilmaydi.
func IsProblem(o AdminkaOrder) bool {
	if !IsPaid(o) {
		return false
	}
	watched := false
	for _, s := range ProblemStatuses() {
		if o.Status == s {
			watched = true
			break
		}
	}
	if !watched {
		return false
	}
	d := DaysSincePaid(o)
	return d > ProblemDays()
}

// OrderView - modelga ketadigan boyitilgan buyurtma: xom status raqami
// o'rniga tushunarli maydonlar.
type OrderView struct {
	AdminkaOrder
	StatusLabel   string `json:"status_label"`
	Paid          bool   `json:"paid"`            // to'langanmi (pay_status)
	DaysSincePaid int    `json:"days_since_paid"` // to'lovdan beri necha kun
	Problem       bool   `json:"problem"`
	InReview      bool   `json:"tekshiruvda"` // ochiq muammo bor — xodimlar xabardor
}

// NewOrderView buyurtmani boyitadi.
func NewOrderView(o AdminkaOrder) OrderView {
	v := OrderView{
		AdminkaOrder:  o,
		StatusLabel:   StatusLabel(o.Status),
		Paid:          IsPaid(o),
		DaysSincePaid: DaysSincePaid(o),
		Problem:       IsProblem(o),
	}
	// To'lanmagan buyurtmada "to'lovdan beri N kun" ma'nosiz.
	if !v.Paid {
		v.DaysSincePaid = 0
	}
	return v
}

// FindOpenIssue - buyurtma bo'yicha ochiq muammo (bo'lmasa nil).
func FindOpenIssue(db *gorm.DB, orderSN string) *OrderIssue {
	var is OrderIssue
	err := db.Where("order_sn = ? AND state = ?", orderSN, IssueOpen).First(&is).Error
	if err != nil {
		return nil
	}
	return &is
}

// LastIssue - buyurtma bo'yicha eng oxirgi yozuv (ochiq yoki yopilgan).
// Yopilgan muammo qayta ko'tarilishi kerakmi-yo'qmi degan qarorda
// ishlatiladi.
func LastIssue(db *gorm.DB, orderSN string) *OrderIssue {
	var is OrderIssue
	if err := db.Where("order_sn = ?", orderSN).Order("id desc").First(&is).Error; err != nil {
		return nil
	}
	return &is
}

// ResolveIssue muammoni yopadi va yechimni saqlaydi.
func ResolveIssue(db *gorm.DB, is *OrderIssue, resolution, by, via string) error {
	now := time.Now()
	is.State = IssueResolved
	is.Resolution = resolution
	is.ResolvedBy = by
	is.ResolvedVia = via
	is.ResolvedAt = &now
	return db.Model(is).Updates(map[string]any{
		"state": IssueResolved, "resolution": resolution,
		"resolved_by": by, "resolved_via": via, "resolved_at": &now,
	}).Error
}

// ListIssues - muammolar ro'yxati (state bo'yicha filtr, sahifalash).
func ListIssues(db *gorm.DB, state string, page, limit int) ([]OrderIssue, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := db.Model(&OrderIssue{})
	if state != "" {
		q = q.Where("state = ?", state)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []OrderIssue
	err := q.Order("state asc, id desc").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error
	return rows, total, err
}
