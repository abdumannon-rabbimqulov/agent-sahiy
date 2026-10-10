// Xabarlarni "o'qilgan" deb belgilash va javobsiz qolgan mijoz
// xabarlarini ajratish (ID'lari bilan ishlash).
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
	return withTokenErr(func(baseURL, token string) error {
		return MarkRead(baseURL, token, ids)
	})
}

// UnansweredClient - oxirgi javobimizdan KEYIN kelgan mijoz xabarlari,
// ya'ni aynan shu murojaat. Javobimiz umuman bo'lmasa — hammasi.
//
// Nega kerak: kod topadigan holatlar (bekor qilish so'rovi va h.k.) butun
// tarixga qarab izlanganda abadiy qaytaverardi. Mijoz bir marta "pulimni
// qaytaring" desa, biz javob bersak ham, keyin u shunchaki "rahmat" yoki
// "?" deb yozganda ham xodimlar guruhiga yana o'sha ogohlantirish
// ketaverardi. Endi faqat JAVOB BERILMAGAN xabarlar ko'riladi.
func UnansweredClient(msgs []Message) []Message {
	start := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if !msgs[i].FromClient() {
			start = i + 1
			break
		}
	}
	out := make([]Message, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		if m.FromClient() {
			out = append(out, m)
		}
	}
	return out
}

// UnansweredClientIDs - o'sha xabarlarning ID'lari.
func UnansweredClientIDs(msgs []Message) []int64 {
	var ids []int64
	for _, m := range UnansweredClient(msgs) {
		if m.ID > 0 {
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
