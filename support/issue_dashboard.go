// Ochiq muammoni YETKAZMA (dashboard) tomoni bilan solishtirish.
//
// Adminkadagi "kiritish uchun kutilmoqda" (status 4) — Xitoy tomonidagi
// holat va u YANGILANMAY QOLISHI mumkin: posilka allaqachon
// O'zbekistonga kelib, dashboardda turadi, adminka esa hamon
// "kutilmoqda" deb ko'rsatadi. Bunday buyurtma uchun "hali hal
// bo'lmagan" eslatmasini yuborish behuda — xodimga aytilishi kerak
// bo'lgan narsa boshqa: POSILKA KELGAN, adminka holati eskirgan.
//
// Ikkinchi tekshiruv — EGASI. Dashboarddagi `user_id` adminkadagi
// `user_id` bilan bir xil bo'lishi kerak. Mos kelmasa posilka boshqa
// odamning akkauntiga biriktirilgan: bu ma'lumot XATOSI, o'zidan hal
// bo'lmaydi va qo'lda tuzatishni talab qiladi.
//
// Ikkala holatda ham muammo YOPILMAYDI — ochiq qoladi, ya'ni mijozga
// "tekshirilmoqda" deb aytiladi (OrderView.InReview → modelga
// `tekshiruvda`), javobni xodim beradi.
package support

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Yetkazma bilan solishtirish natijalari (OrderIssue.DashboardAlert).
const (
	// DashAlertArrived - trek yetkazmada chiqdi, adminka esa hamon
	// "kutilmoqda" deb turadi.
	DashAlertArrived = "arrived"
	// DashAlertOwner - yetkazmadagi user_id adminkadagisiga mos kelmadi.
	DashAlertOwner = "owner_mismatch"
)

// DashboardCheck - bitta buyurtmaning yetkazma tomonidagi holati.
type DashboardCheck struct {
	Track    string        `json:"track"`    // solishtirishga ishlatilgan trek
	Found    bool          `json:"found"`    // trek yetkazmada chiqdimi
	Rows     int           `json:"rows"`     // shu trekka nechta yozuv keldi
	Row      DeliveryOrder `json:"row"`      // asosiy yozuv (egasi mos kelgani)
	OwnerID  int64         `json:"owner_id"` // adminkadagi user_id
	DashID   int64         `json:"dash_id"`  // yetkazmadagi user_id
	Mismatch bool          `json:"mismatch"` // ikkisi mos kelmadi
}

// Delivered - mijoz posilkani ALLAQACHON olib ketganmi.
func (c *DashboardCheck) Delivered() bool {
	return c != nil && c.Found && c.Row.Delivered
}

// Alert - shu natija guruhga chiqishi kerakmi va qanday nom bilan.
// Chiqmasa bo'sh satr.
//
// Mijoz olib ketgan posilka guruhga CHIQMAYDI: u muammo emas, yozuv
// o'z-o'zidan yopiladi (DropDeliveredIssues). Istisno — egasi mos
// kelmagani: posilkani boshqa odam olib ketgan bo'lsa, bu aynan
// aytilishi kerak bo'lgan xato.
func (c *DashboardCheck) Alert() string {
	switch {
	case c == nil || !c.Found:
		return ""
	case c.Mismatch:
		return DashAlertOwner
	case c.Row.Delivered:
		return ""
	default:
		return DashAlertArrived
	}
}

// deliveredResolutionFor - "mijoz olib ketgan" deb yopilgan muammoning
// yechim matni: qachon va qaysi filialdan olingani.
func deliveredResolutionFor(is *OrderIssue, chk *DashboardCheck) string {
	var b strings.Builder
	b.WriteString("Mijoz posilkani olib ketgan (yetkazmada delivered=true)")
	if chk.Row.DeliveredAt != "" {
		fmt.Fprintf(&b, " — %s", chk.Row.DeliveredAt)
	}
	if chk.Row.BranchName != "" {
		fmt.Fprintf(&b, ", %s", trimText(chk.Row.BranchName, 60))
	}
	fmt.Fprintf(&b, ". Adminkadagi holat (%q) eskirgan.", is.StatusLabel)
	return b.String()
}

// DropDeliveredIssues - mijoz ALLAQACHON olib ketgan posilkalarni yangi
// muammolar ro'yxatidan chiqaradi va yozuvini yopadi. Qaytadigan qiymat
// — chiqarib tashlanganlar soni.
//
// Nega kerak: muammo qarori ADMINKA holatiga qarab chiqariladi
// (IsProblem), adminkadagi "kiritish uchun kutilmoqda" esa Xitoy
// tomonidagi holat va u yangilanmay qolishi mumkin. Shu sababli mijoz
// posilkani filialdan olib ketganidan keyin ham buyurtma "muammoli"
// bo'lib guruhga chiqardi. Yetkazmada `delivered=true` — buyurtma
// yopilgan: posilka mijozning qo'lida, xodimga ko'rsatadigan narsa yo'q.
func DropDeliveredIssues(list []*OrderIssue, rows []DeliveryOrder) ([]*OrderIssue, int) {
	if len(list) == 0 || len(rows) == 0 {
		return list, 0
	}
	// Olib ketilgan posilkalar: trek → yozuv.
	taken := make(map[string]DeliveryOrder, len(rows))
	for _, d := range rows {
		if !d.Delivered {
			continue
		}
		if t := trackKey(d.ExpressNum); t != "" {
			taken[t] = d
		}
	}
	if len(taken) == 0 {
		return list, 0
	}

	out := make([]*OrderIssue, 0, len(list))
	dropped := 0
	for _, is := range list {
		d, ok := taken[trackKey(is.ExpressNum)]
		if !ok {
			out = append(out, is)
			continue
		}
		chk := &DashboardCheck{Track: trackKey(is.ExpressNum), Found: true, Rows: 1, Row: d}
		res := deliveredResolutionFor(is, chk)
		if err := ResolveIssue(DB, is, res, "tizim", ResolvedViaAuto); err != nil {
			log.Printf("muammo: %s olib ketilgan deb yopilmadi: %v", is.OrderSN, err)
			out = append(out, is)
			continue
		}
		log.Printf("muammo: %s — mijoz posilkani olib ketgan, guruhga chiqarilmadi", is.OrderSN)
		dropped++
	}
	return out, dropped
}

// CheckDashboard - buyurtmaning trek raqamini yetkazmada qidiradi va
// egasini solishtiradi. Trek yo'q bo'lsa (Xitoyda hali berilmagan) nil
// qaytadi: solishtirishga hech narsa yo'q.
func CheckDashboard(svc Service, token string, o AdminkaOrder) (*DashboardCheck, error) {
	track := trackKey(o.ExpressNum)
	if track == "" {
		return nil, nil
	}
	rows, err := fetchDeliveryRetry(svc, token,
		DeliveryFilter{TrackNumber: track, Size: DefaultOrdersPerCall})
	if err != nil {
		return nil, err
	}
	return compareDashboard(o, rows), nil
}

// compareDashboard - olingan yetkazma yozuvlarini buyurtma bilan
// solishtiradi. CheckDashboard dan ajratilgan: so'rovsiz sinaladi.
func compareDashboard(o AdminkaOrder, rows []DeliveryOrder) *DashboardCheck {
	track := trackKey(o.ExpressNum)
	if track == "" {
		return nil
	}
	chk := &DashboardCheck{Track: track, OwnerID: o.UserID}
	for _, d := range rows {
		// Trek bo'yicha qidiruv butun bazadan ketadi va o'xshash
		// raqamlarni ham qaytarishi mumkin — AYNAN shu trekka
		// tegishlilari qoldiriladi.
		if trackKey(d.ExpressNum) != track {
			continue
		}
		chk.Rows++
		if !chk.Found {
			chk.Found, chk.Row, chk.DashID = true, d, d.UserID
			continue
		}
		// Bitta trekka bir nechta qator kelsa, egasi MOS KELGANI
		// asosiy hisoblanadi: aks holda tasodifiy birinchi qatorga
		// qarab "egasi xato" deb xabar ketib qolardi.
		if o.UserID > 0 && d.UserID == o.UserID {
			chk.Row, chk.DashID = d, d.UserID
		}
	}
	// Egasi noma'lum bo'lsa (biror tomonda user_id bo'sh) xato deb
	// hisoblanmaydi: ikkala API ham bu maydonni ba'zan bermaydi.
	chk.Mismatch = chk.Found && chk.OwnerID > 0 && chk.DashID > 0 && chk.DashID != chk.OwnerID
	return chk
}

// dashboardAlertText - guruhga ketadigan xabar matni.
func dashboardAlertText(is *OrderIssue, chk *DashboardCheck, days int) string {
	var title string
	if chk.Mismatch {
		title = fmt.Sprintf("⛔ XATOLIK: posilka boshqa akkauntda — %s", is.OrderSN)
	} else {
		title = fmt.Sprintf("📦 Posilka KELGAN, adminka holati eskirgan — %s", is.OrderSN)
	}

	var b strings.Builder
	b.WriteString(guruhSarlavha(title, issueOwner(is), is.ClientID, is.ConversationID))
	b.WriteString("\n")
	fmt.Fprintf(&b, "Adminka holati: %s · to'langaniga %d kun\n", is.StatusLabel, days)
	fmt.Fprintf(&b, "Trek: %s\n", chk.Track)

	if chk.Mismatch {
		fmt.Fprintf(&b, "Adminkada egasi: %d\n", chk.OwnerID)
		fmt.Fprintf(&b, "Yetkazmada egasi: %d — MOS KELMAYDI\n", chk.DashID)
	}
	d := chk.Row
	if d.FullName != "" {
		fmt.Fprintf(&b, "Yetkazmada qabul qiluvchi: %s\n", trimText(d.FullName, 60))
	}
	if d.BranchName != "" {
		fmt.Fprintf(&b, "Filial: %s\n", trimText(d.BranchName, 60))
	}
	if d.Delivered {
		fmt.Fprintf(&b, "Olib ketilgan: ha (%s)\n", d.DeliveredAt)
	} else {
		b.WriteString("Olib ketilgan: yo'q\n")
	}
	if chk.Rows > 1 {
		fmt.Fprintf(&b, "Eslatma: shu trekka yetkazmada %d ta yozuv bor\n", chk.Rows)
	}

	if chk.Mismatch {
		b.WriteString("\nAdminka va yetkazmada buyurtma egasi boshqa-boshqa — " +
			"qo'lda tuzatish kerak. Mijozga \"tekshirilmoqda\" deb aytiladi.\n")
	} else {
		b.WriteString("\nPosilka O'zbekistonga kelgan, adminkada esa holat hali " +
			"o'zgarmagan — adminkadagi holatni tekshirib qo'yish kerak.\n")
	}

	b.WriteString(guruhFooter(false))
	return b.String()
}

// notifyDashboardAlert - solishtirish natijasini guruhga BIR MARTA
// chiqaradi. Qaytadigan qiymat: xabar ketdimi.
//
// Takror yuborilmaydi: yozuvda qaysi natija chiqqani saqlanadi va
// faqat natija O'ZGARSA (masalan "kelgan" dan "egasi xato" ga) yangi
// xabar ketadi. Aks holda ochiq muammo IssueReviewSec da bir marta —
// ya'ni kuniga o'nlab marta — guruhni bezovta qilardi.
func notifyDashboardAlert(is *OrderIssue, chk *DashboardCheck, days int) bool {
	kind := chk.Alert()
	if kind == "" || is.DashboardAlert == kind {
		return false
	}

	msgID, err := SendTelegramIssue(dashboardAlertText(is, chk, days))
	if err != nil {
		log.Printf("muammo: %s — yetkazma solishtiruvi guruhga yuborilmadi: %v",
			is.OrderSN, err)
		return false
	}

	now := time.Now()
	RememberTelegramPost(msgID, is.ConversationID, is.ClientID, "issue", 0)
	is.TgMessageID = msgID
	is.NotifyCount++
	is.LastNotifiedAt = &now
	is.DashboardAlert = kind
	is.DashboardAlertAt = &now
	// Reply endi shu xabarga tushadi; last_notified_at yangilangani
	// uchun oddiy eslatma ham ISSUE_REMIND_HOURS kutadi — bitta muammo
	// bo'yicha ketma-ket ikkita xabar chiqmaydi.
	DB.Model(is).Updates(map[string]any{
		"tg_message_id":      msgID,
		"notify_count":       is.NotifyCount,
		"last_notified_at":   &now,
		"dashboard_alert":    kind,
		"dashboard_alert_at": &now,
	})
	log.Printf("muammo: %s — yetkazma solishtiruvi guruhga chiqdi (%s)", is.OrderSN, kind)
	return true
}

// deliveryAuth - yetkazma tokenini BIRINCHI kerak bo'lganda oladi.
// Ochiq muammolarning hammasi treksiz bo'lsa login so'rovi umuman
// ketmaydi; bir siklda esa bir martadan ko'p olinmaydi.
type deliveryAuth struct {
	svc   Service
	token string
	err   error
	done  bool
}

func (d *deliveryAuth) get() (Service, string, error) {
	if !d.done {
		d.done = true
		d.svc = ServiceFromEnv()
		d.token, d.err = ServiceToken(d.svc, ServiceTokenFile)
	}
	return d.svc, d.token, d.err
}

// issuesHaveTrack - ro'yxatda treki bor muammo bormi. Treksiz buyurtma
// yetkazmada qidirilmaydi (posilka Xitoyda, hali yo'lga chiqmagan) —
// bunday holatda yetkazma so'rovi umuman yuborilmaydi.
func issuesHaveTrack(list []*OrderIssue) bool {
	for _, is := range list {
		if trackKey(is.ExpressNum) != "" {
			return true
		}
	}
	return false
}
