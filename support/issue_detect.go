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
//   - yangi muammolarni ochadi,
//   - muammosi yo'qolganlarini avtomatik yopadi.
//
// Guruhga XABAR YUBORMAYDI: yangi ochilgan muammolar ikkinchi qiymat
// bo'lib qaytadi, chaqiruvchi esa ularni AI xulosasi bilan BITTA xabar
// qilib yuboradi (agent.go: DeliverStaffNotice). Ilgari xabar shu yerdan
// darhol ketardi va bitta muammo guruhga IKKI marta — avval
// "⚠️ Muammoli buyurtma", so'ng "🆘 Yordam kerak" bo'lib, ikki xil
// ko'rinishda — tushardi.
//
// Birinchi qaytadigan qiymat — modelga beriladigan boyitilgan ro'yxat.
func DetectIssues(orders []AdminkaOrder, clientID, conversationID int64) ([]OrderView, []*OrderIssue) {
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
				OwnerUserID:    o.UserID,
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
			upd := map[string]any{
				"days_since_paid": v.DaysSincePaid,
				"status":          o.Status,
				"status_label":    v.StatusLabel,
			}
			if o.UserID > 0 && open.OwnerUserID != o.UserID {
				upd["owner_user_id"] = o.UserID
				open.OwnerUserID = o.UserID
			}
			DB.Model(open).Updates(upd)
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

	return views, fresh
}

// NotifyIssues yangi ochilgan muammolarni guruhga chiqaradi.
//
// Bir mijozning bir necha muammosi — bitta xabar. Xodim guruhda bitta
// odam haqida beshta alohida xabarni emas, bitta ro'yxatni ko'radi va
// bitta reply bilan hammasini yopadi. Lekin buyurtmalar har doim ham
// bitta odamniki emas (raqam bo'yicha qidiruv butun adminkadan qidiradi)
// — shuning uchun EGASI bo'yicha ajratiladi.
//
// `help` — AI ning shu suhbat bo'yicha xulosasi. Bo'sh bo'lmasa, alohida
// "🆘 Yordam kerak" xabari sifatida emas, BIRINCHI xabarning ichiga
// qo'shib yuboriladi: bitta muammo — bitta xabar.
//
// Qaytadigan qiymat — birinchi xabarning id'si (AI xulosasi shunga
// ilingan; reply ham shunga tushadi). Hech narsa ketmasa 0.
func NotifyIssues(list []*OrderIssue, help string) int64 {
	var firstID int64
	for i, grp := range groupByOwner(list) {
		// Xulosa faqat birinchi xabarga qo'shiladi — u suhbatga tegishli,
		// har bir buyurtma egasiga alohida takrorlanishi shart emas.
		text := ""
		if i == 0 {
			text = help
		}
		msgID, err := notifyIssues(grp, text)
		if err != nil {
			continue
		}
		if i == 0 {
			firstID = msgID
		}
	}
	return firstID
}

// issueOwner - muammo kimniki: adminkadagi egasi, u noma'lum bo'lsa
// so'ragan mijozning o'zi.
func issueOwner(is *OrderIssue) int64 {
	if is.OwnerUserID > 0 {
		return is.OwnerUserID
	}
	return is.ClientID
}

// groupByOwner - muammolarni egasi bo'yicha guruhlaydi (tartibi saqlanadi).
// Bitta xabarda faqat BITTA odamning buyurtmalari bo'ladi: "Mijoz: X"
// sarlavhasi hamma qatorga to'g'ri kelsin.
func groupByOwner(list []*OrderIssue) [][]*OrderIssue {
	idx := map[int64]int{}
	var out [][]*OrderIssue
	for _, is := range list {
		own := issueOwner(is)
		if i, ok := idx[own]; ok {
			out[i] = append(out[i], is)
			continue
		}
		idx[own] = len(out)
		out = append(out, []*OrderIssue{is})
	}
	return out
}

// issuesText - guruhga ketadigan birinchi xabar: bitta mijozning barcha
// yangi muammolari bitta matnda. `help` bo'sh bo'lmasa, AI ning shu
// suhbat bo'yicha xulosasi ham shu xabarga qo'shiladi.
func issuesText(list []*OrderIssue, help string) string {
	if len(list) == 0 {
		return ""
	}
	first := list[0]

	var b strings.Builder
	title := fmt.Sprintf("⚠️ Muammoli buyurtma — %s", first.OrderSN)
	if len(list) > 1 {
		title = fmt.Sprintf("⚠️ Muammoli buyurtmalar — %d ta", len(list))
	}
	b.WriteString(guruhSarlavha(title, issueOwner(first), first.ClientID, first.ConversationID))

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

	b.WriteString(aiXulosa(help))
	b.WriteString(guruhFooter(len(list) > 1))
	return b.String()
}

// aiXulosa - AI ning muammo haqidagi qisqa xulosasi (guruh xabarining
// ichida, alohida xabar emas). Bo'sh bo'lsa hech narsa qo'shilmaydi.
func aiXulosa(help string) string {
	help = strings.TrimSpace(help)
	if help == "" {
		return ""
	}
	return "\nAI xulosasi:\n" + help + "\n"
}

// remindKey - eslatma guruhining kaliti: buyurtma egasi + so'ragan mijoz.
type remindKey struct {
	Owner  int64
	Client int64
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
	title := fmt.Sprintf("🔁 Hali hal bo'lmagan — %s (%d-eslatma)",
		first.OrderSN, first.NotifyCount+1)
	if len(items) > 1 {
		title = fmt.Sprintf("🔁 Hali hal bo'lmagan — %d ta buyurtma", len(items))
	}
	b.WriteString(guruhSarlavha(title, issueOwner(first), first.ClientID, first.ConversationID))
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

	b.WriteString(guruhFooter(len(items) > 1))
	return b.String()
}

// notifyIssues guruhga BITTA xabar yuboradi va uning message_id sini
// ro'yxatdagi hamma muammoga yozib qo'yadi — reply shu xabarga qilinadi
// va hammasini birdan yopadi. Telegram ishlamasa muammolar baribir
// bazada qoladi (eslatma aylanishida qayta uriniladi).
func notifyIssues(list []*OrderIssue, help string) (int64, error) {
	if len(list) == 0 {
		return 0, nil
	}
	msgID, err := SendTelegramMessage(issuesText(list, help), 0)
	if err != nil {
		log.Printf("muammo: %s guruhga yuborilmadi: %v", issueSNs(list), err)
		return 0, err
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
	return msgID, nil
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
	// xabar ketadi, har bir buyurtma uchun alohida emas. Kalit — buyurtma
	// EGASI va uni so'ragan mijoz birgalikda: bitta xabardagi hamma
	// buyurtma bir odamniki bo'lsin va "mijozga javob berilgan/berilmagan"
	// satri ham o'sha suhbatga to'g'ri kelsin.
	due := map[remindKey][]remindItem{}
	var order []remindKey

	for i := range open {
		is := &open[i]

		// 0. Begona buyurtma: muammo suhbatdagi mijozga emas, boshqa
		//    odamga tegishli. Bunday muammo ochilmasligi kerak edi
		//    (agent.go: onlyOwnOrders) — eski yozuvlar shu yerda
		//    yopiladi, aks holda guruhga hech kim so'ramagan buyurtma
		//    bo'yicha eslatma yog'ilaveradi.
		if is.ClientID > 0 && is.OwnerUserID > 0 && is.OwnerUserID != is.ClientID {
			res := fmt.Sprintf("Begona buyurtma: egasi %d, suhbatdagi mijoz %d — "+
				"mijoz bu buyurtma haqida so'ramagan", is.OwnerUserID, is.ClientID)
			if err := ResolveIssue(db, is, res, "tizim", ResolvedViaAuto); err == nil {
				log.Printf("muammo: %s begona buyurtma sifatida yopildi (egasi %d, mijoz %d)",
					is.OrderSN, is.OwnerUserID, is.ClientID)
				notifyResolved(is, res)
			}
			continue
		}

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
		if cur.UserID > 0 && is.OwnerUserID != cur.UserID {
			db.Model(is).Update("owner_user_id", cur.UserID)
			is.OwnerUserID = cur.UserID
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

		key := remindKey{Owner: issueOwner(is), Client: is.ClientID}
		if _, ok := due[key]; !ok {
			order = append(order, key)
		}
		due[key] = append(due[key], remindItem{
			Issue:    is,
			Days:     DaysSincePaid(*cur),
			Answered: answered,
			LastAt:   lastAt,
		})
	}

	for _, key := range order {
		sendRemind(db, due[key])
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
