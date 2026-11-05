// Yig'ish rejimi hisoboti: panel uchun so'rovlar.
package support

import (
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func sortChatsOldest(chats []Chat) {
	sort.Slice(chats, func(i, j int) bool { return chats[i].MsCreatedAt < chats[j].MsCreatedAt })
}

// CollectTypeRow - bitta tur va guruh bo'yicha son.
type CollectTypeRow struct {
	Type   string `json:"type"`
	Bucket string `json:"bucket"`
	Orders int64  `json:"orders"`
	Users  int64  `json:"users"`
}

// CollectCount - AI turi yoki holat bo'yicha son.
type CollectCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// CollectSummary - dashboard kartalari.
type CollectSummary struct {
	Chats        int64            `json:"chats"`
	WithNumbers  int64            `json:"with_numbers"`
	NoNumbers    int64            `json:"no_numbers"`
	Partial      int64            `json:"partial"`
	Failed       int64            `json:"failed"`
	Orders       int64            `json:"orders"`
	Users        int64            `json:"users"`
	ProblemOrder int64            `json:"problem_orders"`
	ProblemUsers int64            `json:"problem_users"`
	Unverified   int64            `json:"unverified"`
	Types        []CollectTypeRow `json:"types"`
	AITypes      []CollectAIType  `json:"ai_types"`
	AIBlank      int64            `json:"ai_blank"` // buyurtma bor, AI xulosasi yo'q
	ArrivedAfter int64            `json:"arrived_after"`
}

// GetCollectSummary - jonli hisobot: har chaqiruvda bazadan.
func GetCollectSummary() (CollectSummary, error) {
	var s CollectSummary
	var states []CollectCount
	if err := DB.Model(&CollectChat{}).Select("state AS key, COUNT(*) AS count").Group("state").Scan(&states).Error; err != nil {
		return s, err
	}
	for _, r := range states {
		s.Chats += r.Count
		switch r.Key {
		case CollectStateDone:
			s.WithNumbers += r.Count
		case CollectStateNoNumbers:
			s.NoNumbers += r.Count
		case CollectStatePartial:
			s.Partial += r.Count
		case CollectStateError:
			s.Failed += r.Count
		}
	}
	DB.Model(&CollectOrder{}).Count(&s.Orders)
	DB.Model(&CollectOrder{}).Distinct("user_id").Count(&s.Users)
	DB.Model(&CollectOrder{}).Where("verified = ?", false).Count(&s.Unverified)
	DB.Model(&CollectFinding{}).Distinct("order_sn").Count(&s.ProblemOrder)
	DB.Model(&CollectFinding{}).Distinct("user_id").Count(&s.ProblemUsers)

	if err := DB.Model(&CollectFinding{}).
		Select("type, bucket, COUNT(DISTINCT order_sn) AS orders, COUNT(DISTINCT user_id) AS users").
		Group("type, bucket").Order("type, bucket").Scan(&s.Types).Error; err != nil {
		return s, err
	}
	if err := DB.Raw(`SELECT ai_type AS type, COUNT(*) AS items, COUNT(DISTINCT client_id) AS clients,
		COUNT(*) FILTER (WHERE arrived_at > client_wrote_at) AS arrived_after
		FROM (` + collectClientsSQL + `) t GROUP BY ai_type ORDER BY items DESC`).Scan(&s.AITypes).Error; err != nil {
		return s, err
	}
	for _, r := range s.AITypes {
		s.ArrivedAfter += r.ArrivedAfter
	}
	DB.Model(&CollectOrder{}).Where("ai_type = ''").Count(&s.AIBlank)
	return s, nil
}

// CollectAIType - AI xulosasi turi bo'yicha son.
type CollectAIType struct {
	Type         string `json:"type"`
	Items        int64  `json:"items"`
	Clients      int64  `json:"clients"`
	ArrivedAfter int64  `json:"arrived_after"`
}

// collectClientsSQL - har mijoz muammosi bitta qator: raqamli murojaatda
// buyurtma bo'yicha, raqamsizida murojaatning o'zi bo'yicha.
const collectClientsSQL = `
	SELECT o.client_id, o.user_id, o.conversation_id, o.order_sn, o.status, o.status_label,
		o.ai_type, o.ai_note, c.last_text, c.client_wrote_at, o.arrived_at, o.delivered_at,
		o.arrived, o.received, o.branch
	FROM collect_orders o JOIN collect_chats c ON c.conversation_id = o.conversation_id
	WHERE o.ai_type <> ''
	UNION ALL
	SELECT c.client_id, 0, c.conversation_id, '', 0, '',
		c.ai_type, c.ai_note, c.last_text, c.client_wrote_at, NULL, NULL, false, false, ''
	FROM collect_chats c WHERE c.ai_type <> '' AND c.orders_count = 0`

// CollectClientRow - mijozning muammosi: AI xulosasi va sanalar.
type CollectClientRow struct {
	ClientID       int64      `json:"client_id"`
	UserID         int64      `json:"user_id"`
	ConversationID int64      `json:"conversation_id"`
	OrderSN        string     `json:"order_sn"`
	Status         int        `json:"status"`
	StatusLabel    string     `json:"status_label"`
	AIType         string     `json:"ai_type"`
	AINote         string     `json:"ai_note"`
	LastText       string     `json:"last_text"`
	ClientWroteAt  *time.Time `json:"client_wrote_at"`
	ArrivedAt      *time.Time `json:"arrived_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	Arrived        bool       `json:"arrived"`
	Received       bool       `json:"received"`
	Branch         string     `json:"branch"`
}

// ListCollectClients - AI xulosasi bo'yicha mijozlar. arrivedAfter:
// faqat mijoz yozganidan keyin tovari kelganlar.
func ListCollectClients(aiType string, arrivedAfter bool, page, limit int) ([]CollectClientRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	where, args := "WHERE true", []any{}
	if aiType != "" {
		where += " AND ai_type = ?"
		args = append(args, aiType)
	}
	if arrivedAfter {
		where += " AND arrived_at > client_wrote_at"
	}
	from := "FROM (" + collectClientsSQL + ") t " + where
	var total int64
	if err := DB.Raw("SELECT COUNT(*) "+from, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CollectClientRow
	err := DB.Raw("SELECT * "+from+" ORDER BY client_wrote_at ASC NULLS LAST, conversation_id LIMIT ? OFFSET ?",
		append(args, limit, (page-1)*limit)...).Scan(&rows).Error
	return rows, total, err
}

// CollectOrderRow - jadval qatori.
type CollectOrderRow struct {
	CollectOrder
	Type       string     `json:"type"`
	Bucket     string     `json:"bucket"`
	AnchorKind string     `json:"anchor_kind"`
	AnchorAt   *time.Time `json:"anchor_at"`
	Days       *int       `gorm:"-" json:"days"`
	Types      []string   `gorm:"-" json:"types"`
}

// ListCollectOrders - tur va guruh bo'yicha buyurtmalar. Tur berilmasa
// hamma o'rganilgan buyurtma. Eng uzoq kutgani birinchi.
func ListCollectOrders(typ, bucket, aiType string, page, limit int) ([]CollectOrderRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	q := DB.Table("collect_orders AS o")
	if typ != "" {
		q = q.Joins("JOIN collect_findings f ON f.order_sn = o.order_sn AND f.type = ?", typ)
		if bucket != "" {
			q = q.Where("f.bucket = ?", bucket)
		}
	} else {
		q = q.Joins("LEFT JOIN LATERAL (SELECT '' AS type, '' AS bucket, '' AS anchor_kind, NULL::timestamptz AS anchor_at) f ON true")
	}
	if aiType != "" {
		q = q.Where("o.ai_type = ?", aiType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CollectOrderRow
	err := q.Select("o.*, f.type, f.bucket, f.anchor_kind, f.anchor_at").
		Order("f.anchor_at ASC NULLS LAST, o.checked_at DESC").
		Offset((page - 1) * limit).Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	sns := make([]string, 0, len(rows))
	for _, r := range rows {
		sns = append(sns, r.OrderSN)
	}
	var fs []CollectFinding
	DB.Where("order_sn IN ?", sns).Find(&fs)
	byOrder := map[string][]string{}
	for _, f := range fs {
		name := f.Type
		if f.Bucket != "" {
			name += "_" + f.Bucket
		}
		byOrder[f.OrderSN] = append(byOrder[f.OrderSN], name)
	}
	now := time.Now()
	for i := range rows {
		rows[i].Types = byOrder[rows[i].OrderSN]
		at := rows[i].AnchorAt
		if at == nil {
			at = rows[i].OrderCreatedAt
		}
		if at != nil {
			d := daysBetween(*at, now)
			rows[i].Days = &d
		}
	}
	return rows, total, nil
}

// ListCollectChats - suhbatlar, holat bo'yicha filtr bilan.
func ListCollectChats(state string, page, limit int) ([]CollectChat, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	q := DB.Model(&CollectChat{}).Where("NOT dismissed")
	if state != "" {
		q = q.Where("state = ?", state)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CollectChat
	err := q.Order("processed_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error
	return rows, total, err
}

// CollectRecentRow - jonli lenta qatori.
type CollectRecentRow struct {
	CollectChat
	Orders []CollectRecentOrder `json:"orders"`
}

// CollectRecentOrder - lentadagi buyurtma.
type CollectRecentOrder struct {
	OrderSN string   `json:"order_sn"`
	Status  int      `json:"status"`
	AIType  string   `json:"ai_type"`
	AINote  string   `json:"ai_note"`
	Types   []string `json:"types"`
}

// RecentCollect - oxirgi o'rganilgan suhbatlar, yangisi tepada.
func RecentCollect(limit int) ([]CollectRecentRow, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	var chats []CollectChat
	if err := DB.Order("processed_at DESC").Limit(limit).Find(&chats).Error; err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(chats))
	for _, c := range chats {
		ids = append(ids, c.ConversationID)
	}
	var orders []CollectOrder
	DB.Where("conversation_id IN ?", ids).Find(&orders)
	sns := make([]string, 0, len(orders))
	for _, o := range orders {
		sns = append(sns, o.OrderSN)
	}
	var fs []CollectFinding
	DB.Where("order_sn IN ?", sns).Find(&fs)
	types := map[string][]string{}
	for _, f := range fs {
		name := f.Type
		if f.Bucket != "" {
			name += "_" + f.Bucket
		}
		types[f.OrderSN] = append(types[f.OrderSN], name)
	}
	byChat := map[int64][]CollectRecentOrder{}
	for _, o := range orders {
		byChat[o.ConversationID] = append(byChat[o.ConversationID], CollectRecentOrder{
			OrderSN: o.OrderSN, Status: o.Status, AIType: o.AIType, AINote: o.AINote, Types: types[o.OrderSN],
		})
	}
	out := make([]CollectRecentRow, 0, len(chats))
	for _, c := range chats {
		out = append(out, CollectRecentRow{CollectChat: c, Orders: byChat[c.ConversationID]})
	}
	return out, nil
}

// DismissCollectOrder - buyurtmani hisobotdan butunlay o'chiradi: uning hamma
// muammo turlari va AI xulosasi. Keyingi tekshiruvlarda qaytmaydi.
func DismissCollectOrder(orderSN string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		rec := CollectDismissed{OrderSN: orderSN, DismissedAt: time.Now()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error; err != nil {
			return err
		}
		if err := tx.Where("order_sn = ?", orderSN).Delete(&CollectFinding{}).Error; err != nil {
			return err
		}
		return tx.Where("order_sn = ?", orderSN).Delete(&CollectOrder{}).Error
	})
}

// DismissCollectChat - raqamsiz murojaatni hisobotdan o'chiradi. Yozuv
// qoladi: aks holda navbat suhbatni yangi deb qayta ishlaydi.
func DismissCollectChat(conversationID int64) error {
	return DB.Model(&CollectChat{}).Where("conversation_id = ?", conversationID).
		Updates(map[string]any{"dismissed": true, "ai_type": "", "ai_note": "", "attempts": collectMaxAttempts}).Error
}

// RequestCollectRun - paneldan yurish so'rash. max 0 — butun navbat.
func RequestCollectRun(max int) error {
	if err := SetSetting(DB, SettingCollectMax, strconv.Itoa(max)); err != nil {
		return err
	}
	collector.Lock()
	collector.nextRun = time.Time{}
	collector.Unlock()
	return SetSetting(DB, SettingCollectRun, "true")
}

// StopCollect - yurishni to'xtatadi: agent ham o'chadi, aks holda sikl
// keyingi aylanishda yana boshlab yuboradi.
func StopCollect() error {
	if err := SetSetting(DB, SettingCollectRun, "false"); err != nil {
		return err
	}
	return SetSetting(DB, SettingAgentEnabled, "false")
}
