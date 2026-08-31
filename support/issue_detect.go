// Muammoli buyurtmalarni aniqlash, guruhga xabar berish va hal bo'lishini
// kuzatish.
package support

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// oyNomlari - sanani odam o'qiydigan ko'rinishda yozish uchun.
var oyNomlari = [...]string{"yanvar", "fevral", "mart", "aprel", "may", "iyun",
	"iyul", "avgust", "sentabr", "oktabr", "noyabr", "dekabr"}

// paidAtOr - to'lov vaqti (bo'sh bo'lsa buyurtma yaratilgan vaqt).
func paidAtOr(o AdminkaOrder) string {
	if strings.TrimSpace(o.PaidAt) != "" {
		return o.PaidAt
	}
	return o.CreatedAt
}

// sanaMatn - "2026-08-21 10:00:00" → "21-avgust".
func sanaMatn(s string) string {
	t, ok := parseAdminkaTime(s)
	if !ok {
		return s
	}
	return fmt.Sprintf("%d-%s", t.Day(), oyNomlari[int(t.Month())-1])
}

// DetectIssues buyurtmalarni ko'rib chiqadi:
//   - yangi muammolarni ochadi va guruhga xabar beradi,
//   - muammosi yo'qolganlarini avtomatik yopadi.
//
// Qaytadigan qiymat — modelga beriladigan boyitilgan ro'yxat.
func DetectIssues(orders []AdminkaOrder, clientID, conversationID int64) []OrderView {
	views := make([]OrderView, 0, len(orders))

	for _, o := range orders {
		v := NewOrderView(o)
		if DB == nil || o.OrderSN == "" {
			views = append(views, v)
			continue
		}

		open := FindOpenIssue(DB, o.OrderSN)

		switch {
		case v.Problem && open == nil:
			// Yangi muammo.
			is := &OrderIssue{
				OrderSN:        o.OrderSN,
				ClientID:       clientID,
				ConversationID: conversationID,
				Status:         o.Status,
				StatusLabel:    v.StatusLabel,
				DaysSincePaid:  v.DaysSincePaid,
				PackageName:    o.PackageName,
				PaidAt:         paidAtOr(o),
				State:          IssueOpen,
			}
			if err := DB.Create(is).Error; err != nil {
				log.Printf("muammo: %s yozilmadi: %v", o.OrderSN, err)
			} else {
				notifyIssue(is, issueText(is))
				v.InReview = true
			}

		case v.Problem && open != nil:
			// Muammo davom etmoqda — kun sonini yangilab qo'yamiz.
			DB.Model(open).Updates(map[string]any{
				"days_since_paid": v.DaysSincePaid,
				"status":          o.Status,
				"status_label":    v.StatusLabel,
			})
			v.InReview = true

		case !v.Problem && open != nil:
			// Status o'zgardi — muammo o'z-o'zidan hal bo'ldi.
			res := fmt.Sprintf("Adminkada holat o'zgardi: %q → %q",
				open.StatusLabel, v.StatusLabel)
			if err := ResolveIssue(DB, open, res, "tizim", ResolvedViaAuto); err == nil {
				log.Printf("muammo: %s avtomatik yopildi (%s)", o.OrderSN, v.StatusLabel)
				notifyResolved(open, res)
			}
		}

		views = append(views, v)
	}
	return views
}

// issueText - guruhga ketadigan birinchi xabar.
func issueText(is *OrderIssue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ Muammoli buyurtma — %s\n", is.OrderSN)
	fmt.Fprintf(&b, "Holat: %s\n", is.StatusLabel)
	fmt.Fprintf(&b, "To'langan: %s — %d kundan beri kutmoqda\n",
		sanaMatn(is.PaidAt), is.DaysSincePaid)
	fmt.Fprintf(&b, "Mijoz: %d · suhbat #%d\n", is.ClientID, is.ConversationID)
	if is.PackageName != "" {
		fmt.Fprintf(&b, "Posilka: %s\n", trimText(is.PackageName, 80))
	}
	b.WriteString("\nHal bo'lgach shu xabarga REPLY qilib yozing — " +
		"javobingiz yechim sifatida saqlanadi.")
	return b.String()
}

// remindText - takroriy eslatma matni.
func remindText(is *OrderIssue, days int, answered bool, lastAgentAt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔁 Hali hal bo'lmagan — %s (%d-eslatma)\n", is.OrderSN, is.NotifyCount+1)
	fmt.Fprintf(&b, "Holat: %s · to'langaniga %d kun\n", is.StatusLabel, days)
	if answered {
		fmt.Fprintf(&b, "Mijozga javob: berilgan (%s)\n", lastAgentAt)
	} else {
		b.WriteString("Mijozga javob: BERILMAGAN\n")
	}
	fmt.Fprintf(&b, "Mijoz: %d · suhbat #%d\n", is.ClientID, is.ConversationID)
	b.WriteString("\nHal bo'lgach shu xabarga REPLY qiling.")
	return b.String()
}

// notifyIssue guruhga xabar yuboradi va message_id ni saqlaydi (reply shu
// xabarga qilinadi). Telegram ishlamasa muammo baribir bazada qoladi.
func notifyIssue(is *OrderIssue, text string) {
	msgID, err := SendTelegramMessage(text, 0)
	now := time.Now()
	if err != nil {
		log.Printf("muammo: %s guruhga yuborilmadi: %v", is.OrderSN, err)
		return
	}
	is.TgMessageID = msgID
	is.NotifyCount++
	is.LastNotifiedAt = &now
	DB.Model(is).Updates(map[string]any{
		"tg_message_id": msgID, "notify_count": is.NotifyCount, "last_notified_at": &now,
	})
}

// notifyResolved - avtomatik yopilgani haqida guruhga qisqa xabar
// (asl xabarga reply bo'lib chiqadi).
func notifyResolved(is *OrderIssue, res string) {
	if is.TgMessageID == 0 {
		return
	}
	if _, err := SendTelegramMessage(
		fmt.Sprintf("✅ %s — %s", is.OrderSN, res), is.TgMessageID); err != nil {
		log.Printf("muammo: yopilgani haqida xabar ketmadi: %v", err)
	}
}

// ReviewOpenIssues ochiq muammolarni qayta ko'rib chiqadi:
//  1. adminkadagi holat o'zgarganmi — o'zgargan bo'lsa yopadi;
//  2. mijozga biz javob berganmizmi — chatdan tekshiradi;
//  3. shundan keyingina va ISSUE_REMIND_HOURS o'tgan bo'lsa eslatma yuboradi.
func ReviewOpenIssues(db *gorm.DB) error {
	var open []OrderIssue
	if err := db.Where("state = ?", IssueOpen).Order("id asc").Find(&open).Error; err != nil {
		return err
	}
	if len(open) == 0 {
		return nil
	}

	adm := AdminkaFromEnv()
	remind := time.Duration(RemindHours()) * time.Hour

	for i := range open {
		is := &open[i]

		// 1. Adminkadagi hozirgi holat.
		rows, err := FetchOrders(adm, OrderFilter{OrderSN: is.OrderSN, Size: 5})
		if err != nil {
			log.Printf("muammo: %s adminkadan olinmadi: %v", is.OrderSN, err)
			continue
		}
		var cur *AdminkaOrder
		for j := range rows {
			if rows[j].OrderSN == is.OrderSN {
				cur = &rows[j]
				break
			}
		}
		if cur == nil {
			continue
		}
		if !IsProblem(*cur) {
			res := fmt.Sprintf("Adminkada holat o'zgardi: %q → %q",
				is.StatusLabel, StatusLabel(cur.Status))
			if err := ResolveIssue(db, is, res, "tizim", ResolvedViaAuto); err == nil {
				log.Printf("muammo: %s avtomatik yopildi", is.OrderSN)
				notifyResolved(is, res)
			}
			continue
		}

		// 2. Eslatma vaqti kelganmi.
		if is.LastNotifiedAt != nil && time.Since(*is.LastNotifiedAt) < remind {
			continue
		}

		// 3. Mijozga javob berilganmi (chatdan tekshiramiz).
		answered, lastAt := staffAnswered(is.ConversationID)

		text := remindText(is, DaysSincePaid(*cur), answered, lastAt)
		msgID, err := SendTelegramMessage(text, 0)
		if err != nil {
			log.Printf("muammo: %s eslatmasi ketmadi: %v", is.OrderSN, err)
			continue
		}
		now := time.Now()
		db.Model(is).Updates(map[string]any{
			"tg_message_id":    msgID, // reply endi shu yangi xabarga
			"notify_count":     is.NotifyCount + 1,
			"last_notified_at": &now,
			"days_since_paid":  DaysSincePaid(*cur),
		})
	}
	return nil
}

// staffAnswered - suhbatda oxirgi so'z biz tomondanmi (ya'ni mijozga javob
// berilganmi) va qachon.
func staffAnswered(conversationID int64) (bool, string) {
	if conversationID <= 0 {
		return false, ""
	}
	msgs, err := fetchHistory(conversationID)
	if err != nil || len(msgs) == 0 {
		return false, ""
	}
	last := msgs[len(msgs)-1]
	if last.FromClient() {
		return false, ""
	}
	if t, err := time.Parse(time.RFC3339, last.CreatedAt); err == nil {
		return true, t.Local().Format("02.01 15:04")
	}
	return true, last.CreatedAt
}

// trimText - uzun matnni qisqartiradi.
func trimText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
