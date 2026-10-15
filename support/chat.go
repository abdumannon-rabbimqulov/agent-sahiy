// Support tizimidagi suhbatlar ro'yxati: sahifalab olish va javobsiz
// qolganlarini ajratish (operator_unseen_count).
package support

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

const ChatsPath = "/api/v1/support.chat.conversation/filter"

// ErrUnauthorized token rad etilganda qaytadi — chaqiruvchi Refresh qiladi.
var ErrUnauthorized = errors.New("token rad etildi (401)")

// Chat — suhbatdan olinadigan maydonlar. Qolgani tashlanadi.
type Chat struct {
	ID          int64  `json:"id"`
	ClientID    int64  `json:"client_id"`
	CreatedAt   string `json:"created_at"`
	MsCreatedAt string `json:"ms_created_at"` // oxirgi xabar vaqti

	// Message — oxirgi xabar matni (ro'yxatda ko'rinadi).
	Message string `json:"message"`
	// OperatorUnseenCount — BIZ o'qimagan mijoz xabarlari soni.
	// Noldan katta bo'lsa suhbat javobsiz qolgan.
	OperatorUnseenCount int `json:"operator_unseen_count"`
	// UnseenCount — mijoz o'qimagan xabarlar soni.
	UnseenCount int `json:"unseen_count"`
	// State, ResolutionState — suhbat holati (1 — ochiq).
	State           int `json:"state"`
	ResolutionState int `json:"resolution_state"`
}

// Unanswered — suhbatda biz o'qimagan (javobsiz) mijoz xabari bormi.
func (c Chat) Unanswered() bool { return c.OperatorUnseenCount > 0 }

// ChatFilter — so'rov shartlari. ClientID 0 bo'lsa hamma suhbatlar keladi,
// noldan katta bo'lsa faqat o'sha mijoz bilan bog'liq suhbatlar.
type ChatFilter struct {
	ClientID int64 `json:"client_id"`
	Page     int   `json:"page"`
	Limit    int   `json:"limit"`
}

// FetchChats suhbatlarni oladi. Server GET qabul qilmaydi (405), shuning uchun
// so'rov POST bilan ketadi — filtr body'da, page/limit query'da.
func FetchChats(baseURL, token string, f ChatFilter) ([]Chat, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 10
	}
	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := fmt.Sprintf("%s%s?page=%d&limit=%d", strings.TrimRight(base, "/"), ChatsPath, f.Page, f.Limit)

	filter := map[string]any{
		"type":  "client",
		"state": []int{1, 2, 3},
	}
	if f.ClientID > 0 {
		filter["client_id"] = f.ClientID
	}
	body, err := json.Marshal(filter)
	if err != nil {
		return nil, fmt.Errorf("body marshal: %w", err)
	}

	raw, err := apiCall{Method: http.MethodPost, URL: url, Token: token,
		Body: body, What: "suhbatlar ro'yxati"}.do()
	if err != nil {
		return nil, err
	}

	var out struct {
		Data struct {
			Chats []Chat `json:"chats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("javobni o'qish: %w", err)
	}
	return out.Data.Chats, nil
}

// FetchAllChats bir necha sahifani yig'ib qaytaradi (1-sahifadan boshlab).
//
// Server ro'yxatni yangilik bo'yicha saralamaydi: eng yangi xabarlar
// oxirgi sahifalarda ham bo'lishi mumkin. Shuning uchun poller bir
// sahifa bilan cheklanmaydi — bir nechta sahifa olinadi va keyin
// o'zimiz saralaymiz.
func FetchAllChats(baseURL, token string, pages, limit int) ([]Chat, error) {
	if pages < 1 {
		pages = 1
	}
	if limit < 1 {
		limit = 100
	}
	seen := map[int64]bool{}
	var all []Chat
	_, err := eachPage(baseURL, token, pages, limit, func(part []Chat) bool {
		all = appendUnique(all, seen, part...)
		return true
	})
	if err != nil && len(all) == 0 {
		return nil, err
	}
	return all, nil // xato bo'lsa ham olingan qism bilan davom etamiz
}

// eachPage 1-sahifadan boshlab `maxPages` tagacha sahifa oladi va har
// biriga fn ni chaqiradi. Ro'yxat tugasa (bo'sh yoki to'liqmas sahifa)
// yoki fn false qaytarsa to'xtaydi. Oxirgi so'ralgan sahifa raqamini
// qaytaradi.
func eachPage(baseURL, token string, maxPages, limit int, fn func([]Chat) bool) (int, error) {
	page := 0
	for page < maxPages {
		page++
		part, err := FetchChats(baseURL, token, ChatFilter{Page: page, Limit: limit})
		if err != nil {
			return page, err
		}
		if len(part) == 0 || !fn(part) || len(part) < limit {
			break
		}
	}
	return page, nil
}

// appendUnique ID bo'yicha takrorlanmaganlarini qo'shadi.
func appendUnique(all []Chat, seen map[int64]bool, chats ...Chat) []Chat {
	for _, c := range chats {
		if !seen[c.ID] {
			seen[c.ID] = true
			all = append(all, c)
		}
	}
	return all
}

// ChatsJSON suhbatlarni tayyor JSON matn qilib qaytaradi:
// {"count": N, "chats": [...]}
func ChatsJSON(baseURL, token string, f ChatFilter) ([]byte, error) {
	chats, err := FetchChats(baseURL, token, f)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Count int    `json:"count"`
		Chats []Chat `json:"chats"`
	}{len(chats), chats}, "", "  ")
}

// DefaultUnansweredMaxPages — FetchUnansweredChats uchun xavfsizlik
// chegarasi: server saralashni o'zgartirib qo'ysa ham butun ro'yxatni
// (31 mingdan ortiq suhbat) o'qib ketmasin.
const DefaultUnansweredMaxPages = 40

// unansweredZeroPages - javobsiz zona tugagan deb hisoblash uchun kerakli
// ketma-ket "javobsizi yo'q" sahifalar soni.
const unansweredZeroPages = 2

// FetchUnansweredChats javobsiz suhbatlarni TO'LIQ qaytaradi.
//
// Server ro'yxatni javobsizlarni oldinga qo'yib saralaydi: `unread_chats`
// soni aynan 1..N sahifalardagi `operator_unseen_count > 0` suhbatlar
// soniga teng (o'lchandi: 9 to'la sahifa + 10-sahifada 18 ta = 918 =
// javobdagi unread_chats). Shuning uchun javobsiz bermagan birinchi
// sahifada to'xtash mumkin — uning orqasida ham javobsiz yo'q.
//
// Nega bu muhim: ilgari poller sahifa oynasini surib borardi va 11-dan
// 314-sahifagacha — 30 mingdan ortiq javob berilgan suhbatni — bekorga
// aylanib chiqardi. Endi har siklda butun navbat (hamma javobsiz)
// qo'lda bo'ladi, ya'ni "eng uzoq kutgan birinchi" saralash tasodifiy
// oyna ichida emas, haqiqiy navbat bo'ylab ishlaydi.
func FetchUnansweredChats(baseURL, token string, maxPages, limit int) ([]Chat, error) {
	if maxPages < 1 {
		maxPages = DefaultUnansweredMaxPages
	}
	if limit < 1 {
		limit = 100
	}

	seen := map[int64]bool{}
	var all []Chat
	// zeroStreak - ketma-ket nechta sahifa javobsiz bermadi. Bitta bo'sh
	// sahifada darhol to'xtamaymiz: server saralashni o'zgartirib qo'ysa
	// (yoki chegara aynan sahifa boshiga tushsa) ish o'tkazib yuborilmasin.
	// Bitta qo'shimcha sahifaning narxi ~1.2s, o'tkazib yuborilgan
	// mijozning narxi esa ancha qimmat.
	zeroStreak := 0
	lastPage, err := eachPage(baseURL, token, maxPages, limit, func(part []Chat) bool {
		n := 0
		for _, c := range part {
			if c.Unanswered() {
				n++
				all = appendUnique(all, seen, c)
			}
		}
		if n > 0 {
			zeroStreak = 0
			return true
		}
		zeroStreak++
		return zeroStreak < unansweredZeroPages // javobsiz zona ishonchli tugadi
	})
	if err != nil && len(all) == 0 {
		return nil, err
	}
	// Chegaraga urilsak — server saralashi o'zgargan bo'lishi mumkin:
	// javobsizlar ro'yxat bo'ylab tarqalgan. Buni jimgina o'tkazib
	// yubormaymiz, aks holda navbatning bir qismi ko'rinmay qoladi.
	//
	// maxPages == 1 bo'lsa bu ogohlantirish emas: navbat ATAYLAB bitta
	// sahifa bilan cheklangan, shuning uchun jim o'tamiz.
	if maxPages > 1 && lastPage >= maxPages {
		log.Printf("suhbatlar: %d sahifa chegarasiga yetildi (%d javobsiz topildi) — "+
			"javobsizlar ro'yxat boshida to'planmagan bo'lishi mumkin, UNANSWERED_MAX_PAGES ni oshirish kerak",
			maxPages, len(all))
	}
	return all, nil
}
