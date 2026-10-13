// Buyurtmani KETMA-KET tekshirish: bitta buyurtma haqidagi qaror shu
// yerda, qat'iy tartibda chiqariladi.
//
// Nega bitta joyda: ilgari qoidalar uch joyda alohida yozilgan edi —
// IsProblem (issue.go) adminka statusiga qarardi, MarkArrival
// (context.go) yetkazmaga, fetchSystemData (agent.go) esa ikkisining
// natijasini aralashtirardi. Ular bir-birini ko'rmagani uchun javob
// zid chiqardi: modelga "posilka yo'lda, tez orada keladi" deyilib
// turib, xuddi shu buyurtma xodimlar guruhiga "⚠️ Muammoli buyurtma"
// bo'lib tushardi.
//
// Endi tartib bitta (CheckOrder) va undan uchta narsa chiqadi:
//
//	modelga   → OrderBrief maydonlari (ApplyChecks)
//	xodimga   → bir qatorlik ogohlantirish (CheckAlerts)
//	bazaga    → muammo yozuvi (OpenLateShipmentIssues)
package support

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// DashVerdict - buyurtmaning YETKAZMA (dashboard) tomonidagi javobi.
//
// "Qidirildi, topilmadi" bilan "qidirib ko'rilmadi" ni ARALASHTIRIB
// BO'LMAYDI: yetkazma API bir marta xato qaytarsa, aralashtirgan kod
// mijozning butun ro'yxatini "muammo" deb guruhga yog'diradi.
type DashVerdict int

const (
	DashUnknown  DashVerdict = iota // trek yo'q, so'rov qilinmadi yoki xato berdi
	DashMissing                     // qidirildi — yetkazmada YO'Q
	DashFound                       // yetkazmada bor
	DashMismatch                    // yetkazmada bor, lekin EGASI boshqa odam
)

// OrderCheckInput - ketma-ketlikka kerak bo'lgan hamma ma'lumot.
//
// Tashqi so'rov qilinmaydi (xuddi compareDashboard CheckDashboard dan
// ajratilgani kabi): har bir holat testda API'siz tekshiriladi.
type OrderCheckInput struct {
	Order AdminkaOrder
	// Rows - yetkazmadan kelgan BUTUN ro'yxat (so'ralgan buyurtma
	// bo'yicha qisqartirilmagan): "trek yetkazmada bormi" savoliga
	// javob shundan chiqadi.
	Rows []DeliveryOrder
	// HaveDelivery - yetkazma ma'lumoti umuman olindimi.
	HaveDelivery bool
	// DeliveryOK - so'rovlar xatosiz tugadimi.
	DeliveryOK bool
	// ForeignTrack - shu buyurtmaning treki yetkazmada BOSHQA
	// akkauntda chiqdi (onlyOwnDelivery uni ro'yxatdan olib tashlagan).
	ForeignTrack bool
}

// Ketma-ketlik qadamlari — tekshiruv QAYERDA to'xtagani. Log va test
// uchun: "nega bu buyurtma muammo bo'ldi" degan savolga javob.
const (
	StepUnpaid      = "tolanmagan"
	StepNoTrack     = "trek_yoq"
	StepDashUnknown = "yetkazma_nomalum"
	StepOwner       = "egasi_mos_kelmadi"
	StepRegion      = "viloyat_mos_kelmadi"
	StepDashFound   = "yetkazmada_bor"
	StepOnTheWay    = "yol_muddati_ichida"
	StepLate        = "yol_muddati_otdi"
	StepAskReceived = "mijozdan_sora"
	StepNoShipDate  = "jonatish_sanasi_yoq"
	StepStuck       = "holatda_qotgan"
	StepWaiting     = "hali_kutilmoqda"
)

// Muammo sabablari — OrderIssue.Reason ga yoziladi va guruh xabarida
// ko'rinadi.
const (
	ReasonStuckStatus  = "holatda_qotgan"
	ReasonLateShipment = "yol_muddati_otdi"
	ReasonVeryLate     = "mijozdan_sorash"
	ReasonNoShipDate   = "jonatish_sanasi_yoq"
	ReasonOwner        = "egasi_mos_kelmadi"
	ReasonRegion       = "viloyat_mos_kelmadi"
)

// OrderCheck - ketma-ket tekshiruv natijasi.
type OrderCheck struct {
	OrderSN       string
	Track         string
	Status        int
	Paid          bool
	DaysSincePaid int
	// ShippedDays - yo'lga chiqqaniga necha kun. -1 — sana yo'q yoki
	// o'qilmadi (0 EMAS: nol "bugun chiqdi" degani).
	ShippedDays int
	Dash        DashVerdict
	Step        string
	Reason      string
	Problem     bool

	// ETAFrom/ETATo - yetib kelishiga qolgan kun oralig'i (0,0 — oraliq
	// yo'q, hech narsa aytilmaydi).
	ETAFrom, ETATo int

	// Modelga ketadigan tayyor matnlar (OrderBrief maydonlari).
	Arrived     string // yetkazma
	ETA         string // qolgan_kun
	OnTheWay    string // yolda_tez_keladi
	Late        string // DIQQAT_muddat_otdi
	AskReceived string // DIQQAT_mijozdan_sora
	Mismatch    string // DIQQAT_xatolik

	// StaffText - xodimlar guruhiga ketadigan bir qator ("" — chiqmaydi).
	StaffText string
}

// CheckOrder - buyurtmani QAT'IY KETMA-KETLIKDA tekshiradi.
//
// Tartib muhim: har qadam o'zidan keyingisini ma'nosiz qiladi.
//
//  0. to'lanmagan            → hech narsa tekshirilmaydi
//  1. treki yo'q             → yetkazmada qidirishga narsa yo'q (8-ga)
//  2. yetkazma noma'lum      → "topilmadi" deb xulosa chiqarilmaydi (8-ga)
//  3. egasi mos kelmadi      → MUAMMO (adminka id ≠ yetkazma id)
//  4. viloyat mos kelmadi    → MUAMMO (posilka boshqa viloyatga tushgan)
//  5. yetkazmada bor         → MUAMMO EMAS, to'xtaydi
//  6. yo'lda, muddat ichida  → MUAMMO EMAS, qolgan kun oralig'i aytiladi
//  7. yo'l muddati o'tgan    → MUAMMO (adminka statusiga QARALMAYDI)
//  8. statusda qotgan        → MUAMMO (statusga qarab chegara)
func CheckOrder(in OrderCheckInput) OrderCheck {
	o := in.Order
	c := OrderCheck{
		OrderSN:       o.OrderSN,
		Track:         trackKey(o.ExpressNum),
		Status:        o.Status,
		Paid:          IsPaid(o),
		DaysSincePaid: DaysSincePaid(o),
		ShippedDays:   ShippedDaysOf(o),
	}

	// 0. To'lanmagan buyurtma kutib turishi normal: xodim aralashuvi
	//    ham, mijozga muddat aytish ham kerak emas.
	if !c.Paid {
		c.Step = StepUnpaid
		return c
	}

	// 1-2. Yetkazma tomoni javob bera oladimi.
	switch {
	case c.Track == "":
		// Trek Xitoyda hali berilmagan — solishtirishga narsa yo'q.
		c.Dash, c.Step = DashUnknown, StepNoTrack
		return stuckVerdict(c, o)
	case !in.HaveDelivery || !in.DeliveryOK:
		// So'rov qilinmadi yoki xato berdi. Buyurtma "yetkazmada yo'q"
		// DEB HISOBLANMAYDI: aks holda API bir marta yotganda hamma
		// buyurtma muammo bo'lib guruhga tushadi.
		c.Dash, c.Step = DashUnknown, StepDashUnknown
		return stuckVerdict(c, o)
	}

	chk := compareDashboard(o, in.Rows)

	// 3. Egasi mos kelmadi. Bu tekshiruv 7-qadamdan OLDIN turadi:
	//    begona akkauntdagi qator ro'yxatdan olib tashlangan
	//    (onlyOwnDelivery), ya'ni "yetkazmada yo'q" bo'lib ko'rinadi va
	//    soxta "yo'l muddati o'tdi" muammosiga aylanib qolardi.
	if in.ForeignTrack || (chk != nil && chk.Mismatch) {
		c.Dash, c.Step, c.Reason, c.Problem = DashMismatch, StepOwner, ReasonOwner, true
		c.Mismatch = ownerMismatchNote
		c.StaffText = fmt.Sprintf(staffOwnerLine, numOrTrack(c), dashOwner(chk), o.UserID)
		return c
	}

	if chk == nil || !chk.Arrived() {
		c.Dash = DashMissing
		return shipmentVerdict(c, o)
	}

	// Posilka O'zbekistonga kelgan.
	c.Dash, c.Arrived = DashFound, arrivedInDelivery

	// 4. Manzil to'g'rimi: adminkadagi viloyat bilan posilka tushgan
	//    joy bir xilmi.
	if ok, want, got, raw := checkRegion(o, chk.Row); !ok {
		c.Step, c.Reason, c.Problem = StepRegion, ReasonRegion, true
		c.Mismatch = fmt.Sprintf(provinceMismatchNote, want, got, raw)
		c.StaffText = fmt.Sprintf(staffRegionLine, numOrTrack(c), want, got, raw)
		return c
	}

	// 5. Hammasi joyida — ketma-ketlik to'xtaydi (mavjud qoida:
	//    yetkazmada bor posilka MUAMMO EMAS).
	c.Step = StepDashFound
	return c
}

// shipmentVerdict - posilka yetkazmada YO'Q: qaror yo'lga chiqqan
// sanaga qarab chiqariladi (6, 7a, 7b, 7c qadamlari).
func shipmentVerdict(c OrderCheck, o AdminkaOrder) OrderCheck {
	c.Arrived = notInDelivery
	sana := sanaMatn(o.ShippedAt)

	switch {
	// 7c. Jo'natilgan sanasi ham yo'q, yetkazmada ham yo'q: buyurtma
	//     ikki tomonning hech birida ko'rinmaydi. Lekin kun chegarasi
	//     baribir tekshiriladi — bir soat oldin to'langan buyurtma
	//     muammo emas.
	case c.ShippedDays < 0:
		c = stuckVerdict(c, o)
		if c.Problem {
			c.Step, c.Reason = StepNoShipDate, ReasonNoShipDate
			c.Late = noShipDateNote
			c.StaffText = fmt.Sprintf(staffNoDateLine, numOrTrack(c), c.DaysSincePaid)
		}
		return c

	// 6. Yo'l muddati ichida — kutish normal, mijozga qolgan kun
	//    oralig'i aytiladi.
	case c.ShippedDays <= FreshShipmentDays():
		c.Step = StepOnTheWay
		c.ETAFrom, c.ETATo = OrderETA(c.ShippedDays)
		c.ETA = etaText(c.ETAFrom, c.ETATo)
		c.OnTheWay = fmt.Sprintf(onTheWayETANote, sana, sana, c.ETA)
		return c

	// 7a. Muddat o'tdi. Adminka statusiga QARALMAYDI: status 6/7
	//     kuzatiladigan ro'yxatda yo'q, lekin posilka yo'lda yo'qolgan
	//     bo'lsa aynan shu holat xodimga kerak.
	case c.ShippedDays <= LongShipmentDays():
		c.Step, c.Reason, c.Problem = StepLate, ReasonLateShipment, true
		c.Late = fmt.Sprintf(lateShipmentNote, sana)
		c.StaffText = fmt.Sprintf(staffLateLine, numOrTrack(c), c.ShippedDays, sana)
		return c

	// 7b. Juda uzoq: posilka mijozning qo'liga tegib, tizimda
	//     belgilanmay qolgan bo'lishi ham mumkin — avval mijozdan
	//     so'raladi.
	default:
		c.Step, c.Reason, c.Problem = StepAskReceived, ReasonVeryLate, true
		c.AskReceived = fmt.Sprintf(askReceivedNote, LongShipmentDays())
		c.StaffText = fmt.Sprintf(staffVeryLateLine, numOrTrack(c), c.ShippedDays, sana)
		return c
	}
}

// stuckVerdict - 8-qadam: buyurtma adminkada QOTIB QOLGANmi.
//
// Yetkazma tomoni javob bermaganda (trek yo'q, so'rov xato berdi) yoki
// jo'natish sanasi bo'lmaganda yagona qolgan mezon shu.
func stuckVerdict(c OrderCheck, o AdminkaOrder) OrderCheck {
	switch {
	case !watchedStatus(o.Status):
		// Status kuzatilmaydi (masalan 6 — "Xitoyda tranzaksiya
		// yopilgan"): bu yerda aytadigan narsa yo'q.
		c.Step = StepWaiting
	case FreshlyShipped(o):
		// Posilka yaqinda yo'lga chiqqan — to'lovdan beri qancha kun
		// o'tgani ahamiyatsiz, buyurtma qotib qolmagan.
		c.Step = StepOnTheWay
	case c.DaysSincePaid > StatusProblemDays(o.Status):
		c.Step, c.Reason, c.Problem = StepStuck, ReasonStuckStatus, true
	default:
		c.Step = StepWaiting
	}
	return c
}

// numOrTrack - xodim xabarida buyurtma qanday nomlanadi: DG raqami,
// u bo'lmasa trek.
func numOrTrack(c OrderCheck) string {
	if c.OrderSN != "" {
		return c.OrderSN
	}
	return c.Track
}

// dashOwner - yetkazmadagi egasi (noma'lum bo'lsa 0).
func dashOwner(chk *DashboardCheck) int64 {
	if chk == nil {
		return 0
	}
	return chk.DashID
}

// watchedStatus - status kuzatiladigan ro'yxatdami (PROBLEM_STATUSES).
func watchedStatus(status int) bool {
	for _, s := range ProblemStatuses() {
		if s == status {
			return true
		}
	}
	return false
}

// DefaultStatusDays - har bir kuzatiladigan status uchun muammo
// chegarasi (kun).
//
// Statuslarning tezligi bir xil emas: 4 ("kiritish uchun kutilmoqda")
// Xitoy omborida NORMAL holatda ham bir haftaga cho'ziladi, 3 va 12
// esa uch kundan keyin qotib qolgan hisoblanadi. Ilgari hammasiga
// bitta PROBLEM_DAYS=3 qo'llanardi va status 4 li buyurtmalar hech
// qanday muammo bo'lmasa ham guruhga oqib kelardi.
var DefaultStatusDays = map[int]int{
	StatusPaid:     3,
	StatusWaiting:  7,
	StatusAbnormal: 3,
}

// StatusProblemDays - shu status uchun chegara (kun).
//
// .env: PROBLEM_STATUS_DAYS="3:3,4:7,12:3". Ro'yxatda bo'lmagan status
// uchun eski PROBLEM_DAYS ishlatiladi — mavjud sozlamalar buzilmasin.
func StatusProblemDays(status int) int {
	if m := statusDaysMap(); m != nil {
		if d, ok := m[status]; ok {
			return d
		}
		return ProblemDays()
	}
	if d, ok := DefaultStatusDays[status]; ok {
		return d
	}
	return ProblemDays()
}

// statusDaysMap - PROBLEM_STATUS_DAYS ni o'qiydi ("status:kun" juftlari).
// Sozlama berilmagan bo'lsa nil (DefaultStatusDays ishlatiladi).
//
// Noto'g'ri yozilgan juftlik E'TIBORSIZ qoldiriladi, butun sozlama
// tashlanmaydi: bitta xato yozuv butun muammo aniqlashni o'chirib
// qo'ymasligi kerak.
func statusDaysMap() map[int]int {
	v := strings.TrimSpace(envStr("PROBLEM_STATUS_DAYS", ""))
	if v == "" {
		return nil
	}
	out := map[int]int{}
	for _, part := range strings.Split(v, ",") {
		pair := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(pair) != 2 {
			continue
		}
		st, err1 := strconv.Atoi(strings.TrimSpace(pair[0]))
		days, err2 := strconv.Atoi(strings.TrimSpace(pair[1]))
		if err1 != nil || err2 != nil || days < 0 {
			continue
		}
		out[st] = days
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// etaSpan - qolgan kun oralig'ining kengligi.
const etaSpan = 5

// OrderETA - posilka yetib kelishiga QOLGAN kun oralig'i.
//
// Oraliq: [qolgan-5 … qolgan]. Pastki chegara 1 dan tushmaydi —
// "0 kun ichida keladi" degan gap mijozga "bugun" deb o'qiladi va
// bajarilmaydigan va'da bo'lib qoladi.
//
//	12 kun o'tdi → qolgan 8  → 3-8
//	18 kun o'tdi → qolgan 2  → 1-2
//	 1 kun o'tdi → qolgan 19 → 14-19
//
// Sana yo'q (shippedDays < 0) yoki muddat tugagan bo'lsa (0, 0)
// qaytadi: oraliq YO'Q va chaqiruvchi hech narsa yozmasligi kerak.
func OrderETA(shippedDays int) (int, int) {
	if shippedDays < 0 {
		return 0, 0
	}
	left := FreshShipmentDays() - shippedDays
	if left < 1 {
		// Muddatning oxirgi kuni: "0 kun" deyilmaydi.
		left = 1
	}
	from := left - etaSpan
	if from < 1 {
		from = 1
	}
	return from, left
}

// etaText - oraliqning mijozga aytiladigan ko'rinishi.
func etaText(from, to int) string {
	switch {
	case to <= 0:
		return ""
	case from >= to:
		return fmt.Sprintf("uzog'i %d kun ichida yetib keladi", to)
	default:
		return fmt.Sprintf("uzog'i %d-%d kun ichida yetib keladi", from, to)
	}
}

// checkRegion - adminkadagi manzil viloyati (AdminkaOrder.Province) va
// posilka YETKAZMADA tushgan joy (DeliveryOrder.City, u bo'sh bo'lsa
// BranchName) bir viloyatga tushadimi.
//
// Ilgari bu ikki manba bir-biri bilan HECH SOLISHTIRILMAGAN:
// BriefDelivery faqat yetkazma ichidagi `city` va `branch_name` ni
// solishtirardi, adminkadagi `province` esa alohida, modelga
// "viloyat"/"yetkazish" matnini yozishga ketardi. Natijada adminkada
// Jizzax deb turgan buyurtma yetkazmada Sirdaryoga tushsa, yetkazma
// tomonining O'ZIDA ziddiyat yo'q (city=Sirdaryo, filial=Guliston) va
// xatoni hech kim ko'rmasdi.
//
// Ikki tomondan biri bo'sh yoki tanilmagan bo'lsa XATO DEB
// HISOBLANMAYDI: `province` ham, `city` ham ba'zan bo'sh keladi
// (compareDashboard dagi user_id himoyasi kabi).
func checkRegion(o AdminkaOrder, row DeliveryOrder) (bool, string, string, string) {
	want := RegionOf(o.Province)
	raw := firstNonEmpty(row.City, row.BranchName)
	got := RegionOf(raw)
	if want == "" || got == "" || want == got {
		return true, want, got, raw
	}
	return false, want, got, raw
}

// CheckOrders - ro'yxatning hammasi uchun ketma-ketlik BIR MARTA
// bajariladi: natija ham modelga, ham xodimga, ham muammo yozuviga
// ketadi — uch joyda qayta hisoblanmasin.
//
// `foreign` - yetkazmada BOSHQA akkauntda chiqqan treklar
// (onlyOwnDelivery qaytargan ro'yxat), katta harfga keltirilgan.
func CheckOrders(views []OrderView, rows []DeliveryOrder,
	haveDelivery, deliveryOK bool, foreign map[string]bool) []OrderCheck {
	out := make([]OrderCheck, 0, len(views))
	for _, v := range views {
		out = append(out, CheckOrder(OrderCheckInput{
			Order:        v.AdminkaOrder,
			Rows:         rows,
			HaveDelivery: haveDelivery,
			DeliveryOK:   deliveryOK,
			ForeignTrack: foreign[trackKey(v.ExpressNum)],
		}))
	}
	return out
}

// ApplyChecks - tekshiruv natijasini modelga ketadigan buyurtmalarga
// yozadi.
//
// Mavjud matn ustiga yozilmaydi: `DIQQAT_xatolik` ni MarkMismatch ham
// to'ldirishi mumkin (posilka mijoz viloyatidagi filialda emas) va
// ikkita zid ko'rsatma bir vaqtda turmasligi kerak.
func ApplyChecks(briefs []OrderBrief, checks []OrderCheck) {
	byOrder := make(map[string]OrderCheck, len(checks))
	byTrack := make(map[string]OrderCheck, len(checks))
	for _, c := range checks {
		if c.OrderSN != "" {
			byOrder[c.OrderSN] = c
		}
		if c.Track != "" {
			byTrack[c.Track] = c
		}
	}
	for i := range briefs {
		c, ok := byOrder[briefs[i].OrderSN]
		if !ok {
			if c, ok = byTrack[trackKey(briefs[i].ExpressNum)]; !ok {
				continue
			}
		}
		briefs[i].Arrived = c.Arrived
		briefs[i].ETA = c.ETA
		briefs[i].OnTheWay = c.OnTheWay
		briefs[i].Late = c.Late
		briefs[i].AskReceived = c.AskReceived
		if c.Mismatch != "" && briefs[i].Mismatch == "" {
			briefs[i].Mismatch = c.Mismatch
			// Qayerdan olishini aytib bo'lmaydi: ma'lumot ishonchsiz.
			briefs[i].Kind = ""
		}
	}
}

// CheckAlerts - xodimlar guruhiga chiqadigan qatorlar.
//
// Hamma muammo chiqmaydi, faqat ikki holat:
//
//  1. muammo yozuvi AYNAN HOZIR ochilgan (`opened`) — xodim uni
//     birinchi marta ko'radi;
//  2. mijoz O'ZI shu buyurtma yoki trek raqamini yozgan (`asked`) —
//     demak savol hali ochiq.
//
// Nega cheklov kerak: mijozda yarim yil oldin yo'lda yo'qolgan posilka
// bo'lsa, u har bir murojaatda (hatto "rahmat" deganda ham) guruhga
// qayta-qayta chiqib, HOZIR hal qilinishi kerak bo'lgan holatlarni
// ko'zdan yashirardi. Muammo yozuvi esa bazada ochiq turadi va xodim
// uni panelda ko'radi (DropStaleAlerts bilan bir mantiq).
func CheckAlerts(checks []OrderCheck, asked map[string]bool, opened []*OrderIssue) []string {
	fresh := make(map[string]bool, len(opened))
	for _, is := range opened {
		if is != nil {
			fresh[is.OrderSN] = true
		}
	}
	var out []string
	for _, c := range checks {
		if c.StaffText == "" {
			continue
		}
		if !fresh[c.OrderSN] && !asked[c.Track] && !asked[trackKey(c.OrderSN)] {
			continue
		}
		out = append(out, c.StaffText)
	}
	return out
}

// OpenLateShipmentIssues - yetkazma ma'lumoti KELGANDAN KEYIN ochiladigan
// muammolar.
//
// Nega alohida: DetectIssues zanjir boshida, yetkazma ma'lumoti hali
// yo'q paytda ishlaydi va faqat adminka statusiga qaraydi. Posilka
// yo'lda yo'qolgan bo'lsa status 6 yoki 7 da turadi — ular
// PROBLEM_STATUSES da yo'q, ya'ni DetectIssues bu holatni UMUMAN
// ko'rmaydi. Shuning uchun yo'l muddati bo'yicha ochilgan muammo shu
// yerda, yetkazma javobidan keyin yoziladi.
func OpenLateShipmentIssues(checks []OrderCheck, views []OrderView,
	clientID, conversationID int64) []*OrderIssue {
	if DB == nil || len(checks) == 0 {
		return nil
	}
	idx := make(map[string]int, len(views))
	for i, v := range views {
		if v.OrderSN != "" {
			idx[v.OrderSN] = i
		}
	}
	var fresh []*OrderIssue
	for _, c := range checks {
		if !c.Problem || !shipmentReason(c.Reason) {
			continue
		}
		i, ok := idx[c.OrderSN]
		if !ok {
			continue
		}
		is := openIssue(DB, views[i].AdminkaOrder, views[i], clientID, conversationID, c.Reason)
		if is == nil {
			continue
		}
		views[i].InReview = true
		fresh = append(fresh, is)
		log.Printf("muammo: %s — %s (%s), yetkazma javobidan keyin ochildi",
			c.OrderSN, reasonText(c.Reason), c.Step)
	}
	return fresh
}

// shipmentReason - muammo YO'L bo'yicha ochilganmi (adminka statusiga
// emas).
//
// Bunday muammoni IsProblem ko'rmaydi (status 6/7 kuzatilmaydi), ya'ni
// "adminkada holat o'zgardi" degan avtomatik yopish uni ochilgan
// zahoti yopib yuborardi. Uni faqat posilkaning yetkazmada chiqishi
// yopadi.
func shipmentReason(reason string) bool {
	switch reason {
	case ReasonLateShipment, ReasonVeryLate, ReasonNoShipDate:
		return true
	}
	return false
}

// reasonText - sababning guruh xabaridagi ko'rinishi.
func reasonText(reason string) string {
	switch reason {
	case ReasonStuckStatus:
		return "adminkada shu holatda qotib qoldi"
	case ReasonLateShipment:
		return "yo'l muddatidan oshdi, yetkazmada yo'q"
	case ReasonVeryLate:
		return "juda uzoq yo'lda — mijozdan olgan-olmagani so'ralsin"
	case ReasonNoShipDate:
		return "jo'natilgan sanasi yo'q va yetkazmada ham yo'q"
	case ReasonOwner:
		return "yetkazmadagi egasi adminkadagiga mos kelmadi"
	case ReasonRegion:
		return "posilka mijoz viloyatiga tushmagan"
	}
	return reason
}

// Modelga ketadigan tayyor matnlar. Hammasi bir xil qoidaga bo'ysunadi:
// aniq SANA va'da qilinmaydi, mijozdan raqami qayta so'ralmaydi, ichki
// atamalar (status, dashboard) mijozga chiqmaydi.

// onTheWayETANote - posilka yo'l muddati ichida va yetkazmada hali
// yo'q. Ikki narsa beriladi: yo'lga chiqqan SANA va QOLGAN kun
// oralig'i. Oraliqni kod hisoblaydi — model o'zidan kun to'qimasin.
const onTheWayETANote = "Buyurtma %s kuni Xitoydan yo'lga chiqqan, hozir yo'lda — " +
	"bu odatdagi yo'l muddati ichida, MUAMMO EMAS. Mijozga shuni ayt: buyurtma %s kuni " +
	"yo'lga chiqqan va %s. Shu oraliqni AYNAN ayt, aniq sana yoki hafta kuniga " +
	"aylantirma (\"ertaga\", \"dushanbaga\" dema). Kechikish yoki muammo haqida gapirma, " +
	"mijozdan hech narsa so'rama va raqamini QAYTA SO'RAMA."

// lateShipmentNote - yo'l muddati o'tgan, posilka yetkazmada hali
// chiqmagan. Mijozga yangi muddat aytilmaydi: oraliq tugagan, o'ylab
// topadigan narsa yo'q.
const lateShipmentNote = "Posilka %s kuni yo'lga chiqqan — odatdagi yo'l muddatidan oshdi " +
	"va yetkazmada hali chiqmagan. Murojaat xodimga topshirildi. Mijozga shuni ayt: " +
	"buyurtma tekshirilmoqda, hal bo'lishi bilan xabar beramiz. Muddat, sabab yoki " +
	"filial AYTMA, raqamini qayta so'rama."

// noShipDateNote - jo'natish sanasi yo'q va posilka yetkazmada ham
// chiqmadi: buyurtma ikki tomonning hech birida ko'rinmaydi.
const noShipDateNote = "Buyurtmaning jo'natilgan sanasi yo'q va posilka yetkazmada ham " +
	"chiqmadi — murojaat xodimga topshirildi. Mijozga shuni ayt: buyurtma tekshirilmoqda, " +
	"hal bo'lishi bilan xabar beramiz. Sabab va muddat aytma, raqamini qayta so'rama."

// ownerMismatchNote - yetkazmadagi egasi adminkadagiga mos kelmadi.
// Posilka boshqa odamning akkauntiga biriktirilgan: qayerdaligini
// aytish xato bo'ladi.
const ownerMismatchNote = "Posilka yetkazmada BOSHQA akkauntga biriktirilgan — ma'lumot " +
	"xatosi, xodimga topshirildi. Mijozga posilka qayerdaligini yoki qachon yetishini " +
	"AYTMA. Faqat shuni ayt: buyurtmada xatolik aniqlandi, uzr so'raymiz, xodimlar " +
	"tekshirib tuzatmoqda."

// provinceMismatchNote - posilka mijoz viloyatiga emas, boshqa
// viloyatga tushgan (adminkadagi `province` va yetkazmadagi
// `city`/`branch_name` solishtirildi).
const provinceMismatchNote = "Posilka mijozning viloyatiga (%s) emas, %s viloyatiga (%s) " +
	"tushgan — xodimga topshirildi. Mijozga qaysi filialdan olishini AYTMA: hozirgi " +
	"ma'lumot ishonchsiz. Faqat \"tekshirilmoqda\" deb ayt."

// Xodimlar guruhiga ketadigan qatorlar.
const (
	staffLateLine = "%s — posilka %d kundan beri yo'lda (%s kuni chiqqan), yetkazmada yo'q. " +
		"Xitoy tomoni bilan tekshirish kerak"
	staffVeryLateLine = "%s — posilka %d kundan beri yo'lda (%s kuni chiqqan), yetkazmada yo'q. " +
		"Mijozdan olgan-olmagani so'ralsin"
	staffNoDateLine = "%s — jo'natilgan sanasi yo'q va yetkazmada ham yo'q " +
		"(to'lovdan beri %d kun). Qo'lda tekshirish kerak"
	staffOwnerLine = "%s — yetkazmadagi egasi (%d) adminkadagiga (%d) mos kelmadi: " +
		"posilka boshqa akkauntga biriktirilgan"
	staffRegionLine = "%s — adminkada manzil %s viloyati, posilka esa %s (%s) ga tushgan. " +
		"Manzilni tekshirish kerak"
)
