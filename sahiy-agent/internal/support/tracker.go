package support

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"sahiy-agent/internal/models"
)

// Tracker har bir suhbat bo'yicha oxirgi javob berilgan mijoz xabarini
// eslab qoladi. Shu sabab eski suhbatga kelgan yangi xabar ham seziladi.
type Tracker struct {
	db *gorm.DB
}

// NewTracker yangi tracker.
func NewTracker(db *gorm.DB) *Tracker {
	return &Tracker{db: db}
}

// Count kuzatilayotgan suhbatlar soni (0 bo'lsa — birinchi ishga tushish).
func (t *Tracker) Count() (int64, error) {
	var n int64
	err := t.db.Model(&models.ConversationState{}).Count(&n).Error
	return n, err
}

// Candidates tekshirishga arziydigan suhbatlarni ajratadi: operatorda
// o'qilmagan xabar bo'lsa yoki suhbat hali umuman ko'rilmagan bo'lsa.
func (t *Tracker) Candidates(chats []Conversation) []Conversation {
	seen := t.knownIDs(chats)
	var out []Conversation
	for _, c := range chats {
		if c.OperatorUnseenCount > 0 || !seen[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// Handled true qaytarsa — bu xabarga allaqachon javob berilgan.
func (t *Tracker) Handled(conversationID, lastMessageID int64) bool {
	var st models.ConversationState
	err := t.db.First(&st, "conversation_id = ?", conversationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		return false
	}
	return lastMessageID > 0 && st.LastMessageID >= lastMessageID
}

// Commit suhbat bo'yicha oxirgi ko'rilgan mijoz xabarini saqlaydi (upsert).
func (t *Tracker) Commit(conversationID, lastMessageID int64) error {
	return t.db.Save(&models.ConversationState{
		ConversationID: conversationID,
		LastMessageID:  lastMessageID,
		UpdatedAt:      time.Now(),
	}).Error
}

// knownIDs berilgan chatlardan qaysilari bazada bor ekanini bitta so'rovda oladi.
func (t *Tracker) knownIDs(chats []Conversation) map[int64]bool {
	ids := make([]int64, 0, len(chats))
	for _, c := range chats {
		ids = append(ids, c.ID)
	}
	seen := map[int64]bool{}
	if len(ids) == 0 {
		return seen
	}
	var found []int64
	t.db.Model(&models.ConversationState{}).
		Where("conversation_id IN ?", ids).
		Pluck("conversation_id", &found)
	for _, id := range found {
		seen[id] = true
	}
	return seen
}
