package support

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReadPath - xabarlarni "o'qilgan" deb belgilash.
const ReadPath = "/api/v1/support.chat.message/read"

// MarkRead berilgan xabarlarni o'qilgan deb belgilaydi (PUT ...?ids=1,2,3).
func MarkRead(baseURL, token string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	base := baseURL
	if base == "" {
		base = DefaultBaseURL
	}
	url := fmt.Sprintf("%s%s?ids=%s", strings.TrimRight(base, "/"), ReadPath, JoinIDs(ids))

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("o'qilgan deb belgilash: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("o'qilgan deb belgilanmadi (status %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil
}

// MarkReadCached token keshidan foydalanadi; token eskirgan bo'lsa yangilab
// bir marta qayta uriniladi.
func MarkReadCached(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	creds := CredentialsFromEnv()
	token, err := Token(creds, TokenFile)
	if err != nil {
		return err
	}
	err = MarkRead(creds.BaseURL, token, ids)
	if err == ErrUnauthorized {
		if token, err = Refresh(creds, TokenFile); err == nil {
			err = MarkRead(creds.BaseURL, token, ids)
		}
	}
	return err
}

// UnansweredClientIDs - oxirgi xodim javobidan keyin kelgan mijoz
// xabarlarining ID'lari. Ya'ni aynan shu murojaatda javob berilayotganlar.
// Xodim javobi umuman bo'lmasa — barcha mijoz xabarlari.
func UnansweredClientIDs(msgs []Message) []int64 {
	start := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].FromClient() {
			start = i + 1
			break
		}
	}
	var ids []int64
	for _, m := range msgs[start:] {
		if m.FromClient() && m.ID > 0 {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// JoinIDs - ID'larni "1,2,3" ko'rinishiga keltiradi.
func JoinIDs(ids []int64) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(out, ",")
}

// SplitIDs - "1,2,3" satrini ID'lar ro'yxatiga aylantiradi.
func SplitIDs(s string) []int64 {
	var ids []int64
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if n, err := strconv.ParseInt(p, 10, 64); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	return ids
}
