// Kunlik hisobot: panel kartalaridagi "bugungi" ko'rsatkichlarning
// HAR KUN uchun varianti.
//
// Tarix alohida saqlanmaydi va saqlanishi ham shart emas: `interactions`
// va `order_issues` jadvallarida har bir yozuvning sanasi turadi
// (created_at, sent_at, updated_at, resolved_at), hech narsa
// o'chirilmaydi va ustiga yozilmaydi. "Bugungi hisobot" — shunchaki
// `WHERE created_at >= date_trunc('day', now())` filtri. Ya'ni istalgan
// kunning raqamini o'sha ma'lumotdan qayta hisoblash mumkin, shu fayl
// aynan shuni qiladi.
//
// Bitta istisno bor — `issues_reminded`, quyida izohlangan.
package support

import (
	"time"

	"gorm.io/gorm"
)

// MaxReportDays - bir so'rovda beriladigan eng ko'p kun.
const MaxReportDays = 365

// DefaultReportDays - `days` berilmaganda.
const DefaultReportDays = 30

// DailyReport - bitta kunning to'liq kesimi.
//
// Qaysi sana bo'yicha sanalgani ko'rsatkichdan ko'rsatkichga farq
// qiladi va bu ataylab: kecha kelgan murojaatni bugun tasdiqlash
// mumkin — u BUGUNGI ish hisoblanadi. Shuning uchun murojaat
// `created_at`, yuborilgani `sent_at`, rad etilgani `updated_at`
// bo'yicha sanaladi (GetStats dagi bilan bir xil).
type DailyReport struct {
	Day time.Time `json:"day"`

	// --- Murojaatlar ---
	Total        int64   `json:"total"`         // o'sha kuni kelgan murojaat
	Sent         int64   `json:"sent"`          // AI o'zi yuborgan
	Approved     int64   `json:"approved"`      // admin tasdiqlab yuborgan
	Rejected     int64   `json:"rejected"`      // rad etilgan
	StaffReplies int64   `json:"staff_replies"` // "xodim javobidan" tug'ilgan murojaat
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`

	// --- Telegram guruh ---
	HelpSent       int64 `json:"help_sent"`       // guruhga ketgan yordam so'rovi
	HelpAnswered   int64 `json:"help_answered"`   // shundan javob olgani
	HelpUnanswered int64 `json:"help_unanswered"` // shundan hali javobsizi

	// --- Muammoli buyurtmalar ---
	IssuesOpened   int64   `json:"issues_opened"`    // o'sha kuni aniqlangan
	IssuesResolved int64   `json:"issues_resolved"`  // o'sha kuni yopilgan
	IssuesOpenEnd  int64   `json:"issues_open_end"`  // kun OXIRIDA ochiq qolgani
	IssuesAvgHours float64 `json:"issues_avg_hours"` // o'sha kuni yopilganlarning o'rtachasi
	IssuesNotified int64   `json:"issues_notified"`  // guruhga chiqarilgani
	IssuesTelegram int64   `json:"issues_telegram"`  // guruhda reply bilan yopilgani

	// IssuesReminded - o'sha kuni takroriy eslatma ketgan muammolar.
	//
	// DIQQAT, bu son TAXMINIY. `order_issues.last_notified_at` — bitta
	// maydon va har eslatmada USTIGA YOZILADI, ya'ni bazada faqat
	// ENG OXIRGI eslatma sanasi qoladi. Bitta muammo 1- va 5-kuni
	// eslatilgan bo'lsa, u faqat 5-kunda ko'rinadi. Bugungi son
	// to'g'ri, o'tgan kunlarniki esa kamaytirilgan bo'lishi mumkin.
	// To'g'ri tarix uchun eslatmalarni alohida jadvalga yozish kerak.
	IssuesReminded int64 `json:"issues_reminded"`
}

// DailyReports - oxirgi `days` kunning to'liq kesimi (yangisidan
// eskisiga). Ma'lumot bo'lmagan kun ham qatorda turadi — nollar bilan.
//
// Hammasi BITTA so'rovda yig'iladi: har bir kesim o'z CTE'sida kun
// bo'yicha guruhlanadi va kunlar ro'yxatiga chapdan ulanadi.
func DailyReports(db *gorm.DB, days int) ([]DailyReport, error) {
	if days < 1 || days > MaxReportDays {
		days = DefaultReportDays
	}
	var out []DailyReport
	err := db.Transaction(func(tx *gorm.DB) error {
		// JIT bu so'rov uchun sof zarar. Rejalashtiruvchi CTE'lar
		// zanjiridagi qatorlar sonini juda yuqori baholaydi
		// (~6·10^8), baho esa jit_above_cost (100000) dan oshgani
		// uchun Postgres so'rovni mashina kodiga kompilyatsiya
		// qiladi. O'lchov: kompilyatsiya ~470 ms, bajarish ~5 ms —
		// ya'ni vaqtning 94 foizi behuda ketadi.
		//
		// SET LOCAL faqat shu tranzaksiyaga ta'sir qiladi: qolgan
		// so'rovlar va boshqa ulanishlar tegilmaydi.
		if err := tx.Exec("SET LOCAL jit = off").Error; err != nil {
			return err
		}
		return tx.Raw(`
		WITH d AS (
		  SELECT generate_series(date_trunc('day', now()) - make_interval(days => ?),
		                         date_trunc('day', now()), '1 day') AS day
		),
		-- Oyna boshi. Har bir kesim shu sanadan keyingi yozuvlar bilan
		-- CHEGARALANADI: usiz har bir CTE butun jadvalni skanerlardi va
		-- so'rov kun soniga emas, jadval hajmiga qarab sekinlashardi
		-- (30 kunlik hisobot ham 365 kunlikcha vaqt olardi).
		w AS (SELECT MIN(day) AS from_day FROM d),
		-- Murojaatlar: kelgan sanasi bo'yicha.
		ix AS (
		  SELECT date_trunc('day', created_at) AS day,
		         COUNT(*)                                         AS total,
		         COUNT(*) FILTER (WHERE source = 'telegram')      AS staff_replies,
		         COALESCE(SUM(prompt_tokens + completion_tokens),0) AS tokens,
		         COALESCE(SUM(cost_usd),0)                        AS cost
		    FROM interactions, w WHERE created_at >= w.from_day GROUP BY 1
		),
		-- Yuborilgani: sent_at bo'yicha (kechagi murojaat bugun ketishi mumkin).
		snt AS (
		  SELECT date_trunc('day', sent_at) AS day,
		         COUNT(*) FILTER (WHERE status = 'sent')     AS sent,
		         COUNT(*) FILTER (WHERE status = 'approved') AS approved
		    FROM interactions, w WHERE sent_at >= w.from_day GROUP BY 1
		),
		-- Rad etilgani: updated_at bo'yicha.
		rej AS (
		  SELECT date_trunc('day', updated_at) AS day, COUNT(*) AS rejected
		    FROM interactions, w WHERE status = 'rejected' AND updated_at >= w.from_day GROUP BY 1
		),
		-- Guruhga ketgan yordam so'rovlari. "Javob olgan" sharti
		-- GroupHelpStats dagi bilan AYNAN bir xil bo'lishi kerak,
		-- aks holda bugungi karta va kunlik jadval har xil son
		-- ko'rsatadi.
		hlp AS (
		  SELECT date_trunc('day', h.created_at) AS day,
		         COUNT(*)                        AS help_sent,
		         COUNT(*) FILTER (WHERE a.ok)    AS help_answered
		    FROM interactions h
		    CROSS JOIN LATERAL (
		      SELECT (h.help_answered_at IS NOT NULL OR EXISTS (
		        SELECT 1 FROM interactions x
		         WHERE x.conversation_id = h.conversation_id
		           AND x.source = 'telegram'
		           AND x.created_at > h.created_at)) AS ok
		    ) a
		   WHERE h.help_sent AND h.created_at >= (SELECT from_day FROM w) GROUP BY 1
		),
		-- Muammolar: ochilgani.
		iop AS (
		  SELECT date_trunc('day', created_at) AS day,
		         COUNT(*)                                  AS opened,
		         COUNT(*) FILTER (WHERE tg_message_id <> 0) AS notified
		    FROM order_issues, w WHERE created_at >= w.from_day GROUP BY 1
		),
		-- Muammolar: yopilgani va o'rtacha hal qilish vaqti.
		ire AS (
		  SELECT date_trunc('day', resolved_at) AS day,
		         COUNT(*)                                       AS resolved,
		         COUNT(*) FILTER (WHERE resolved_via = 'telegram') AS telegram,
		         COALESCE(AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 3600), 0) AS avg_hours
		    FROM order_issues, w WHERE resolved_at >= w.from_day GROUP BY 1
		),
		-- Eslatmalar. Taxminiy: last_notified_at ustiga yoziladi,
		-- bazada faqat oxirgi eslatma qoladi (DailyReport izohiga qarang).
		irm AS (
		  SELECT date_trunc('day', last_notified_at) AS day, COUNT(*) AS reminded
		    FROM order_issues, w WHERE last_notified_at >= w.from_day GROUP BY 1
		)
		SELECT d.day,
		       COALESCE(ix.total, 0)         AS total,
		       COALESCE(snt.sent, 0)         AS sent,
		       COALESCE(snt.approved, 0)     AS approved,
		       COALESCE(rej.rejected, 0)     AS rejected,
		       COALESCE(ix.staff_replies, 0) AS staff_replies,
		       COALESCE(ix.tokens, 0)        AS tokens,
		       COALESCE(ix.cost, 0)          AS cost,
		       COALESCE(hlp.help_sent, 0)                              AS help_sent,
		       COALESCE(hlp.help_answered, 0)                          AS help_answered,
		       COALESCE(hlp.help_sent, 0) - COALESCE(hlp.help_answered, 0) AS help_unanswered,
		       COALESCE(iop.opened, 0)   AS issues_opened,
		       COALESCE(ire.resolved, 0) AS issues_resolved,
		       COALESCE(ire.avg_hours, 0) AS issues_avg_hours,
		       COALESCE(iop.notified, 0) AS issues_notified,
		       COALESCE(ire.telegram, 0) AS issues_telegram,
		       COALESCE(irm.reminded, 0) AS issues_reminded,
		       -- Kun OXIRIDA ochiq qolgan muammolar: o'sha paytgacha
		       -- ochilgan va hali yopilmaganlari. "Hozir ochiq" kartasi
		       -- shu ustunning oxirgi qatoriga to'g'ri keladi.
		       (SELECT COUNT(*) FROM order_issues o
		         WHERE o.created_at < d.day + interval '1 day'
		           AND (o.resolved_at IS NULL OR o.resolved_at >= d.day + interval '1 day')
		       ) AS issues_open_end
		  FROM d
		  LEFT JOIN ix  ON ix.day  = d.day
		  LEFT JOIN snt ON snt.day = d.day
		  LEFT JOIN rej ON rej.day = d.day
		  LEFT JOIN hlp ON hlp.day = d.day
		  LEFT JOIN iop ON iop.day = d.day
		  LEFT JOIN ire ON ire.day = d.day
		  LEFT JOIN irm ON irm.day = d.day
		 ORDER BY d.day DESC`, days-1).Scan(&out).Error
	})
	return out, err
}
