package escalation

import (
	"database/sql"
	"time"
)

// Item — xodimlar guruhiga yuborilgan bitta murojaat.
type Item struct {
	TgMessageID    int64     `json:"tg_message_id"`
	ConversationID int64     `json:"conversation_id"`
	ClientName     string    `json:"client_name"`
	Question       string    `json:"question"`
	CreatedAt      time.Time `json:"created_at"`
	Resolved       bool      `json:"resolved"`
	Answer         string    `json:"answer"`
}

// Store — Postgres bilan ishlaydi.
type Store struct {
	db *sql.DB
}

// New yangi Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add yangi murojaatni qo'shadi.
func (s *Store) Add(it Item) error {
	_, err := s.db.Exec(
		`INSERT INTO escalations
		 (tg_message_id, conversation_id, client_name, question)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (tg_message_id) DO NOTHING`,
		it.TgMessageID, it.ConversationID, it.ClientName, it.Question)
	return err
}

// Get reply qilingan tg message_id bo'yicha murojaatni topadi.
func (s *Store) Get(tgMessageID int64) (*Item, bool) {
	var it Item
	err := s.db.QueryRow(
		`SELECT tg_message_id, conversation_id, client_name, question,
		        created_at, resolved, COALESCE(answer,'')
		 FROM escalations WHERE tg_message_id=$1`, tgMessageID).
		Scan(&it.TgMessageID, &it.ConversationID, &it.ClientName, &it.Question,
			&it.CreatedAt, &it.Resolved, &it.Answer)
	if err != nil {
		return nil, false
	}
	return &it, true
}

// Resolve murojaatni hal qilingan deb belgilaydi.
func (s *Store) Resolve(tgMessageID int64, answer string) error {
	_, err := s.db.Exec(
		`UPDATE escalations SET resolved=true, answer=$2 WHERE tg_message_id=$1`,
		tgMessageID, answer)
	return err
}
