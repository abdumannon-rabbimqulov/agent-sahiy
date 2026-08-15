package models

import (
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

// Prompt kalitlari.
const (
	// PromptBase — agentning asosiy ko'rsatmasi.
	PromptBase = "base"
	// PromptClassify — router: mijoz savolini kategoriyaga ajratadi.
	PromptClassify = "classify"
	// PromptSummarize — xodimlar guruhi uchun xulosa.
	PromptSummarize = "summarize"
	// PromptCatPrefix — kategoriya bilimlari: "cat:yetkazib-berish".
	PromptCatPrefix = "cat:"

	// Quyidagilar — system prompt ichiga qo'shiladigan bloklar. Ular ham
	// bazada yotadi: kodda birorta prompt matni saqlanmaydi.

	// PromptBlockCategory — kategoriya bilimlari qo'shilayotgandagi ko'rsatma.
	// Matn ichida {{CATEGORY}} bo'lsa, kategoriya bilimlari o'sha joyga qo'yiladi.
	PromptBlockCategory = "block:category"
	// PromptBlockOrder — tizimdan olingan buyurtma ma'lumoti bloki.
	// {{ORDERS}} o'rniga buyurtmalar ro'yxati qo'yiladi.
	PromptBlockOrder = "block:order"
	// PromptBlockImage — mijoz rasm yuborgan, buyurtma ma'lumoti YO'Q.
	PromptBlockImage = "block:image"
	// PromptBlockImageOrder — mijoz rasm yuborgan, buyurtma ma'lumoti BOR.
	PromptBlockImageOrder = "block:image_order"
)

// RequiredPrompts — agent ishlashi uchun bazada bo'lishi SHART bo'lgan
// promptlar. Bittasi yo'q bo'lsa agent ishga tushmaydi (kodda zaxira matn
// yo'q — barcha promptlar Postgres'da).
var RequiredPrompts = []string{PromptBase, PromptClassify, PromptSummarize}

// OptionalPrompts — bo'lmasa tegishli blok qo'shilmaydi, agent ishlayveradi.
var OptionalPrompts = []string{
	PromptBlockCategory, PromptBlockOrder, PromptBlockImage, PromptBlockImageOrder,
}

// Prompt — bazada saqlanadigan prompt. Dashboarddan tahrirlanadi va
// o'zgarish darhol kuchga kiradi (agent qayta ishga tushirilmaydi).
type Prompt struct {
	Key       string    `gorm:"primaryKey;size:100" json:"key"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Version   int       `gorm:"not null;default:1" json:"version"`
	Enabled   bool      `gorm:"not null;default:true;index" json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PromptHistory — promptning eski nusxasi (rollback uchun).
type PromptHistory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"index;size:100" json:"key"`
	Content   string    `gorm:"type:text" json:"content"`
	Version   int       `json:"version"`
	ChangedAt time.Time `json:"changed_at"`
}

// PromptChanged — prompt o'zgarganda chaqiriladigan funksiya (keshni
// yangilash uchun). prompts.Store ishga tushganda o'rnatadi.
//
// models paketi prompts paketini import qila olmaydi (aylanma bog'lanish
// bo'lardi), shuning uchun bog'lanish shu o'zgaruvchi orqali.
var PromptChanged func()

// notifyPromptChanged — keshni yangilashni so'raydi (o'rnatilmagan bo'lsa
// hech narsa qilmaydi).
func notifyPromptChanged() {
	if PromptChanged != nil {
		PromptChanged()
	}
}

// BeforeUpdate — matn o'zgargan bo'lsa versiyani oshiradi va ESKI matnni
// tarixga yozadi. Shu tufayli dashboarddan istalgan versiyaga qaytish mumkin.
func (p *Prompt) BeforeUpdate(tx *gorm.DB) error {
	if p.Key == "" {
		return nil // ommaviy update — tarix yozilmaydi
	}
	var old Prompt
	err := tx.Session(&gorm.Session{NewDB: true}).
		Where("key = ?", p.Key).Limit(1).Find(&old).Error
	if err != nil || old.Key == "" {
		return err
	}
	if old.Content == p.Content {
		return nil // matn o'zgarmagan — versiya ham o'zgarmaydi
	}
	if err := tx.Session(&gorm.Session{NewDB: true}).Create(&PromptHistory{
		Key:       old.Key,
		Content:   old.Content,
		Version:   old.Version,
		ChangedAt: time.Now(),
	}).Error; err != nil {
		return err
	}
	p.Version = old.Version + 1
	return nil
}

// AfterSave — yozilgandan keyin kesh yangilanadi.
func (p *Prompt) AfterSave(*gorm.DB) error {
	notifyPromptChanged()
	return nil
}

// AfterDelete — o'chirilgandan keyin ham kesh yangilanadi.
func (p *Prompt) AfterDelete(*gorm.DB) error {
	notifyPromptChanged()
	return nil
}

// CatKey — kategoriya slug'idan prompt kaliti yasaydi.
func CatKey(slug string) string { return PromptCatPrefix + slug }

// Slugify nomdan kalit yasaydi: "Yetkazib berish" → "yetkazib-berish".
// O'zbekcha apostrof va boshqa belgilar tashlab yuboriladi.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// Faqat ASCII harflar va raqamlar kalitda qoladi; qolgani
			// (kirill, apostrof) tashlanadi.
			if r < 128 {
				b.WriteRune(r)
				dash = false
			}
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
