package support

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"sahiy-agent/internal/client"
)

// readPath — xabarlarni "o'qilgan" deb belgilash.
const readPath = "/api/v1/support.chat.message/read"

// MarkRead berilgan xabar ID'larini o'qilgan deb belgilaydi (PUT ...?ids=1,2,3).
func MarkRead(c *client.Client, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatInt(id, 10)
	}
	path := fmt.Sprintf("%s?ids=%s", readPath, strings.Join(strs, ","))

	body, status, err := c.Do(http.MethodPut, path, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("o'qilgan deb belgilash muvaffaqiyatsiz (status %d): %s", status, string(body))
	}
	return nil
}

// ClientMessageIDs xabarlar ichidan mijoz (client) yuborganlarining ID'larini
// qaytaradi — odatda o'qilgan deb belgilanadigan xabarlar shular.
func ClientMessageIDs(msgs []Message) []int64 {
	var ids []int64
	for _, m := range msgs {
		if m.SenderType == "client" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}
