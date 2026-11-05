// Yig'ish rejimi: bitta buyurtma qaysi muammo turlariga tushishini
// aniqlash. Sof funksiya: tashqi so'rov va baza yo'q, har chegara testda
// API'siz tekshiriladi.
package support

import (
	"strings"
	"time"
)

// Muammo turlari (collect_findings.type).
const (
	FindPurchased = "sotib_olingan"     // status 3, 5/20/30 kundan oshgan
	FindAbnormal  = "status_12"         // status 12, hammasi
	FindOnWay     = "yolda"             // yo'lga chiqqaniga 20/30 kundan oshgan, kelmagan
	FindStatus6   = "status_6_kelmagan" // status 6, 25 kundan oshgan, kelmagan
	FindOther     = "boshqa_kelmagan"   // boshqa status, buyurtmaga 25 kundan oshgan, kelmagan
	FindWrong     = "xato_kelgan"       // yetkazmada egasi boshqa odam
	FindCourier   = "kuryer_olmagan"    // punktga kelgan, kuryer olib bormagan
	FindAddress   = "manzilga_yetmagan" // posilka mijoz viloyatiga tushmagan
)

// Kun qaysi sanadan hisoblangani (panelda ko'rsatiladi).
const (
	AnchorCreated  = "created_at"
	AnchorShipped  = "shipped_at"
	AnchorDelivery = "yetkazma_created_at"
)

// CollectLateDays - status 6 va "boshqa" turlari uchun kun chegarasi.
const CollectLateDays = 25

// Finding - buyurtmaning bitta muammo turi.
type Finding struct {
	Type       string
	Bucket     string // "5"/"20"/"30"; bo'linmaydigan turda bo'sh
	AnchorAt   *time.Time
	AnchorKind string
}

// CollectInput - tasniflash uchun kerak bo'lgan hamma ma'lumot.
type CollectInput struct {
	Order AdminkaOrder
	// Rows - trek bo'yicha yetkazmadan kelgan qatorlar.
	Rows []DeliveryOrder
	// DeliveryOK - yetkazma so'rovi xatosiz tugadimi. Trek bo'lmasa
	// so'rov kerak emas va qiymat ahamiyatsiz.
	DeliveryOK bool
}

// CollectVerdict - tasniflash natijasi.
type CollectVerdict struct {
	Paid bool
	// Verified - false: yetkazma so'rovi xato berdi, "kelmagan" deb
	// xulosa chiqarib bo'lmaydi. Buyurtma turga kirmaydi va qayta
	// tekshiriladi.
	Verified bool
	Arrived  bool
	Received bool
	Row      *DeliveryOrder
	Findings []Finding
}

// daysBetween - to'liq kunlar soni (kelajakdagi sana 0 bo'ladi).
func daysBetween(t, now time.Time) int {
	d := int(now.Sub(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// parseOrderTime - sanani o'qiydi. Bo'sh yoki noto'g'ri bo'lsa nil.
func parseOrderTime(s string) *time.Time {
	if t, ok := parseAnyTime(s); ok {
		return &t
	}
	return nil
}

// ageBucket - yoshga qarab guruh: `high` dan oshsa high, `mid` dan oshsa
// mid, `low` dan oshsa low. Hech biri bo'lmasa bo'sh.
func ageBucket(days int, limits []int, names []string) string {
	for i := len(limits) - 1; i >= 0; i-- {
		if days > limits[i] {
			return names[i]
		}
	}
	return ""
}

// ClassifyOrder - buyurtmani muammo turlariga ajratadi. Bitta buyurtma
// bir nechta turga tushishi mumkin.
func ClassifyOrder(in CollectInput, now time.Time) CollectVerdict {
	o := in.Order
	v := CollectVerdict{Paid: IsPaid(o), Verified: true}
	if !v.Paid {
		return v
	}
	hasTrack := trackKey(o.ExpressNum) != ""
	if hasTrack && !in.DeliveryOK {
		// "Tekshira olmadik" va "yetkazmada yo'q" aralashtirilmaydi.
		v.Verified = false
		return v
	}

	chk := compareDashboard(o, in.Rows)
	v.Arrived = chk.Arrived()
	if v.Arrived {
		row := chk.Row
		v.Row = &row
		v.Received = isReceived(row)
	}

	created := parseOrderTime(o.CreatedAt)
	shipped := parseOrderTime(o.ShippedAt)
	add := func(typ, bucket string, at *time.Time, kind string) {
		v.Findings = append(v.Findings, Finding{Type: typ, Bucket: bucket, AnchorAt: at, AnchorKind: kind})
	}

	// Status 3: 5 kundan oshgani muammo, posilka yetkazmada chiqqan bo'lsa
	// ham (panelda yetkazma ustunida ko'rinadi). Yoshroq buyurtma keyin
	// refreshPurchasedBuckets orqali o'tadi.
	if o.Status == StatusPaid && created != nil {
		if b := ageBucket(daysBetween(*created, now), []int{5, 20, 30}, []string{"5", "20", "30"}); b != "" {
			add(FindPurchased, b, created, AnchorCreated)
		}
	}

	// Status 12: hammasi, kun buyurtma yaratilganidan hisoblanadi.
	if o.Status == StatusAbnormal {
		add(FindAbnormal, "", created, AnchorCreated)
	}

	if !v.Arrived {
		if shipped != nil {
			if b := ageBucket(daysBetween(*shipped, now), []int{20, 30}, []string{"20", "30"}); b != "" {
				add(FindOnWay, b, shipped, AnchorShipped)
			}
		}
		if o.Status == StatusFinished {
			at, kind := shipped, AnchorShipped
			if at == nil {
				at, kind = created, AnchorCreated
			}
			if at != nil && daysBetween(*at, now) > CollectLateDays {
				add(FindStatus6, "", at, kind)
			}
		}
		if collectOtherStatus(o.Status) && created != nil && daysBetween(*created, now) > CollectLateDays {
			add(FindOther, "", created, AnchorCreated)
		}
	}

	if v.Arrived {
		if chk.Mismatch {
			add(FindWrong, "", nil, "")
		}
		// Mijoz olgan tovar manzil yoki kuryer muammosi emas.
		if !v.Received {
			addressBad := collectAddressBad(o, chk.Row)
			if addressBad {
				add(FindAddress, "", nil, "")
			}
			if !addressBad && !chk.Mismatch {
				if at := courierOverdue(chk.Row, now); at != nil {
					add(FindCourier, "", at, AnchorDelivery)
				}
			}
		}
	}
	return v
}

// collectOtherStatus - "boshqa" turiga kiradigan status: alohida
// ko'rsatiladigan 3, 6, 12 va jo'natilmaydigan taqiqlangan tovardan boshqa.
func collectOtherStatus(status int) bool {
	switch status {
	case StatusPaid, StatusFinished, StatusAbnormal, StatusBanned:
		return false
	}
	return true
}

// collectAddressBad - posilka mijoz belgilagan viloyatga tushmagan:
// adminka manzili yoki yetkazmadagi shahar filial bilan mos emas.
func collectAddressBad(o AdminkaOrder, row DeliveryOrder) bool {
	if row.InLocker() {
		return false
	}
	if ok, _, _, _ := checkRegion(o, row); !ok {
		return true
	}
	region, branch := RegionOf(row.City), RegionOf(row.BranchName)
	return region != "" && branch != "" && region != branch
}

// courierOverdue - posilka kuryer ishlaydigan viloyatdagi punktda muddatdan
// ortiq turibdi va kuryerga berilmagan. Mantiq BriefDelivery dagi
// courierOverdueNote shoxi bilan bir xil. Holat bo'lmasa nil.
func courierOverdue(row DeliveryOrder, now time.Time) *time.Time {
	if row.Delivered || row.InLocker() || expressLineKind(row.ExpressLine) == "pickup" {
		return nil
	}
	if row.Status == 8 && expressLineKind(row.ExpressLine) == "delivery" {
		return nil // kuryer olgan, yo'lda
	}
	branchRegion := RegionOf(row.BranchName)
	central := strings.EqualFold(strings.TrimSpace(row.BranchName), centralWarehouseBranch)
	if central || (branchRegion == "" && !isPickupBranch(row.LocationNumber)) {
		return nil // hali punktga yetmagan
	}
	region := firstNonEmpty(RegionOf(row.City), branchRegion)
	if region != "" && !HomeDeliveryRegion(region) {
		return nil // bu viloyatda kuryer yo'q
	}
	at := parseOrderTime(row.CreatedAt)
	if at == nil || daysBetween(*at, now) <= DeliveryDays {
		return nil
	}
	return at
}
