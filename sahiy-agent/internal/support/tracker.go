package support

import (
	"database/sql"
	"sort"
	"strconv"

	"sahiy-agent/internal/db"
)

const trackerKey = "last_seen_id"

// Tracker oxirgi ko'rilgan eng katta chat ID'sini Postgres kv'da saqlaydi.
type Tracker struct {
	db         *sql.DB
	LastSeenID int64
}

// LoadTracker kv'dan last_seen_id ni o'qiydi.
func LoadTracker(database *sql.DB) *Tracker {
	t := &Tracker{db: database}
	if v, err := db.GetKV(database, trackerKey); err == nil && v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			t.LastSeenID = id
		}
	}
	return t
}

// New chatlardan faqat oldin ko'rilmaganlarini (ID > LastSeenID) qaytaradi.
func (t *Tracker) New(chats []Conversation) []Conversation {
	var fresh []Conversation
	for _, c := range chats {
		if c.ID > t.LastSeenID {
			fresh = append(fresh, c)
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].ID > fresh[j].ID })
	return fresh
}

// Commit ko'rilgan chatlar ichidagi eng katta ID'ni saqlaydi.
func (t *Tracker) Commit(chats []Conversation) error {
	for _, c := range chats {
		if c.ID > t.LastSeenID {
			t.LastSeenID = c.ID
		}
	}
	return db.SetKV(t.db, trackerKey, strconv.FormatInt(t.LastSeenID, 10))
}
