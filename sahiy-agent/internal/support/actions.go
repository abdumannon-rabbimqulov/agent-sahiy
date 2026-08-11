package support

import (
	"fmt"
	"net/http"

	"sahiy-agent/internal/client"
)

const (
	deleteMessagePath = "/api/v1/support.chat.message"
	resolutionPath    = "/api/v1/support.chat.conversation/resolution"
)

// DeleteMessage bitta xabarni o'chiradi (DELETE .../support.chat.message/{id}).
func DeleteMessage(c *client.Client, messageID int64) error {
	path := fmt.Sprintf("%s/%d", deleteMessagePath, messageID)
	body, status, err := c.Do(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("xabar o'chirish muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return nil
}

// ResolutionRequest — suhbat resolution holatini yangilash body'si.
// izoh maydoni bo'sh bo'lsa yuborilmaydi.
type ResolutionRequest struct {
	ResolutionState int    `json:"resolution_state"`
	Comment         string `json:"comment,omitempty"`
}

// UpdateResolution suhbatning resolution holatini yangilaydi
// (PUT .../support.chat.conversation/resolution/{conversation_id}).
func UpdateResolution(c *client.Client, conversationID int64, state int, comment string) error {
	path := fmt.Sprintf("%s/%d", resolutionPath, conversationID)
	req := ResolutionRequest{ResolutionState: state, Comment: comment}
	body, status, err := c.Do(http.MethodPut, path, req)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("resolution yangilash muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return nil
}
