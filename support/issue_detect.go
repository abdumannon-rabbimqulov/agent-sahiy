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
			last := LastIssue(DB, o.OrderSN)

			// Yopilgan muammoni takror ko'tarmaymiz: buyurtma o'sha
			// holatda qolgan bo'lsa xodimlar allaqachon ko'rgan.
			if last != nil && last.State == IssueResolved && last.Status == o.Status {
				views = append(views, v)
				continue
			}

			// Shu buyurtma bo'yicha xodim ALLAQACHON javob berganmi.
			//
			// Adminkadagi holat o'zgarishi (3 → 4: "to'langan" →
			// "kiritish uchun kutilmoqda") Xitoy tomonidagi oddiy
			// bosqich — mijoz uchun YANGI muammo emas. Yuqoridagi
			// tekshiruv faqat holat AYNAN bir xil qolganda ushlab
			// qoladi, shuning uchun bitta buyurtma guruhga ikkinchi
			// marta tushadi va xodim o'zi aytgan gapni qaytadan o'qiydi.
			//
			// Xabar yashirilmaydi — xodim muammo qaytganini ko'rsin —
			// lekin ustiga darhol "TEPADA JAVOB BERILGAN" degan reply
			// tushadi va yozuv yopiladi (noteAlreadyAnswered).
			answered := last
			if !AnsweredByStaff(answered) {
				answered = nil
			}

			// Yangi muammo.
			is := &OrderIssue{
				OrderSN:        o.OrderSN,
				ClientID:       clientID,
				OwnerUserID:    o.UserID,
				ExpressNum:     strings.TrimSpace(o.ExpressNum),
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
				// Takror bo'lsa xabar guruhga BARIBIR chiqadi (xodim
				// muammo qaytganini ko'rsin), lekin darhol o'sha
				// xabarga "tepada javob berilgan" degan reply tushadi
				// va yozuv yopiladi — eslatma aylanishiga tushmaydi.
				if answered != nil {
					is.Repeat = repeatResolution(answered)
					is.ResolvedBy = answered.ResolvedBy
				}
				fresh = append(fresh, is)
				v.InReview = true
			}

		case v.Problem && open != nil && StatusAdvanced(open.Status, o.Status):
			// Holat 3 dan 4 ga o'tgan — buyurtma Xitoyda oldinga
			// siljidi, muammo hal bo'ldi.
			resolveAdvanced(DB, open, o.Status)

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

		case !v.Problem && open != nil && StatusAdvanced(open.Status, o.Status):
			// Buyurtma oldinga siljidi (3 → 4, 4 → 7). Yo'lga
			// chiqqan bo'lsa mijozga ham xabar beriladi.
			resolveAdvanced(DB, open, o.Status)

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

// currentOrder - buyurtmaning adminkadagi HOZIRGI yozuvi.
// Topilmasa yoki so'rov xato bersa nil.
func currentOrder(adm Adminka, orderSN string) *AdminkaOrder {
	rows, err := FetchOrders(adm, OrderFilter{OrderSN: orderSN, Size: 5})
	if err != nil {
		log.Printf("muammo: %s adminkadan olinmadi: %v", orderSN, err)
		return nil
	}
	for i := range rows {
		if rows[i].OrderSN == orderSN {
			return &rows[i]
		}
	}
	return nil
}

// resolveAdvanced - adminkadagi holat oldinga siljigani uchun muammoni
// yopadi (3 → 4, qarang: StatusAdvanced).
//
// Yangi holat yozuvga ham ko'chiriladi. Busiz keyingi tekshiruvda
// "yopilgan muammo, holat o'sha" himoyasi (DetectIssues) ishlamay
// qolardi: yozuvda 3 turgani uchun xuddi shu buyurtma endi status 4
// bilan QAYTA ochilib, guruhga ikkinchi marta tushardi.
func resolveAdvanced(db *gorm.DB, is *OrderIssue, now int) bool {
	res := fmt.Sprintf("Adminkada holat o'zgardi: %q → %q — buyurtma Xitoyda "+
		"keyingi bosqichga o'tdi, qotib qolmagan",
		StatusLabel(is.Status), StatusLabel(now))

	db.Model(is).Updates(map[string]any{
		"status": now, "status_label": StatusLabel(now),
	})
	is.Status, is.StatusLabel = now, StatusLabel(now)

	if err := ResolveIssue(db, is, res, "tizim", ResolvedViaAuto); err != nil {
		log.Printf("muammo: %s yopilmadi: %v", is.OrderSN, err)
		return false
	}
	log.Printf("muammo: %s — adminkada holat oldinga siljidi, yopildi", is.OrderSN)
	notifyResolved(is, res)
	if now == StatusShipped {
		noticeShipped(is)
	}
	return true
}

// shippedNotice - posilka yo'lga chiqqani haqida mijozga ketadigan
// matn.
//
// Ataylab TAYYOR matn, model yozgani emas: bu oddiy holat xabari va
// unda o'ylab topadigan narsa yo'q. Muddat VA'DA QILINMAYDI — posilka
// qachon yetib kelishini hech kim bilmaydi; "kelgach xabar beramiz"
// ham yozilmaydi, chunki kelganda avtomatik xabar yuborilmaydi va
// bajarilmaydigan va'da bergandan ko'ra aytmagan yaxshi.
const shippedNotice = "Xushxabar: %s raqamli buyurtmangiz Xitoydan yo'lga chiqdi — " +
	"hozir yo'lda. O'zbekistonga yetib kelgach, filialdan olib ketishingiz mumkin bo'ladi."

// noticeShipped - buyurtma yo'lga chiqqani haqida mijozga xabar beradi.
//
// AutoReplyOn() ga bo'ysunadi: avto-javob o'chiq bo'lsa tizim mijozga
// o'zidan xabar yozmaydi — bu sozlamaning butun ma'nosi shunda.
//
// Xabar BIR MARTA ketadi: muammo shu qadamda yopiladi, ya'ni keyingi
// sikllarda bu yozuv umuman ko'rilmaydi.
func noticeShipped(is *OrderIssue) {
	if is.ConversationID <= 0 {
		return
	}
	if !AutoReplyOn() {
		log.Printf("muammo: %s yo'lga chiqdi, lekin avto-javob o'chiq — mijozga yozilmadi",
			is.OrderSN)
		return
	}
	if err := SendToClient(is.ConversationID, fmt.Sprintf(shippedNotice, is.OrderSN)); err != nil {
		log.Printf("muammo: %s — mijozga \"yo'lga chiqdi\" xabari ketmadi: %v",
			is.OrderSN, err)
		return
	}
	log.Printf("muammo: %s — mijozga \"yo'lga chiqdi\" xabari yuborildi (suhbat %d)",
		is.OrderSN, is.ConversationID)
}

// DropResolvedIssues - guruhga YUBORISHDAN OLDIN har bir muammoning
// adminkadagi HOZIRGI holatini qayta so'raydi va hal bo'lganlarini
// ro'yxatdan olib tashlaydi (yozuvini yopib).
//
// Nega kerak: muammo zanjir BOSHIDA, adminkadan o'sha paytda olingan
// ma'lumot bo'yicha ochiladi; guruhga esa zanjir OXIRIDA, AI xulosasi
// bilan birga chiqadi. Oradagi vaqtda holat o'zgargan bo'lishi mumkin
// — xodim allaqachon hal bo'lgan buyurtmani qidirib o'tirmasin.
//
// Adminka javob bermasa muammo ro'yxatda QOLADI: holat noma'lum
// bo'lgani xabarni yashirish uchun asos emas.
func DropResolvedIssues(list []*OrderIssue) []*OrderIssue {
	if len(list) == 0 || DB == nil {
		return list
	}
	adm := AdminkaFromEnv()
	out := make([]*OrderIssue, 0, len(list))
	for _, is := range list {
		cur := currentOrder(adm, is.OrderSN)
		if cur == nil {
			out = append(out, is)
			continue
		}
		switch {
		case StatusAdvanced(is.Status, cur.Status):
			if !resolveAdvanced(DB, is, cur.Status) {
				out = append(out, is)
			}
		case !IsProblem(*cur):
			res := fmt.Sprintf("Adminkada holat o'zgardi: %q → %q",
				is.StatusLabel, StatusLabel(cur.Status))
			if err := ResolveIssue(DB, is, res, "tizim", ResolvedViaAuto); err != nil {
				out = append(out, is)
				continue
			}
			log.Printf("muammo: %s — yuborishdan oldin hal bo'lgan deb yopildi", is.OrderSN)
		default:
			out = append(out, is)
		}
	}
	return out
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

// notifyIssues guruhga BITTA xabar yuboradi va uning message_id sini
// ro'yxatdagi hamma muammoga yozib qo'yadi — reply shu xabarga qilinadi
// va hammasini birdan yopadi. Telegram ishlamasa muammolar baribir
// bazada qoladi (eslatma aylanishida qayta uriniladi).
func notifyIssues(list []*OrderIssue, help string) (int64, error) {
	if len(list) == 0 {
		return 0, nil
	}
	msgID, err := SendTelegramIssue(issuesText(list, help))
	if err != nil {
		log.Printf("muammo: %s guruhga yuborilmadi: %v", issueSNs(list), err)
		return 0, err
	}
	now := time.Now()
	for _, is := range list {
		RememberTelegramPost(msgID, is.ConversationID, is.ClientID, "issue", 0)
		is.TgMessageID = msgID
		is.NotifyCount++
		is.LastNotifiedAt = &now
		DB.Model(is).Updates(map[string]any{
			"tg_message_id": msgID, "notify_count": is.NotifyCount, "last_notified_at": &now,
		})
		noteAlreadyAnswered(is)
	}
	return msgID, nil
}

// repeatResolution - qayta chiqqan muammo uchun yechim matni: javob
// qachon va kim tomonidan berilganiga havola.
func repeatResolution(prev *OrderIssue) string {
	when := ""
	if prev.ResolvedAt != nil {
		when = " (" + vaqtMatn(*prev.ResolvedAt) + ")"
	}
	who := prev.ResolvedBy
	if who == "" {
		who = "xodim"
	}
	txt := fmt.Sprintf("Bu buyurtma bo'yicha javob allaqachon berilgan%s — %s", when, who)
	if r := strings.TrimSpace(prev.Resolution); r != "" {
		txt += ": " + r
	}
	return txt
}

// answeredEarlier - shu buyurtma bo'yicha ilgari xodim javob bergan
// yozuv (bo'lmasa nil). Hozirgi yozuvning o'zi hisobga olinmaydi.
func answeredEarlier(db *gorm.DB, is *OrderIssue) *OrderIssue {
	if is == nil || is.OrderSN == "" {
		return nil
	}
	var prev OrderIssue
	err := db.Where("order_sn = ? AND id <> ? AND state = ? AND resolved_via IN ?",
		is.OrderSN, is.ID, IssueResolved,
		[]string{ResolvedViaTelegram, ResolvedViaChat, ResolvedViaPanel}).
		Order("id desc").First(&prev).Error
	if err != nil {
		return nil
	}
	return &prev
}

// noteAlreadyAnswered - guruhga qayta chiqqan muammoga "tepada javob
// berilgan" deb reply qiladi va yozuvni yopadi.
//
// Xabarning o'zi yashirilmaydi: xodim muammo qaytganini ko'rishi kerak.
// Lekin ustiga darhol belgi tushadi — kim, qachon javob bergani bilan
// — va muammo eslatma aylanishiga tushmaydi.
func noteAlreadyAnswered(is *OrderIssue) {
	if is == nil || is.Repeat == "" {
		return
	}
	text := "✅ " + is.OrderSN + " — TEPADA JAVOB BERILGAN.\n" + is.Repeat +
		"\nQayta javob berish shart emas."
	if _, err := SendTelegramMessage(text, is.TgMessageID); err != nil {
		log.Printf("muammo: %s — \"tepada javob berilgan\" belgisi qo'yilmadi: %v", is.OrderSN, err)
	}
	if err := ResolveIssue(DB, is, is.Repeat, is.ResolvedBy, ResolvedViaRepeat); err != nil {
		log.Printf("muammo: %s takror yozuvi yopilmadi: %v", is.OrderSN, err)
		return
	}
	log.Printf("muammo: %s qayta chiqdi — tepadagi javobga havola qilindi", is.OrderSN)
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

// ReviewOpenIssues ochiq muammolarni qayta ko'rib chiqadi va hal
// bo'lganlarini YOPADI:
//  1. muammo boshqa odamning buyurtmasimi;
//  2. shu buyurtma bo'yicha xodim allaqachon javob berganmi;
//  3. mijozga biz javob berganmizmi — chatdan tekshiradi;
//  4. adminkadagi holat o'zgarganmi;
//  5. posilka yetkazmada (dashboardda) chiqqanmi.
//
// Guruhga YANGI XABAR YUBORMAYDI. Faqat yopilgani haqida asl xabarga
// "✅ …" deb reply qiladi (notifyResolved).
//
// Ilgari bu yerdan takroriy eslatma ("🔁 Hali hal bo'lmagan") ketardi:
// javob kelmagan muammo har ISSUE_REMIND_HOURS da guruhga qayta
// tushardi. Guruhda bitta muammo bo'yicha o'nlab xabar to'planib
// qolardi va yangi, hali ko'rilmagan muammolar ular orasida yo'qolardi.
// Endi guruhga ikki turdagina xabar boradi: "⚠️ Muammoli buyurtma" va
// "🆘 Yordam kerak". Javobsiz qolgan muammolar panelda ko'rinadi
// (ochiq muammolar ro'yxati va "javobsiz" hisobi).
func ReviewOpenIssues(db *gorm.DB) error {
	var open []OrderIssue
	if err := db.Where("state = ?", IssueOpen).Order("id asc").Find(&open).Error; err != nil {
		return err
	}
	if len(open) == 0 {
		return nil
	}

	adm := AdminkaFromEnv()
	// Yetkazma tokeni faqat treki bor buyurtma uchraganda olinadi.
	var dash deliveryAuth

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

		// 0.1. Shu buyurtma bo'yicha oldinroq XODIM javob bergan
		//      bo'lsa (boshqa yozuv), bu takror muammo — guruhni
		//      qayta bezovta qilmaymiz.
		if prev := answeredEarlier(db, is); prev != nil {
			// Guruhdagi OXIRGI xabariga (odatda eslatmaga) "tepada
			// javob berilgan" deb reply tushadi va muammo yopiladi.
			is.Repeat = repeatResolution(prev)
			is.ResolvedBy = prev.ResolvedBy
			noteAlreadyAnswered(is)
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
		cur := currentOrder(adm, is.OrderSN)
		if cur == nil {
			continue
		}
		// Holat 3 dan 4 ga o'tgan — buyurtma oldinga siljidi,
		// muammo hal bo'ldi (StatusAdvanced).
		if StatusAdvanced(is.Status, cur.Status) {
			resolveAdvanced(db, is, cur.Status)
			continue
		}
		if cur.UserID > 0 && is.OwnerUserID != cur.UserID {
			db.Model(is).Update("owner_user_id", cur.UserID)
			is.OwnerUserID = cur.UserID
		}
		// Trek Xitoyda keyinroq beriladi — eski yozuvlarda bo'sh
		// bo'lishi mumkin, shuning uchun har ko'rishda to'ldiriladi.
		if t := strings.TrimSpace(cur.ExpressNum); t != "" && is.ExpressNum != t {
			db.Model(is).Update("express_num", t)
			is.ExpressNum = t
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

		// 3. Yetkazma (dashboard) tomoni: adminkadagi holat nima deb
		//    tursa ham, posilka allaqachon O'zbekistonga kelgan
		//    bo'lishi mumkin — bunda muammo yopiladi
		//    (issue_dashboard.go).
		if trackKey(cur.ExpressNum) != "" {
			svc, token, err := dash.get()
			if err != nil {
				log.Printf("muammo: yetkazma tokeni olinmadi: %v", err)
			} else if chk, err := CheckDashboard(svc, token, *cur); err != nil {
				log.Printf("muammo: %s — yetkazma tomoni tekshirilmadi: %v",
					is.OrderSN, err)
			} else if chk.Arrived() && !chk.Mismatch {
				// Posilka O'zbekistonga kelgan (filialda yoki mijoz
				// olib ketgan) — muammo qolmadi. Adminkadagi holat
				// hamon "kutilmoqda" bo'lishi mumkin, lekin u Xitoy
				// tomonidagi holat va bu yerda hech narsani
				// o'zgartirmaydi.
				res := arrivedResolution(is, chk)
				if err := ResolveIssue(db, is, res, "tizim", ResolvedViaAuto); err == nil {
					log.Printf("muammo: %s — posilka yetkazmada bor, yopildi", is.OrderSN)
					notifyResolved(is, res)
				}
				continue
			}
			// Egasi mos kelmagan buyurtma ATAYLAB yopilmaydi: u
			// ochiq "⚠️ Muammoli buyurtma" bo'lib turaveradi, ya'ni
			// xodim uni panelda ko'radi va mijozga "tekshirilmoqda"
			// deyiladi. Alohida "⛔ XATOLIK" xabari yuborilmaydi —
			// guruhga faqat ikki turdagi xabar boradi.
		}

		// Muammo hali ham ochiq — guruhga qayta xabar yuborilmaydi.
		// Xodim uni panelda, ochiq muammolar ro'yxatida ko'radi.
	}

	return nil
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
