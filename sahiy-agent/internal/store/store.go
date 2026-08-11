package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Interaction — AI bir marta kim bilan qanday gaplashganini yozib qo'yish.
type Interaction struct {
	Time           time.Time `json:"time"`
	ConversationID int64     `json:"conversation_id"`
	ClientID       int64     `json:"client_id"`
	ClientName     string    `json:"client_name"`
	Title          string    `json:"title"`
	ClientMessage  string    `json:"client_message"`
	AIReply        string    `json:"ai_reply"`
	Sent           bool      `json:"sent"`
}

// Store — Postgres bilan ishlaydigan tarix do'koni.
type Store struct {
	db *sql.DB
}

// New yangi Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Append bitta yozuvni qo'shadi.
func (s *Store) Append(in Interaction) error {
	_, err := s.db.Exec(
		`INSERT INTO interactions
		 (conversation_id, client_id, client_name, title, client_message, ai_reply, sent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		in.ConversationID, in.ClientID, in.ClientName, in.Title,
		in.ClientMessage, in.AIReply, in.Sent)
	return err
}

// Recent oxirgi n yozuvni (eng yangisi birinchi) qaytaradi.
func (s *Store) Recent(n int) ([]Interaction, error) {
	if n <= 0 {
		n = 100
	}
	rows, err := s.db.Query(
		`SELECT created, conversation_id, client_id, client_name, title,
		        client_message, ai_reply, sent
		 FROM interactions ORDER BY id DESC LIMIT $1`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var in Interaction
		if err := rows.Scan(&in.Time, &in.ConversationID, &in.ClientID,
			&in.ClientName, &in.Title, &in.ClientMessage, &in.AIReply, &in.Sent); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Stats — umumiy statistika.
type Stats struct {
	TotalReplies  int
	SentReplies   int
	UniqueClients int
	UniqueChats   int
}

// Stats statistikani hisoblaydi.
func (s *Store) Stats() (Stats, error) {
	var st Stats
	err := s.db.QueryRow(
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE sent),
			COUNT(DISTINCT client_id) FILTER (WHERE client_id <> 0),
			COUNT(DISTINCT conversation_id)
		 FROM interactions`).
		Scan(&st.TotalReplies, &st.SentReplies, &st.UniqueClients, &st.UniqueChats)
	return st, err
}

// String qisqacha statistika.
func (st Stats) String() string {
	return fmt.Sprintf("odamlar: %d | suhbatlar: %d | javoblar: %d (yuborilgan: %d)",
		st.UniqueClients, st.UniqueChats, st.TotalReplies, st.SentReplies)
}
