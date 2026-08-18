package support

import (
	"encoding/json"
	"fmt"
	"net/http"

	"sahiy-agent/internal/client"
)

// endpointPath — kerak bo'lsa shu yerdan o'zgartiring.
const endpointPath = "/api/v1/support.chat.conversation/filter"

// Conversation — response'dagi bitta chat. Null bo'lishi mumkin maydonlar
// pointer sifatida olingan. topic_key son ham, satr ham bo'lishi mumkin →
// RawMessage.
type Conversation struct {
	ID                  int64           `json:"id"`
	ClientID            *int64          `json:"client_id"`
	SellerID            *int64          `json:"seller_id"`
	AgentID             *int64          `json:"agent_id"`
	Title               string          `json:"title"`
	Message             string          `json:"message"`
	ConversationType    string          `json:"conversation_type"`
	Status              int             `json:"status"`
	State               int             `json:"state"`
	ResolutionState     int             `json:"resolution_state"`
	TopicKey            json.RawMessage `json:"topic_key"`
	OperatorUnseenCount int             `json:"operator_unseen_count"`
	UnseenCount         int             `json:"unseen_count"`
	CreatedAt           string          `json:"created_at"`
	MsCreatedAt         json.RawMessage `json:"ms_created_at"`
	SellerName          string          `json:"seller_name"`
	ClientName          string          `json:"client_name"`
}

// FilterRequest — POST body.
// ClientID > 0 bo'lsa qidiruv (search) rejimida ishlaydi.
type FilterRequest struct {
	Type     string `json:"type"`
	State    []int  `json:"state"`
	TopicKey string `json:"topic_key,omitempty"`
	ClientID int64  `json:"client_id,omitempty"`
}

// filterResponse — server javobi (bizga kerakli qismi).
type filterResponse struct {
	Data struct {
		Chats []Conversation `json:"chats"`
	} `json:"data"`
}

// FetchConversations bitta sahifani olib keladi.
func FetchConversations(c *client.Client, page, limit int, req FilterRequest) ([]Conversation, error) {
	path := fmt.Sprintf("%s?page=%d&limit=%d", endpointPath, page, limit)
	body, status, err := c.Do(http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("filter muvaffaqiyatsiz (status %d): %s", status, string(body))
	}

	var resp filterResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("javobni o'qib bo'lmadi: %w\nXom javob: %s", err, string(body))
	}
	return resp.Data.Chats, nil
}

// SearchByClient client_id bo'yicha chatlarni qidiradi (search).
// Xuddi FetchConversations, faqat body'ga client_id qo'shiladi.
func SearchByClient(c *client.Client, clientID int64, page, limit int, state []int) ([]Conversation, error) {
	if len(state) == 0 {
		state = []int{1, 2, 3}
	}
	return FetchConversations(c, page, limit, FilterRequest{
		Type:     "client",
		State:    state,
		ClientID: clientID,
	})
}
