package support

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// Poller sozlamalari (.env).
const (
	DefaultPollInterval = 60 // sekund
	DefaultChatsLimit   = 30
)

// StartPoller fon siklini ishga tushiradi: har `POLL_INTERVAL_SEC` da
// yangi mijoz xabari bo'lgan suhbatlarni topib, zanjirni yuritadi.
// Sikl `poll_enabled` sozlamasi bilan panel orqali to'xtatiladi.
func StartPoller(ctx context.Context) {
	interval := time.Duration(envInt("POLL_INTERVAL_SEC", DefaultPollInterval)) * time.Second
	log.Printf("poller: har %s da yangi xabarlar tekshiriladi", interval)

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("poller: to'xtadi")
				return
			case <-t.C:
				if !PollEnabled() {
					continue
				}
				if err := PollOnce(ctx); err != nil {
					log.Printf("poller: %v", err)
				}
			}
		}
	}()
}

// PollOnce bitta siklni bajaradi: suhbatlarni oladi, yangi mijoz xabari
// borlarini ajratadi va zanjirni yuritadi. Tezlik chegarasi:
// RATE_LIMIT_COUNT — bitta siklda ko'pi bilan nechta suhbat ishlanadi
// (qolganlari keyingi siklda navbat bilan olinadi).
func PollOnce(ctx context.Context) error {
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return err
	}
	f := ChatFilter{Limit: envInt("CHATS_LIMIT", DefaultChatsLimit)}
	chats, err := FetchChats(creds.BaseURL, token, f)
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			chats, err = FetchChats(creds.BaseURL, token, f)
		}
	}
	if err != nil {
		return err
	}

	limit := envInt("RATE_LIMIT_COUNT", 5)
	done := 0

	for _, c := range chats {
		if done >= limit {
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
			continue
		}

		if _, err := RunChain(ctx, c.ID, c.ClientID); err != nil {
			log.Printf("poller: suhbat %d zanjiri: %v", c.ID, err)
		}
		markHandled(c.ID, c.ClientID, last)
		done++
	}

	if done > 0 {
		log.Printf("poller: %d ta suhbat ishlandi", done)
	}
	return nil
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
func markHandled(conversationID, clientID int64, last Message) {
	now := time.Now()
	st := ConversationState{
		ConversationID: conversationID,
		ClientID:       clientID,
		LastMessageID:  last.ID,
		LastMessageAt:  last.CreatedAt,
		LastHandledAt:  &now,
		UpdatedAt:      now,
	}
	if err := DB.Save(&st).Error; err != nil {
		log.Printf("poller: holatni yozish (%d): %v", conversationID, err)
	}
}
