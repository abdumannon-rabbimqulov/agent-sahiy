// Fon sikli: javobsiz suhbatlarni topib, har biri uchun agent zanjirini
// yuritadi. Panel orqali o'chirib qo'yiladi.
//
// ScanOnce — paneldan qo'lda ishga tushiriladigan to'liq skanerlash:
// filtr va chegarasiz, hamma suhbat ko'riladi.
package support

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// Poller sozlamalari (.env).
const (
	DefaultPollInterval = 60  // sekund
	DefaultChatsLimit   = 100 // bir sahifada nechta suhbat
	DefaultChatsPages   = 6   // nechta sahifa ko'riladi
)

// StartPoller fon siklini ishga tushiradi: har `poll_interval_sec` da
// yangi mijoz xabari bo'lgan suhbatlarni topib, zanjirni yuritadi.
//
// Oraliq HAR AYLANISHDA qaytadan o'qiladi — paneldan tezlikni
// o'zgartirsangiz darhol kuchga kiradi, qayta ishga tushirish shart emas.
// Sikl `poll_enabled` va `agent_enabled` sozlamalari bilan to'xtatiladi.
func StartPoller(ctx context.Context) {
	log.Printf("poller: ishga tushdi (hozirgi oraliq %ds, bir siklda %d suhbat)",
		PollInterval(), BatchSize())

	go func() {
		last := PollInterval()
		for {
			// Sozlama har aylanishda qaytadan o'qiladi; o'zgargani
			// logda ko'rinsin — tezlik nima uchun o'zgargani aniq bo'ladi.
			cur := PollInterval()
			if cur != last {
				log.Printf("poller: oraliq %ds → %ds (bir siklda %d suhbat, tanaffus %ds)",
					last, cur, BatchSize(), ChatDelay())
				last = cur
			}
			wait := time.Duration(cur) * time.Second
			select {
			case <-ctx.Done():
				log.Println("poller: to'xtadi")
				return
			case <-time.After(wait):
			}

			if !AgentEnabled() || !PollEnabled() {
				continue
			}
			if err := PollOnce(ctx); err != nil {
				log.Printf("poller: %v", err)
			}
		}
	}()
}

// PollOnce bitta siklni bajaradi:
//
//  1. Bir necha sahifa suhbat olinadi (server ro'yxatni yangilik bo'yicha
//     saralamaydi — eng yangi xabar oxirgi sahifada bo'lishi mumkin).
//  2. Faqat JAVOBSIZ suhbatlar qoldiriladi: `operator_unseen_count > 0`
//     — ya'ni biz o'qimagan mijoz xabari bor.
//  3. Allaqachon ishlanganlari tashlanadi (oxirgi xabar vaqti bo'yicha) —
//     bu har suhbat uchun alohida so'rovni tejaydi.
//  4. ENG ESKISIDAN (eng ko'p kutgan mijozdan) boshlab `batch_size` tasi
//     ishlanadi. Qolganlari yo'qolmaydi — keyingi siklda navbat bilan
//     olinadi.
func PollOnce(ctx context.Context) error {
	if !AgentEnabled() {
		return ErrAgentDisabled
	}
	pages := envInt("CHATS_PAGES", DefaultChatsPages)
	limit := envInt("CHATS_LIMIT", DefaultChatsLimit)

	chats, err := fetchChats(pages, limit)
	if err != nil {
		return err
	}

	todo := pendingChats(chats)
	if len(todo) == 0 {
		return nil
	}

	batch := BatchSize()
	delay := time.Duration(ChatDelay()) * time.Second
	log.Printf("poller: %d ta javobsiz suhbat, shundan %d tasi ishlanadi",
		len(todo), min(batch, len(todo)))

	done := 0
	for _, c := range todo {
		if done >= batch {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fresh, last, err := hasNewClientMessage(c.ID)
		if err != nil {
			log.Printf("poller: suhbat %d: %v", c.ID, err)
			continue
		}
		if !fresh {
			// Ro'yxat "javobsiz" degan edi, lekin xabarlar bo'yicha
			// oxirgi so'z bizniki — holatni yangilab, o'tib ketamiz.
			markHandled(c.ID, c.ClientID, last, c.MsCreatedAt)
			continue
		}

		if _, err := RunChain(ctx, c.ID, c.ClientID); err != nil {
			// Oxirgi so'z biz tomondan bo'lsa — normal holat, log shart emas.
			if !errors.Is(err, ErrAlreadyAnswered) {
				log.Printf("poller: suhbat %d zanjiri: %v", c.ID, err)
			}
		}
		markHandled(c.ID, c.ClientID, last, c.MsCreatedAt)
		done++

		// Sekinlashtirish: model va tashqi API'larni bosmaslik uchun
		// suhbatlar orasida tanaffus (chat_delay_sec).
		if delay > 0 && done < batch {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
		}
	}

	if done > 0 {
		log.Printf("poller: %d ta suhbat ishlandi", done)
	}

	// Ochiq muammolarni qayta ko'rib chiqamiz: holat o'zgarganmi,
	// mijozga javob berilganmi va eslatma vaqti kelganmi.
	if err := ReviewOpenIssues(DB); err != nil {
		log.Printf("poller: muammolarni ko'rib chiqish: %v", err)
	}
	return nil
}

// pendingChats — javobsiz va hali ishlanmagan suhbatlar, ENG ESKISIDAN
// yangisiga qarab saralangan (FIFO).
//
// Muhim: batch_size bilan cheklangani uchun tartib katta ahamiyatga ega.
// Eng yangisi birinchi bo'lsa, doimiy oqib turgan yangi xabarlar eski,
// uzoq kutgan mijozlarni navbatning oxiriga surib, ular hech qachon
// ishlanmay qolib ketaveradi. Shu sababli eng uzoq kutgan (eng eski
// oxirgi xabarli) suhbat birinchi navbatda ishlanadi.
func pendingChats(chats []Chat) []Chat {
	// Ishlangan suhbatlar holati (bitta so'rovda).
	handled := map[int64]ConversationState{}
	if DB != nil {
		var rows []ConversationState
		if err := DB.Find(&rows).Error; err == nil {
			for _, r := range rows {
				handled[r.ConversationID] = r
			}
		}
	}

	out := make([]Chat, 0, len(chats))
	for _, c := range chats {
		if !c.Unanswered() {
			continue
		}
		st, ok := handled[c.ID]
		if ok {
			if st.Skip {
				continue
			}
			// Oxirgi xabar vaqti o'zgarmagan bo'lsa — yangi xabar yo'q.
			if c.MsCreatedAt != "" && c.MsCreatedAt == st.LastMessageAt {
				continue
			}
		}
		out = append(out, c)
	}

	// Eng eski xabar birinchi bo'lsin: eng ko'p kutgan mijoz navbatning
	// boshida turadi, batch_size tugab qolsa ham u shu siklda ishlanadi.
	sort.Slice(out, func(i, j int) bool { return out[i].MsCreatedAt < out[j].MsCreatedAt })
	return out
}

// hasNewClientMessage suhbatda javob berilmagan yangi mijoz xabari bormi.
// Oxirgi xabar mijozdan bo'lishi va uning id'si bazadagidan katta bo'lishi kerak.
func hasNewClientMessage(conversationID int64) (bool, Message, error) {
	msgs, err := fetchHistory(conversationID)
	if err != nil {
		return false, Message{}, err
	}
	if len(msgs) == 0 {
		return false, Message{}, nil
	}
	last := msgs[len(msgs)-1]
	if !last.FromClient() {
		return false, last, nil // oxirgi so'z xodimniki — javob kutilmayapti
	}

	var st ConversationState
	err = DB.First(&st, "conversation_id = ?", conversationID).Error
	if err == gorm.ErrRecordNotFound {
		return true, last, nil // birinchi marta ko'rilmoqda
	}
	if err != nil {
		return false, last, err
	}
	if st.Skip {
		return false, last, nil
	}
	return last.ID > st.LastMessageID, last, nil
}

// markHandled suhbat qayergacha ishlanganini yozadi.
// msAt — ro'yxatdagi oxirgi xabar vaqti: keyingi siklda shu bilan
// solishtirib, xabarlarni qayta so'ramasdan o'tib ketamiz.
func markHandled(conversationID, clientID int64, last Message, msAt string) {
	now := time.Now()
	if msAt == "" {
		msAt = last.CreatedAt
	}
	st := ConversationState{
		ConversationID: conversationID,
		ClientID:       clientID,
		LastMessageID:  last.ID,
		LastMessageAt:  msAt,
		LastHandledAt:  &now,
		UpdatedAt:      now,
	}
	if err := DB.Save(&st).Error; err != nil {
		log.Printf("poller: holatni yozish (%d): %v", conversationID, err)
	}
}

// ScanResult - skanerlash natijasi (logga va API javobiga).
type ScanResult struct {
	Chats    int `json:"chats"`    // ro'yxatdan olingan suhbatlar
	Ran      int `json:"ran"`      // zanjir yurgan (modelga borgan)
	Answered int `json:"answered"` // oxirgi so'z bizniki — o'tkazib yuborilgan
	Failed   int `json:"failed"`   // xato bilan tugagan
}

// scanning - bir vaqtda faqat bitta skanerlash yursin.
var scanning atomic.Bool

// ScanRunning - hozir skanerlash ketyaptimi.
func ScanRunning() bool { return scanning.Load() }

// ScanOnce suhbatlar ro'yxatini (support.chat.conversation/filter) boshidan
// oxirigacha ko'rib chiqadi va har bir mijoz uchun zanjirni KETMA-KET
// yuritadi.
//
// PollOnce'dan farqi: `operator_unseen_count` va "allaqachon ishlangan"
// filtrlari YO'Q — ro'yxatdagi hamma mijoz ko'riladi. Shuning uchun
// `batch_size` ham qo'llanmaydi: chegara faqat `max` (0 — hammasi).
//
// Javob berilgan suhbat modelga BORMAYDI: `RunChain` oxirgi so'z biz
// tomondan bo'lsa `ErrAlreadyAnswered` qaytaradi — token sarflanmaydi.
func ScanOnce(ctx context.Context, pages, limit, max int) (ScanResult, error) {
	var res ScanResult
	if !AgentEnabled() {
		return res, ErrAgentDisabled
	}
	if !scanning.CompareAndSwap(false, true) {
		return res, errors.New("skanerlash allaqachon ketyapti")
	}
	defer scanning.Store(false)

	if pages < 1 {
		pages = envInt("CHATS_PAGES", DefaultChatsPages)
	}
	if limit < 1 {
		limit = envInt("CHATS_LIMIT", DefaultChatsLimit)
	}

	chats, err := fetchChats(pages, limit)
	if err != nil {
		return res, err
	}

	// Eng eski xabar birinchi: eng ko'p kutgan mijoz `max` cheklangan
	// bo'lganda ham navbatning boshida turadi (pendingChats bilan bir xil
	// mantiq — qarang: yuqoridagi izoh).
	sort.Slice(chats, func(i, j int) bool { return chats[i].MsCreatedAt < chats[j].MsCreatedAt })
	res.Chats = len(chats)
	if max > 0 && max < len(chats) {
		chats = chats[:max]
	}
	log.Printf("skaner: %d ta suhbat olindi, %d tasi ko'riladi", res.Chats, len(chats))

	delay := time.Duration(ChatDelay()) * time.Second
	for i, c := range chats {
		select {
		case <-ctx.Done():
			log.Printf("skaner: to'xtatildi (%d ta ko'rildi)", i)
			return res, nil
		default:
		}

		in, err := RunChain(ctx, c.ID, c.ClientID)
		switch {
		case errors.Is(err, ErrAlreadyAnswered):
			res.Answered++
		case err != nil:
			res.Failed++
			log.Printf("skaner: suhbat %d: %v", c.ID, err)
		default:
			res.Ran++
		}
		// Zanjir yurgan bo'lsa holatni yozamiz — poller shu suhbatni
		// qaytadan olmasin.
		if in != nil && err == nil {
			if msgs, e := fetchHistory(c.ID); e == nil && len(msgs) > 0 {
				markHandled(c.ID, c.ClientID, msgs[len(msgs)-1], c.MsCreatedAt)
			}
		}

		if delay > 0 && i < len(chats)-1 {
			select {
			case <-ctx.Done():
				return res, nil
			case <-time.After(delay):
			}
		}
	}

	log.Printf("skaner: tugadi — %d zanjir, %d javob berilgan, %d xato (jami %d)",
		res.Ran, res.Answered, res.Failed, res.Chats)
	return res, nil
}

// fetchChats - bir necha sahifa suhbatni oladi (token eskirsa yangilanadi).
func fetchChats(pages, limit int) ([]Chat, error) {
	return withToken(func(baseURL, token string) ([]Chat, error) {
		return FetchAllChats(baseURL, token, pages, limit)
	})
}
