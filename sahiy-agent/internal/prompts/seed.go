package prompts

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// DefaultClassify — router prompti. Uning vazifasi FAQAT kategoriyani
// tanlash: mijoz matnini qayta yozmaydi, xulosa qilmaydi, javob yozmaydi.
// {{CATEGORIES}} o'rniga bazadagi kategoriyalar ro'yxati qo'yiladi.
const DefaultClassify = `Sen — yo'naltiruvchi (router). Mijoz murojaatini kategoriyaga ajratasan.

Mavjud kategoriyalar:
{{CATEGORIES}}

Javobni FAQAT JSON ko'rinishida yoz, boshqa hech narsa yozma:
{"category":"<kalit>","escalate":false}

Qoidalar:
- category — yuqoridagi ro'yxatdagi kalitlardan biri. Hech qaysi mos
  kelmasa: "".
- escalate — mijoz jahli chiqqan, pul qaytarish yoki nizo bo'lsa, yoki
  masala inson xodimini talab qilsa: true. Aks holda false.
- Mijoz matnini TARJIMA QILMA, QAYTA YOZMA, XULOSA QILMA.
- Izoh, sarlavha, markdown yozma. Faqat bitta qator JSON.`

// DefaultSummarize — xodimlar guruhi uchun xulosa prompti.
const DefaultSummarize = `Sen support jamoasiga muammoni tushuntiruvchi yordamchisan.
Quyida mijoz bilan bo'lgan suhbat tarixi berilgan. Uni to'liq o'qib chiq va
navbatchi xodim uchun qisqa xulosa yoz — xodim suhbatni o'qimasdan ham
muammoni tushunishi kerak.

Aynan quyidagi 4 qatorni yoz (o'zbek tilida):
Daraja: <yuqori | o'rta | past>
Muammo: <mijozning umumiy muammosi>
Tafsilot: <muhim faktlar: buyurtma/track raqami, sana, nima urinib ko'rilgan>
Kerak: <xodimdan aniq nima talab qilinadi>

Daraja qanday tanlanadi:
- yuqori: yetkazish muddati o'tgan, buyurtma yo'qolgan yoki shikastlangan,
  pul qaytarish yoki to'lov nizosi, mijoz jahli chiqqan yoki bir necha
  marta javobsiz murojaat qilgan
- o'rta: holat noaniq, tekshirish kerak, lekin shoshilinch emas
- past: oddiy savol yoki ma'lumot yetishmayotgani uchun aniqlik kerak

Boshqa hech narsa yozma. Suhbatda yo'q ma'lumotni o'ylab topma —
bilinmasa "noma'lum" deb yoz.`

// Seed prompts jadvali bo'sh bo'lsa uni to'ldiradi:
//   - "base"      ← prompt.txt (fayl o'chirilmaydi, zaxira bo'lib qoladi)
//   - "classify"  ← DefaultClassify
//   - "summarize" ← DefaultSummarize
//   - "cat:<slug>" ← categories jadvalidagi har bir bo'lim
//
// Mavjud yozuvlarga tegilmaydi — dashboarddagi tahrirlar saqlanib qoladi.
func (s *Store) Seed() error {
	if err := s.seedCategorySlugs(); err != nil {
		return err
	}

	var n int64
	if err := s.db.Model(&models.Prompt{}).Count(&n).Error; err != nil {
		return fmt.Errorf("promptlarni sanash: %w", err)
	}
	if n > 0 {
		return nil // allaqachon to'ldirilgan
	}

	base := s.fallbackBase
	if base == "" {
		base = "Sen Sahiy support xizmatining yordamchi agentisan. Mijozlarga o'zbek tilida qisqa, xushmuomala va aniq javob ber."
	}
	seed := []models.Prompt{
		{Key: models.PromptBase, Content: base, Version: 1, Enabled: true},
		{Key: models.PromptClassify, Content: DefaultClassify, Version: 1, Enabled: true},
		{Key: models.PromptSummarize, Content: DefaultSummarize, Version: 1, Enabled: true},
	}

	var cats []models.Category
	if err := s.db.Find(&cats).Error; err != nil {
		return fmt.Errorf("kategoriyalarni o'qish: %w", err)
	}
	for _, c := range cats {
		if c.Slug == "" || strings.TrimSpace(c.Content) == "" {
			continue
		}
		seed = append(seed, models.Prompt{
			Key: models.CatKey(c.Slug), Content: c.Content, Version: 1, Enabled: c.Active,
		})
	}

	if err := s.db.Create(&seed).Error; err != nil {
		return fmt.Errorf("promptlarni yozish: %w", err)
	}
	log.Printf("✓ %d ta prompt bazaga ko'chirildi (prompt.txt + %d kategoriya)", len(seed), len(seed)-3)
	return nil
}

// seedCategorySlugs slug'i yo'q kategoriyalarga nom asosida slug yozadi.
// Bir xil slug chiqsa oxiriga id qo'shiladi.
func (s *Store) seedCategorySlugs() error {
	var cats []models.Category
	if err := s.db.Where("slug IS NULL OR slug = ''").Find(&cats).Error; err != nil {
		return fmt.Errorf("slug'siz kategoriyalar: %w", err)
	}
	for _, c := range cats {
		slug := models.Slugify(c.Name)
		if slug == "" {
			slug = fmt.Sprintf("kategoriya-%d", c.ID)
		}
		var busy int64
		if err := s.db.Model(&models.Category{}).
			Where("slug = ? AND id <> ?", slug, c.ID).Count(&busy).Error; err != nil {
			return err
		}
		if busy > 0 {
			slug = fmt.Sprintf("%s-%d", slug, c.ID)
		}
		if err := s.db.Model(&models.Category{}).Where("id = ?", c.ID).
			Update("slug", slug).Error; err != nil {
			return fmt.Errorf("slug yozish (%d): %w", c.ID, err)
		}
	}
	return nil
}

// SyncCategory kategoriya qo'shilganda/tahrirlanganda uning promptini
// yangilaydi ("cat:<slug>").
func SyncCategory(db *gorm.DB, c *models.Category) error {
	if c.Slug == "" {
		c.Slug = models.Slugify(c.Name)
	}
	if c.Slug == "" || strings.TrimSpace(c.Content) == "" {
		return nil
	}
	key := models.CatKey(c.Slug)

	var cur []models.Prompt
	if err := db.Where("key = ?", key).Limit(1).Find(&cur).Error; err != nil {
		return err
	}
	if len(cur) == 0 {
		return db.Create(&models.Prompt{
			Key: key, Content: c.Content, Version: 1, Enabled: c.Active,
		}).Error
	}
	p := cur[0]
	p.Content, p.Enabled = c.Content, c.Active
	return db.Save(&p).Error
}
