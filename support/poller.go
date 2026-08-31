package support

import (
	"context"
	"errors"
	"log"
	"sort"
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
//  4. ENG YANGISIDAN boshlab `batch_size` tasi ishlanadi. Qolganlari
//     yo'qolmaydi — keyingi siklda navbat bilan olinadi.
func PollOnce(ctx context.Context) error {
	if !AgentEnabled() {
		return ErrAgentDisabled
	}
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return err
	}

	pages := envInt("CHATS_PAGES", DefaultChatsPages)
	limit := envInt("CHATS_LIMIT", DefaultChatsLimit)

	chats, err := FetchAllChats(creds.BaseURL, token, pages, limit)
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			chats, err = FetchAllChats(creds.BaseURL, token, pages, limit)
		}
	}
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

// pendingChats — javobsiz va hali ishlanmagan suhbatlar, eng yangisidan
// eskisiga qarab saralangan.
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

	// Eng yangi xabar birinchi bo'lsin: kutib qolgan mijoz tezroq javob olsin.
	sort.Slice(out, func(i, j int) bool { return out[i].MsCreatedAt > out[j].MsCreatedAt })
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
