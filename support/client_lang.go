// Mijozning tili — bir marta aniqlanib, bazada saqlanadi.
//
// Nega kerak: til har bir murojaatda qaytadan "topilardi". AI zanjirida
// uni 1-promt aniqlar (`uzb`/`rus`), xodim javobi yo'lida esa (5-promt)
// hech kim aniqlamas — model suhbat tarixiga qarab o'zi taxmin qilardi.
// Shuning uchun rus tilida yozib yurgan mijoz xodim javobini birdan
// o'zbekcha olardi: tarixning oxirida bizning o'zbekcha xabarimiz turgan
// bo'lsa, model o'shanga ergashardi.
//
// Endi til MIJOZGA biriktiriladi: bir marta aniqlangach bazada qoladi va
// HAMMA yo'lga (AI zanjiri ham, xodim javobi ham) bir xil JSON bo'lib
// uzatiladi. Mijoz tilni o'zgartirsa — yangi aniqlangan til ustiga
// yoziladi.
//
// Promtga qo'shimcha matn YOZILMAYDI: til faqat ma'lumot sifatida,
// JSON ichida boradi. Qanday yozish kerakligi bazadagi promtlarda
// aytilgan.
package support

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

// ClientLang - mijoz bilan qaysi tilda yozishamiz.
type ClientLang struct {
	ClientID  int64     `gorm:"primaryKey" json:"client_id"`
	Lang      string    `gorm:"size:8;not null" json:"lang"` // uzb | rus
	Script    string    `gorm:"size:8" json:"script"`        // lotin | kirill (uzb uchun)
	UpdatedAt time.Time `json:"updated_at"`
}

// Til nomlari — bazada va JSON da bir xil yoziladi.
const (
	LangUzb   = "uzb"
	LangRus   = "rus"
	ScriptLat = "lotin"
	ScriptCyr = "kirill"
)

// SaveClientLang - aniqlangan tilni mijozga biriktiradi.
//
// `sample` — mijozning o'z matni: o'zbekchaning alifbosi shundan
// aniqlanadi (lotin yoki kirill).
func SaveClientLang(clientID int64, uzb, rus bool, sample string) {
	if DB == nil || clientID == 0 || uzb == rus {
		// uzb == rus: model ikkalasini ham (yoki hech birini) belgilagan —
		// bu til aniqlanmagani, eski qiymatni buzmaymiz.
		return
	}
	row := ClientLang{ClientID: clientID, Lang: LangRus}
	if uzb {
		row.Lang = LangUzb
		row.Script = scriptOf(sample)
	}
	// Upsert: mijoz uchun yozuv bor-yo'qligini oldindan bilmaymiz
	// (Save faqat yangilashga urinishi mumkin).
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"lang", "script", "updated_at"}),
	}).Create(&row).Error; err != nil {
		log.Printf("mijoz tili: %d saqlanmadi: %v", clientID, err)
	}
}

// LoadClientLang - saqlangan til (topilmasa bo'sh ClientLang).
func LoadClientLang(clientID int64) ClientLang {
	var row ClientLang
	if DB == nil || clientID == 0 {
		return row
	}
	if err := DB.First(&row, "client_id = ?", clientID).Error; err != nil {
		return ClientLang{}
	}
	return row
}

// scriptOf - o'zbekcha matn qaysi alifboda.
func scriptOf(text string) string {
	if greetLangOf(text) == langUzCyr {
		return ScriptCyr
	}
	return ScriptLat
}

// detectLang - mijozning oxirgi xabarlaridan tilni aniqlaydi.
//
// Bazada hali hech narsa bo'lmaganda ishlatiladi: zanjirning BIRINCHI
// bosqichi ham tilni bilib tursin (model o'zi aniqlaguncha).
func detectLang(msgs []Message) ClientLang {
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].FromClient() {
			continue
		}
		txt := strings.TrimSpace(msgs[i].Message)
		if txt == "" || isImageLink(txt) {
			continue
		}
		switch greetLangOf(txt) {
		case langRU:
			return ClientLang{Lang: LangRus}
		case langUzCyr:
			return ClientLang{Lang: LangUzb, Script: ScriptCyr}
		default:
			return ClientLang{Lang: LangUzb, Script: ScriptLat}
		}
	}
	return ClientLang{}
}

// ClientLangJSON - promtga ketadigan til qatori: "Til: {...}".
//
// Avval bazadagi qiymat, u yo'q bo'lsa mijozning oxirgi xabaridan
// aniqlangani. Hech biri bo'lmasa bo'sh satr — qator umuman qo'shilmaydi.
func ClientLangJSON(clientID int64, msgs []Message) string {
	row := LoadClientLang(clientID)
	if row.Lang == "" {
		row = detectLang(msgs)
	}
	if row.Lang == "" {
		return ""
	}
	return langJSON(row.Lang == LangUzb, row.Lang == LangRus, row.Script)
}

// langJSON - tilning promtdagi ko'rinishi. Maydon nomlari 1-promt
// qaytaradigan javob bilan bir xil ("uzb"/"rus") — model uni tanish
// shaklda ko'radi.
func langJSON(uzb, rus bool, script string) string {
	m := map[string]any{"uzb": uzb, "rus": rus}
	if uzb && script != "" {
		m["alifbo"] = script
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// SetClientLang - tilni QO'LDA (paneldan) belgilaydi.
//
// Nega kerak: model tilni har doim to'g'ri aniqlamaydi (mijoz ruscha
// yozib, bir xabarni o'zbekcha yozsa — til almashib ketishi mumkin).
// Admin paneldan to'g'rilaganda shu funksiya ishlaydi: SaveClientLang
// bilan bir xil yozuvga yozadi, demak AI zanjiri ham, xodim javobi ham
// darhol yangi tilni ko'radi.
//
// `lang` — "uzb" yoki "rus". `script` faqat uzb uchun ("lotin"/"kirill",
// bo'sh bo'lsa lotin). Rus uchun alifbo tozalanadi.
func SetClientLang(clientID int64, lang, script string) (ClientLang, error) {
	if DB == nil {
		return ClientLang{}, errors.New("baza ulanmagan")
	}
	if clientID <= 0 {
		return ClientLang{}, errors.New("client_id noto'g'ri")
	}
	row := ClientLang{ClientID: clientID, Lang: strings.ToLower(strings.TrimSpace(lang))}
	switch row.Lang {
	case LangRus:
		row.Script = ""
	case LangUzb:
		switch strings.ToLower(strings.TrimSpace(script)) {
		case ScriptCyr:
			row.Script = ScriptCyr
		case ScriptLat, "":
			row.Script = ScriptLat
		default:
			return ClientLang{}, errors.New("alifbo faqat lotin yoki kirill bo'ladi")
		}
	default:
		return ClientLang{}, errors.New("til faqat uzb yoki rus bo'ladi")
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"lang", "script", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return ClientLang{}, err
	}
	return row, nil
}
