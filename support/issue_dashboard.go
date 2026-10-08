// Adminka buyurtmasini YETKAZMA (dashboard) bilan trek raqami orqali
// solishtirish — HAR QANDAY adminka statusida.
//
// Nega status bo'yicha cheklab bo'lmaydi: adminkadagi holat Xitoy
// tomonidagi holat va u yangilanmay qolishi mumkin. "Kiritish uchun
// kutilmoqda" (status 4) deb turgan posilka allaqachon O'zbekistonda,
// filialda yoki mijozning qo'lida bo'lishi mumkin. Shuning uchun
// buyurtmaning haqiqiy holati faqat bitta narsadan bilinadi: TREK
// RAQAMI yetkazmada chiqdimi.
//
// Shundan ikkita qoida kelib chiqadi:
//
//  1. Trek yetkazmada bor — posilka O'zbekistonga KELGAN (filialda
//     kutmoqda yoki mijoz olib ketgan). Bu MUAMMO EMAS: adminkadagi
//     holat nima deb tursa ham, buyurtma o'z yo'lida. Bunday buyurtma
//     muammolar ro'yxatiga qo'shilmaydi va guruhga chiqmaydi.
//
//  2. EGASI mos kelmasa — yetkazmadagi `user_id` adminkadagi `user_id`
//     bilan bir xil emas — bu ma'lumot XATOSI: posilka boshqa odamning
//     akkauntiga biriktirilgan. U o'zidan hal bo'lmaydi va qo'lda
//     tuzatishni talab qiladi, shuning uchun xodimga chiqadi. Bu
//     tekshiruv ham har qanday statusda ishlaydi.
package support

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// DashAlertOwner - yetkazmadagi user_id adminkadagisiga mos kelmadi
// (OrderIssue.DashboardAlert). Guruhga chiqadigan yagona natija.
const DashAlertOwner = "owner_mismatch"

// DashboardCheck - bitta buyurtmaning yetkazma tomonidagi holati.
type DashboardCheck struct {
	OrderSN  string        `json:"order_sn"`
	Track    string        `json:"track"`    // solishtirishga ishlatilgan trek
	Found    bool          `json:"found"`    // trek yetkazmada chiqdimi
	Rows     int           `json:"rows"`     // shu trekka nechta yozuv keldi
	Row      DeliveryOrder `json:"row"`      // asosiy yozuv (egasi mos kelgani)
	OwnerID  int64         `json:"owner_id"` // adminkadagi user_id
	DashID   int64         `json:"dash_id"`  // yetkazmadagi user_id
	Mismatch bool          `json:"mismatch"` // ikkisi mos kelmadi
}

// Arrived - posilka O'zbekistonga kelganmi (yetkazmada yozuvi bormi).
func (c *DashboardCheck) Arrived() bool { return c != nil && c.Found }

// Delivered - mijoz posilkani ALLAQACHON olib ketganmi.
func (c *DashboardCheck) Delivered() bool { return c.Arrived() && c.Row.Delivered }

// Alert - shu natija xodimga chiqishi kerakmi va qanday nom bilan.
// Chiqmasa bo'sh satr.
//
// Yetkazmada chiqqan posilkaning o'zi xabar emas: u normal holat —
// posilka kelgan, filialda kutmoqda yoki mijoz olib ketgan. Xabarga
// arzigulik yagona narsa — EGASI mos kelmagani.
func (c *DashboardCheck) Alert() string {
	if c.Arrived() && c.Mismatch {
		return DashAlertOwner
	}
	return ""
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
	chk := &DashboardCheck{OrderSN: o.OrderSN, Track: track, OwnerID: o.UserID}
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

// CrossCheckOrders - adminkadagi HAMMA buyurtmani (statusidan qat'i
// nazar) yetkazma yozuvlari bilan solishtiradi. Treksiz buyurtmalar
// tashlab ketiladi: ular hali Xitoyda, solishtirishga narsa yo'q.
//
// Yetkazma ro'yxati bir marta olinadi va shu yerda taqsimlanadi —
// har bir buyurtma uchun alohida so'rov yuborilmaydi.
func CrossCheckOrders(views []OrderView, rows []DeliveryOrder) []*DashboardCheck {
	out := make([]*DashboardCheck, 0, len(views))
	for _, v := range views {
		if chk := compareDashboard(v.AdminkaOrder, rows); chk != nil {
			out = append(out, chk)
		}
	}
	return out
}

// MismatchAlerts - egasi mos kelmagan buyurtmalar uchun xodimga
// ketadigan ogohlantirishlar (agent.go dagi `alerts` ro'yxatiga
// qo'shiladi: bunday holat guruhga alohida xabar emas, mavjud xabar
// ichida chiqadi).
//
// Yetkazma yozuvining yoshi ham qaytariladi: qolgan ogohlantirishlar
// kabi bu ham eskirgan bo'lsa to'siladi (DropStaleAlerts) — mijoz
// o'zi so'ramagan oylik yozuv bo'yicha xodim qiladigan ish yo'q.
func MismatchAlerts(checks []*DashboardCheck) []DeliveryAlert {
	var out []DeliveryAlert
	for _, c := range checks {
		if c.Alert() != DashAlertOwner {
			continue
		}
		out = append(out, DeliveryAlert{
			ExpressNum: c.Track,
			Days:       daysSinceText(c.Row.CreatedAt),
			Text: fmt.Sprintf(
				"%s — XATOLIK: posilka boshqa akkauntga biriktirilgan. "+
					"Adminkada egasi %d, yetkazmada %d (trek %s). Qo'lda tuzatish kerak.",
				firstNonEmpty(c.OrderSN, c.Track), c.OwnerID, c.DashID, c.Track),
		})
	}
	return out
}

// arrivedResolution - "yetkazmada bor" deb yopilgan muammoning yechim
// matni: posilka mijozning qo'lidami yoki filialda kutmoqdami.
func arrivedResolution(is *OrderIssue, chk *DashboardCheck) string {
	var b strings.Builder
	if chk.Delivered() {
		b.WriteString("Mijoz posilkani olib ketgan (yetkazmada delivered=true)")
		if chk.Row.DeliveredAt != "" {
			fmt.Fprintf(&b, " — %s", chk.Row.DeliveredAt)
		}
	} else {
		b.WriteString("Posilka O'zbekistonga kelgan — yetkazmada yozuvi bor")
	}
	if chk.Row.BranchName != "" {
		fmt.Fprintf(&b, ", %s", trimText(chk.Row.BranchName, 60))
	}
	fmt.Fprintf(&b, ". Adminkadagi holat (%q) eskirgan.", is.StatusLabel)
	return b.String()
}

// DropArrivedIssues - posilkasi yetkazmada CHIQQAN buyurtmalarni yangi
// muammolar ro'yxatidan olib tashlaydi va yozuvini yopadi. Qaytadigan
// qiymatlar: qolgan ro'yxat va tashlanganlar soni.
//
// Nega kerak: muammo qarori ADMINKA holatiga qarab chiqariladi
// (IsProblem), adminkadagi holat esa Xitoy tomonidagi va yangilanmay
// qolishi mumkin. Shu sababli O'zbekistonga yetib kelgan — hatto mijoz
// olib ketgan — buyurtma ham "muammoli" bo'lib guruhga chiqardi.
// Yetkazmada yozuvi bor posilka o'z yo'lida: xodimga ko'rsatadigan
// narsa yo'q.
//
// Egasi mos kelmagani bundan mustasno — u tashlanmaydi: xato aynan shu
// yerda va u haqida xabar berish kerak.
func DropArrivedIssues(list []*OrderIssue, rows []DeliveryOrder) ([]*OrderIssue, int) {
	if len(list) == 0 || len(rows) == 0 {
		return list, 0
	}
	out := make([]*OrderIssue, 0, len(list))
	dropped := 0
	for _, is := range list {
		chk := compareDashboard(
			AdminkaOrder{OrderSN: is.OrderSN, UserID: is.OwnerUserID, ExpressNum: is.ExpressNum},
			rows)
		if !chk.Arrived() || chk.Mismatch {
			out = append(out, is)
			continue
		}
		res := arrivedResolution(is, chk)
		if err := ResolveIssue(DB, is, res, "tizim", ResolvedViaAuto); err != nil {
			log.Printf("muammo: %s yetkazmada bor deb yopilmadi: %v", is.OrderSN, err)
			out = append(out, is)
			continue
		}
		log.Printf("muammo: %s — posilka yetkazmada bor, guruhga chiqarilmadi", is.OrderSN)
		dropped++
	}
	return out, dropped
}

// dashboardAlertText - guruhga ketadigan xabar matni (egasi mos
// kelmagani). Ochiq muammolar siklida ishlatiladi: u yerda ilinadigan
// mavjud xabar bo'lmaydi, shuning uchun alohida xabar chiqariladi.
func dashboardAlertText(is *OrderIssue, chk *DashboardCheck, days int) string {
	var b strings.Builder
	b.WriteString(guruhSarlavha(
		fmt.Sprintf("⛔ XATOLIK: posilka boshqa akkauntda — %s", is.OrderSN),
		issueOwner(is), is.ClientID, is.ConversationID))
	b.WriteString("\n")
	fmt.Fprintf(&b, "Adminka holati: %s · to'langaniga %d kun\n", is.StatusLabel, days)
	fmt.Fprintf(&b, "Trek: %s\n", chk.Track)
	fmt.Fprintf(&b, "Adminkada egasi: %d\n", chk.OwnerID)
	fmt.Fprintf(&b, "Yetkazmada egasi: %d — MOS KELMAYDI\n", chk.DashID)

	d := chk.Row
	if d.FullName != "" {
		fmt.Fprintf(&b, "Yetkazmada qabul qiluvchi: %s\n", trimText(d.FullName, 60))
	}
	if d.BranchName != "" {
		fmt.Fprintf(&b, "Filial: %s\n", trimText(d.BranchName, 60))
	}
	if d.Delivered {
		fmt.Fprintf(&b, "Olib ketilgan: ha (%s) — posilkani BOSHQA odam olgan\n", d.DeliveredAt)
	} else {
		b.WriteString("Olib ketilgan: yo'q\n")
	}
	if chk.Rows > 1 {
		fmt.Fprintf(&b, "Eslatma: shu trekka yetkazmada %d ta yozuv bor\n", chk.Rows)
	}

	b.WriteString("\nAdminka va yetkazmada buyurtma egasi boshqa-boshqa — " +
		"qo'lda tuzatish kerak. Mijozga \"tekshirilmoqda\" deb aytiladi.\n")
	b.WriteString(guruhFooter(false))
	return b.String()
}

// notifyDashboardAlert - solishtirish natijasini guruhga BIR MARTA
// chiqaradi. Qaytadigan qiymat: xabar ketdimi.
//
// Takror yuborilmaydi: yozuvda natija saqlanadi va faqat u O'ZGARSA
// yangi xabar ketadi. Aks holda ochiq muammo har `ISSUE_REVIEW_SEC` da
// — ya'ni kuniga o'nlab marta — guruhni bezovta qilardi.
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
