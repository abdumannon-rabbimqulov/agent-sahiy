// Yig'ish rejimining jadvallari va promti.
package support

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// CollectChat holatlari.
const (
	CollectStateDone      = "done"       // raqamlari tekshirildi
	CollectStateNoNumbers = "no_numbers" // matnda ham, rasmda ham raqam yo'q
	CollectStatePartial   = "partial"    // biror so'rov xato berdi, qayta ko'riladi
	CollectStateError     = "error"      // suhbat tarixi o'qilmadi, qayta ko'riladi
)

// CollectSourceAccount - mijoz bu raqamni aytmagan: yozgan mijozning status 3
// yoki 12 dagi tovari sifatida akkauntidan olingan.
const CollectSourceAccount = "akkaunt"

// CollectChat - bitta suhbat qanday o'rganilgani.
type CollectChat struct {
	ConversationID int64  `gorm:"primaryKey;autoIncrement:false" json:"conversation_id"`
	ClientID       int64  `gorm:"index" json:"client_id"`
	LastMessageID  int64  `json:"last_message_id"`
	LastMessageAt  string `gorm:"size:64" json:"last_message_at"`
	State          string `gorm:"size:16;index" json:"state"`
	Numbers        string `gorm:"type:text" json:"numbers"`
	NotFound       string `gorm:"type:text" json:"not_found,omitempty"`
	OrdersCount    int    `json:"orders_count"`
	ImagesRead     int    `json:"images_read"`
	LastText       string `gorm:"type:text" json:"last_text"`
	// Dismissed - xodim raqamsiz murojaatni hisobotdan o'chirgan. Mijoz yangi
	// xabar yozsa suhbat qayta o'rganiladi va yana chiqadi.
	Dismissed bool `gorm:"not null;default:false" json:"dismissed"`
	// ClientWroteAt - mijozning oxirgi xabari qachon yozilgan.
	ClientWroteAt *time.Time `gorm:"index" json:"client_wrote_at"`
	AIType        string     `gorm:"size:32;index" json:"ai_type"`
	AINote        string     `gorm:"type:text" json:"ai_note"`
	Attempts      int        `json:"attempts"`
	Error         string     `gorm:"type:text" json:"error,omitempty"`
	DurationMS    int64      `json:"duration_ms"`
	PromptTokens  int        `json:"prompt_tokens"`
	OutputTokens  int        `json:"output_tokens"`
	ProcessedAt   *time.Time `gorm:"index" json:"processed_at"`
}

// CollectOrder - o'rganilgan buyurtmaning oxirgi holati.
type CollectOrder struct {
	OrderSN        string     `gorm:"primaryKey;size:64" json:"order_sn"`
	UserID         int64      `gorm:"index" json:"user_id"`
	ClientID       int64      `json:"client_id"`
	ConversationID int64      `gorm:"index" json:"conversation_id"`
	Source         string     `gorm:"size:8" json:"source"` // matn, rasm yoki akkaunt
	Status         int        `gorm:"index" json:"status"`
	StatusLabel    string     `gorm:"size:128" json:"status_label"`
	PayStatus      int        `json:"pay_status"`
	ExpressNum     string     `gorm:"size:64" json:"express_num"`
	OrderCreatedAt *time.Time `json:"order_created_at"`
	ShippedAt      *time.Time `json:"shipped_at"`
	Province       string     `gorm:"size:128" json:"province"`
	Verified       bool       `gorm:"index" json:"verified"`
	Arrived        bool       `json:"arrived"`
	Received       bool       `json:"received"`
	Delivered      bool       `json:"delivered"`
	Branch         string     `gorm:"size:255" json:"branch"`
	// ArrivedAt - yetkazmada yozuv paydo bo'lgan sana: tovar O'zbekistonga
	// kelgan kun. DeliveredAt - kuryerga berilgan yoki mijoz olgan kun.
	ArrivedAt   *time.Time `json:"arrived_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
	AIType      string     `gorm:"size:32;index" json:"ai_type"`
	AINote      string     `gorm:"type:text" json:"ai_note"`
	CheckedAt   time.Time  `gorm:"index" json:"checked_at"`
}

// CollectFinding - buyurtmaning bitta muammo turi. Kun soni o'qish
// paytida AnchorAt dan hisoblanadi.
type CollectFinding struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	OrderSN    string     `gorm:"size:64;uniqueIndex:idx_collect_finding" json:"order_sn"`
	Type       string     `gorm:"size:32;uniqueIndex:idx_collect_finding;index" json:"type"`
	Bucket     string     `gorm:"size:8" json:"bucket"`
	UserID     int64      `gorm:"index" json:"user_id"`
	AnchorAt   *time.Time `json:"anchor_at"`
	AnchorKind string     `gorm:"size:32" json:"anchor_kind"`
}

// CollectDismissed - xodim hisobotdan o'chirgan buyurtma. Qayta tekshiruv yoki
// yangi yurish uni qaytarmasligi uchun saqlanadi.
type CollectDismissed struct {
	OrderSN     string    `gorm:"primaryKey;size:64" json:"order_sn"`
	DismissedAt time.Time `json:"dismissed_at"`
}

// CollectRun - bitta yurish.
type CollectRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Errors     int        `json:"errors"`
	Max        int        `json:"max"`
	Note       string     `gorm:"type:text" json:"note"`
}

// CollectPromtTitle - yig'ish promti shu nom bilan topiladi.
const CollectPromtTitle = "8. Yig'ish (mijozga javob yozilmaydi)"

// collectPromtText - boshlang'ich matn. Bazaga yozilgach paneldan
// tahrirlanadi va keyin kod uni qayta yozmaydi.
const collectPromtText = `Sen Sahiy Market murojaatlarini tasniflovchisan. Mijozga javob YOZMAYSAN.
Senga oxirgi xabarlar va tizim faktlari beriladi. Faktlarni o'zgartirma, kun hisoblama.

Faktlardagi HAR BIR order_sn uchun alohida xulosa yoz, birortasini tashlab ketma.
Buyurtma bo'lmasa bitta xulosa yoz, order_sn: null. Har xulosa uchun bitta tur tanla:
- sotib_olingan: status 3, buyurtma Xitoyda sotib olingan va kutilmoqda
- status_12: status 12, g'ayritabiiy holat
- yolda: posilka yo'lga chiqqan, hali kelmagan
- kelmagan: mijoz tovar kelmayotganidan shikoyat qilmoqda
- xato_kelgan: mijoz boshqa yoki noto'g'ri tovar kelganini aytmoqda, yoki faktda egasi mos emas
- kuryer_olmagan: tovar kelgan, kuryer olib kelmagan yoki bog'lanmagan
- manzilga_yetmagan: posilka mijoz belgilagan manzilga yoki viloyatga yetmagan
- boshqa: boshqa savol yoki muammo
- muammo_yoq: mijoz faqat so'ragan yoki minnatdorchilik bildirgan, muammo ko'rinmaydi

"manba": "akkaunt" bo'lgan buyurtmani mijoz aytmagan: u haqda shikoyat bo'lmasa muammo_yoq yoki statusiga mos turni tanla.
xato_kelgan, kuryer_olmagan va manzilga_yetmagan turlarini faqat mijoz shuni aniq aytgan
bo'lsa yoki faktlarda bo'lsa tanla. Taxmin qilma.
Izoh o'zbekcha, bir jumla, mijozning gapiga tayansin.

Faqat JSON qaytar:
{"xulosalar":[{"order_sn":"DG... yoki null","tur":"...","izoh":"..."}]}`

// collectAITypes - AI qaytarishi mumkin bo'lgan turlar.
var collectAITypes = map[string]bool{
	"sotib_olingan": true, "status_12": true, "yolda": true, "kelmagan": true,
	"xato_kelgan": true, "kuryer_olmagan": true, "manzilga_yetmagan": true,
	"boshqa": true, "muammo_yoq": true,
}

// ensureCollectPromt - promt yo'q bo'lsa yaratadi, bor bo'lsa tegmaydi.
func ensureCollectPromt(db *gorm.DB) error {
	var p Promt
	err := db.Where("title = ?", CollectPromtTitle).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&Promt{Title: CollectPromtTitle, Promt: collectPromtText}).Error
	}
	return err
}
