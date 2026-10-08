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
//     tuzatishni talab qiladi. Bunday buyurtma yuqoridagi 1-qoidadan
//     MUSTASNO: yetkazmada chiqqan bo'lsa ham muammo yopilmaydi,
//     odatdagi "⚠️ Muammoli buyurtma" bo'lib ochiq qolaveradi.
//     Tekshiruv har qanday statusda ishlaydi.
package support

import (
	"fmt"
	"log"
	"strings"
)

// DashAlertOwner - yetkazmadagi user_id adminkadagisiga mos kelmadi
// (OrderIssue.DashboardAlert).
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

// Alert - shu natija e'tiborga olinishi kerakmi va qanday nom bilan.
// Bo'lmasa bo'sh satr.
//
// Yetkazmada chiqqan posilkaning o'zi hodisa emas: u normal holat —
// posilka kelgan, filialda kutmoqda yoki mijoz olib ketgan. E'tiborga
// arzigulik yagona narsa — EGASI mos kelmagani.
//
// Bu ALOHIDA guruh xabarini tug'dirmaydi: guruhga faqat ikki turdagi
// xabar boradi ("⚠️ Muammoli buyurtma" va "🆘 Yordam kerak"). Egasi
// mos kelmasa muammo shunchaki OCHIQ qoladi (DropArrivedIssues uni
// yopmaydi) va odatdagi "⚠️" xabari bo'lib chiqadi.
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
