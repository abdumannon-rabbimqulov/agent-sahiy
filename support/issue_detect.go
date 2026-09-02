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
	// Yangi ochilgan muammolar shu yerda to'planadi: bitta mijozning
	// hamma muammosi guruhga BITTA xabar bo'lib ketadi.
	var fresh []*OrderIssue

	for _, o := range orders {
		v := NewOrderView(o)
		if DB == nil || o.OrderSN == "" {
			views = append(views, v)
			continue
		}

		open := FindOpenIssue(DB, o.OrderSN)

		switch {
		case v.Problem && open == nil:
			// Yopilgan muammoni takror ko'tarmaymiz: buyurtma o'sha
			// holatda qolgan bo'lsa xodimlar allaqachon ko'rgan.
			// Faqat adminkadagi holat o'zgargan bo'lsa qayta ochamiz.
			if last := LastIssue(DB, o.OrderSN); last != nil &&
				last.State == IssueResolved && last.Status == o.Status {
				views = append(views, v)
				continue
			}

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
				fresh = append(fresh, is)
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

	// Bir mijozning bir necha muammosi — bitta xabar. Xodim guruhda
	// bitta odam haqida beshta alohida xabarni emas, bitta ro'yxatni
	// ko'radi va bitta reply bilan hammasini yopadi.
	notifyIssues(fresh)
	return views
}

// issuesText - guruhga ketadigan birinchi xabar: bitta mijozning barcha
// yangi muammolari bitta matnda.
func issuesText(list []*OrderIssue) string {
	if len(list) == 0 {
		return ""
	}
	first := list[0]

	var b strings.Builder
	if len(list) == 1 {
		fmt.Fprintf(&b, "⚠️ Muammoli buyurtma — %s\n", first.OrderSN)
	} else {
		fmt.Fprintf(&b, "⚠️ Muammoli buyurtmalar — %d ta\n", len(list))
	}
	fmt.Fprintf(&b, "Mijoz: %d · suhbat #%d\n", first.ClientID, first.ConversationID)

	for i, is := range list {
		b.WriteString("\n")
		if len(list) > 1 {
			fmt.Fprintf(&b, "%d) %s\n", i+1, is.OrderSN)
		}
		fmt.Fprintf(&b, "Holat: %s\n", is.StatusLabel)
		fmt.Fprintf(&b, "To'langan: %s — %d kundan beri kutmoqda\n",
			sanaMatn(is.PaidAt), is.DaysSincePaid)
		if is.PackageName != "" {
			fmt.Fprintf(&b, "Posilka: %s\n", trimText(is.PackageName, 80))
		}
	}

	b.WriteString("\nHal bo'lgach shu xabarga REPLY qilib yozing — " +
		"javobingiz yechim sifatida saqlanadi")
	if len(list) > 1 {
		b.WriteString(" (reply yuqoridagi buyurtmalarning hammasini yopadi)")
	}
	b.WriteString(".")
	return b.String()
}

// remindItem - eslatmaga tushadigan bitta muammo va uning yangi holati.
type remindItem struct {
	Issue    *OrderIssue
	Days     int
	Answered bool
	LastAt   time.Time
}

// remindText - takroriy eslatma matni: bitta mijozning eslatma vaqti
// kelgan hamma muammosi bitta xabarda.
func remindText(items []remindItem) string {
	if len(items) == 0 {
		return ""
	}
	first := items[0].Issue

	var b strings.Builder
	if len(items) == 1 {
		fmt.Fprintf(&b, "🔁 Hali hal bo'lmagan — %s (%d-eslatma)\n",
			first.OrderSN, first.NotifyCount+1)
	} else {
		fmt.Fprintf(&b, "🔁 Hali hal bo'lmagan — %d ta buyurtma\n", len(items))
	}
	fmt.Fprintf(&b, "Mijoz: %d · suhbat #%d\n", first.ClientID, first.ConversationID)
	// Mijozga javob berilgani suhbatga tegishli — hamma buyurtma uchun bir xil.
	if items[0].Answered {
		fmt.Fprintf(&b, "Mijozga javob: berilgan (%s)\n", vaqtMatn(items[0].LastAt))
	} else {
		b.WriteString("Mijozga javob: BERILMAGAN\n")
	}

	for i, it := range items {
		b.WriteString("\n")
		if len(items) > 1 {
			fmt.Fprintf(&b, "%d) %s (%d-eslatma)\n", i+1, it.Issue.OrderSN, it.Issue.NotifyCount+1)
		}
		fmt.Fprintf(&b, "Holat: %s · to'langaniga %d kun\n", it.Issue.StatusLabel, it.Days)
	}

	b.WriteString("\nHal bo'lgach shu xabarga REPLY qiling")
	if len(items) > 1 {
		b.WriteString(" (reply yuqoridagi buyurtmalarning hammasini yopadi)")
	}
	b.WriteString(".")
	return b.String()
}

// notifyIssues guruhga BITTA xabar yuboradi va uning message_id sini
// ro'yxatdagi hamma muammoga yozib qo'yadi — reply shu xabarga qilinadi
// va hammasini birdan yopadi. Telegram ishlamasa muammolar baribir
// bazada qoladi (eslatma aylanishida qayta uriniladi).
func notifyIssues(list []*OrderIssue) {
	if len(list) == 0 {
		return
	}
	msgID, err := SendTelegramMessage(issuesText(list), 0)
	if err != nil {
		log.Printf("muammo: %s guruhga yuborilmadi: %v", issueSNs(list), err)
		return
	}
	now := time.Now()
	for _, is := range list {
		is.TgMessageID = msgID
		is.NotifyCount++
		is.LastNotifiedAt = &now
		DB.Model(is).Updates(map[string]any{
			"tg_message_id": msgID, "notify_count": is.NotifyCount, "last_notified_at": &now,
		})
	}
}

// issueSNs - log uchun buyurtma raqamlari.
func issueSNs(list []*OrderIssue) string {
	sns := make([]string, 0, len(list))
	for _, is := range list {
		sns = append(sns, is.OrderSN)
	}
	return strings.Join(sns, ", ")
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

	// Eslatmalar ham mijoz bo'yicha to'planadi: bitta odam uchun bitta
	// xabar ketadi, har bir buyurtma uchun alohida emas.
	due := map[int64][]remindItem{}
	var order []int64

	for i := range open {
		is := &open[i]

		// 1. Xodim mijozga chatda javob berganmi? Bergan bo'lsa muammo
		//    hal qilingan hisoblanadi. Bu tekshiruv birinchi turadi:
		//    adminka javob bermayotgan bo'lsa ham muammo yopilaveradi.
		answered, lastAt := staffAnswered(is.ConversationID)
		if answered && (lastAt.IsZero() || lastAt.After(is.CreatedAt)) {
			res := fmt.Sprintf("Xodim mijozga chatda javob berdi (%s)", vaqtMatn(lastAt))
			if err := ResolveIssue(db, is, res, "xodim", ResolvedViaChat); err == nil {
				log.Printf("muammo: %s chatdagi javob bilan yopildi", is.OrderSN)
				notifyResolved(is, res)
			}
			continue
		}

		// 2. Adminkadagi hozirgi holat.
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

		// 3. Eslatma vaqti kelganmi.
		if is.LastNotifiedAt != nil && time.Since(*is.LastNotifiedAt) < remind {
			continue
		}

		if _, ok := due[is.ClientID]; !ok {
			order = append(order, is.ClientID)
		}
		due[is.ClientID] = append(due[is.ClientID], remindItem{
			Issue:    is,
			Days:     DaysSincePaid(*cur),
			Answered: answered,
			LastAt:   lastAt,
		})
	}

	for _, clientID := range order {
		sendRemind(db, due[clientID])
	}
	return nil
}

// sendRemind - bitta mijozning eslatmalarini bitta xabar qilib yuboradi
// va yangi message_id ni hamma muammoga yozadi (reply endi shu xabarga).
func sendRemind(db *gorm.DB, items []remindItem) {
	if len(items) == 0 {
		return
	}
	msgID, err := SendTelegramMessage(remindText(items), 0)
	if err != nil {
		log.Printf("muammo: mijoz %d eslatmasi ketmadi: %v", items[0].Issue.ClientID, err)
		return
	}
	now := time.Now()
	for _, it := range items {
		db.Model(it.Issue).Updates(map[string]any{
			"tg_message_id":    msgID, // reply endi shu yangi xabarga
			"notify_count":     it.Issue.NotifyCount + 1,
			"last_notified_at": &now,
			"days_since_paid":  it.Days,
		})
	}
}

// staffAnswered - mijozga XODIM javob berganmi va qachon.
func staffAnswered(conversationID int64) (bool, time.Time) {
	if conversationID <= 0 {
		return false, time.Time{}
	}
	msgs, err := fetchHistory(conversationID)
	if err != nil {
		return false, time.Time{}
	}
	return staffAnsweredIn(msgs)
}

// staffAnsweredIn - oxirgi so'z XODIMdan kelganmi.
//
// AI agentning o'z javobi hisobga olinmaydi: u ham "agent" turida va
// AGENT_SENDER_ID bilan yoziladi. Agent "tekshirilmoqda" deb javob
// bergani muammo hal bo'ldi degani emas.
func staffAnsweredIn(msgs []Message) (bool, time.Time) {
	if len(msgs) == 0 {
		return false, time.Time{}
	}
	last := msgs[len(msgs)-1]
	if last.FromClient() {
		return false, time.Time{}
	}
	if agentID := AgentSenderID(); agentID > 0 && last.SenderID == agentID {
		return false, time.Time{} // bu bizning AI javobimiz
	}
	t, _ := parseAnyTime(last.CreatedAt)
	return true, t
}

// vaqtMatn - "30.08 18:42" ko'rinishi (vaqt bo'sh bo'lsa "—").
func vaqtMatn(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("02.01 15:04")
}

// trimText - uzun matnni qisqartiradi.
func trimText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
