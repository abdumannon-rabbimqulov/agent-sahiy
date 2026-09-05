// Panel API'si: statistika, AI javoblari navbati (ko'rish, tahrirlash,
// tasdiqlash yoki rad etish), muammoli buyurtmalar, sozlamalar va
// agentni qo'lda ishga tushirish.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"sahiy/support"

	"gorm.io/gorm"
)

// queryInt - so'rovdagi butun son parametri (bo'lmasa def).
func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// requireID - yo'ldagi {id}; noto'g'ri bo'lsa javobni o'zi yozadi.
func requireID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
	}
	return id, ok
}

// requireClaims - so'rovdagi token egasi; xato bo'lsa javobni o'zi yozadi.
func requireClaims(w http.ResponseWriter, r *http.Request) (*support.Claims, bool) {
	claims, err := support.ClaimsFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return nil, false
	}
	return claims, true
}

// loadInteraction - {id} bo'yicha interaksiyani topadi. Topilmasa yoki
// xato bo'lsa javobni o'zi yozib false qaytaradi.
func loadInteraction(w http.ResponseWriter, r *http.Request) (*support.Interaction, bool) {
	id, ok := requireID(w, r)
	if !ok {
		return nil, false
	}
	in, err := support.GetInteraction(support.DB, id)
	if !handleFindErr(w, err, "topilmadi") {
		return nil, false
	}
	return in, true
}

// handleFindErr - baza qidiruvining xatosini javobga aylantiradi.
// Xato bo'lmasa true qaytaradi.
func handleFindErr(w http.ResponseWriter, err error, notFound string) bool {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeErr(w, http.StatusNotFound, notFound)
		return false
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	return true
}

// statsHandler: GET /api/stats — umumiy hisob.
func statsHandler(w http.ResponseWriter, r *http.Request) {
	s, err := support.GetStats(support.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// dailyStatsHandler: GET /api/stats/daily?days=30
func dailyStatsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := support.DailyStats(support.DB, queryInt(r, "days", 30))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// clientStatsHandler: GET /api/stats/clients?days=30&limit=50
func clientStatsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := support.ClientStats(support.DB, queryInt(r, "days", 30), queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// interactionsHandler: GET /api/interactions?status=pending&page=1&limit=20
func interactionsHandler(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page, limit := queryInt(r, "page", 1), queryInt(r, "limit", 20)

	list, total, err := support.ListInteractions(support.DB, status, page, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "page": page, "limit": limit, "items": list,
	})
}

// interactionGetHandler: GET /api/interactions/{id} — bosqichlari bilan.
func interactionGetHandler(w http.ResponseWriter, r *http.Request) {
	in, ok := loadInteraction(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// interactionPatchHandler: PATCH /api/interactions/{id}
// Tasdiqdan oldin chat/help matnini tahrirlash.
func interactionPatchHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChatReply *string `json:"chat_reply"`
		HelpText  *string `json:"help_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	in, ok := loadInteraction(w, r)
	if !ok {
		return
	}
	if in.Status != support.StatusPending && in.Status != support.StatusFailed {
		writeErr(w, http.StatusConflict, "faqat kutayotgan yoki xato javobni tahrirlash mumkin")
		return
	}
	if body.ChatReply != nil {
		in.ChatReply = *body.ChatReply
	}
	if body.HelpText != nil {
		in.HelpText = *body.HelpText
	}
	if err := support.DB.Model(in).Updates(map[string]any{
		"chat_reply": in.ChatReply, "help_text": in.HelpText,
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// approveHandler: POST /api/interactions/{id}/approve
// chat -> mijozga, help -> Telegram guruhga.
func approveHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	in, ok := loadInteraction(w, r)
	if !ok {
		return
	}
	if in.Status == support.StatusSent || in.Status == support.StatusApproved {
		writeErr(w, http.StatusConflict, "bu javob allaqachon yuborilgan")
		return
	}
	if in.ChatReply == "" && in.HelpText == "" {
		writeErr(w, http.StatusBadRequest, "yuboradigan matn yo'q")
		return
	}

	if err := support.Deliver(in); err != nil {
		support.DB.Model(in).Updates(map[string]any{
			"status": support.StatusFailed, "error": err.Error(),
		})
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	now := time.Now()
	if err := support.DB.Model(in).Updates(map[string]any{
		"status": support.StatusApproved, "handled_by": claims.Login,
		"sent_at": &now, "error": "",
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "yuborildi"})
}

// rejectHandler: POST /api/interactions/{id}/reject
func rejectHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r)
	if !ok {
		return
	}
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	res := support.DB.Model(&support.Interaction{}).
		Where("id = ? AND status NOT IN ?", id, []string{support.StatusSent, support.StatusApproved}).
		Updates(map[string]any{"status": support.StatusRejected, "handled_by": claims.Login})
	if res.Error != nil {
		writeErr(w, http.StatusInternalServerError, res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		writeErr(w, http.StatusConflict, "topilmadi yoki allaqachon yuborilgan")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rad etildi"})
}

// settingsHandler: GET /api/settings
func settingsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, support.AllSettings())
}

// settingsUpdateHandler: PUT /api/settings
// {"auto_reply":true} yoki {"poll_interval_sec":120,"chat_delay_sec":10}
//
// Bool va sonli sozlamalar birga kelishi mumkin. Darhol kuchga kiradi.
func settingsUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}

	// Har bir sozlama uchun ruxsat etilgan oraliq. Chegara buzilsa
	// xato qaytariladi — jim turib qiymatni o'zgartirib qo'ymaymiz.
	limits := map[string]struct{ min, max int }{
		support.SettingPollInterval: {10, 3600},
		support.SettingBatchSize:    {1, 50},
		support.SettingChatDelay:    {0, 600},
	}
	bools := map[string]bool{
		support.SettingAgentEnabled: true,
		support.SettingAutoReply:    true,
		support.SettingPollEnabled:  true,
		support.SettingAutoResolve:  true,
	}

	for k, v := range body {
		switch {
		case bools[k]:
			b, ok := v.(bool)
			if !ok {
				writeErr(w, http.StatusBadRequest, k+": true yoki false bo'lishi kerak")
				return
			}
			if err := support.SetSetting(support.DB, k, strconv.FormatBool(b)); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}

		default:
			lim, ok := limits[k]
			if !ok {
				writeErr(w, http.StatusBadRequest, "noma'lum sozlama: "+k)
				return
			}
			f, ok := v.(float64)
			if !ok {
				writeErr(w, http.StatusBadRequest, k+": son bo'lishi kerak")
				return
			}
			n := int(f)
			if n < lim.min || n > lim.max {
				writeErr(w, http.StatusBadRequest,
					fmt.Sprintf("%s: %d–%d oralig'ida bo'lishi kerak", k, lim.min, lim.max))
				return
			}
			if err := support.SetSetting(support.DB, k, strconv.Itoa(n)); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, support.AllSettings())
}

// agentRunHandler: POST /api/agent/run {"conversation_id":123,"client_id":456}
// Zanjirni qo'lda ishga tushiradi (test va qayta urinish uchun).
func agentRunHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID int64 `json:"conversation_id"`
		ClientID       int64 `json:"client_id"`
		// Force - oxirgi so'z biz tomondan bo'lsa ham ishga tushirish.
		Force bool `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if body.ConversationID <= 0 {
		writeErr(w, http.StatusBadRequest, "conversation_id majburiy")
		return
	}

	if !support.AgentEnabled() {
		writeErr(w, http.StatusConflict, support.ErrAgentDisabled.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	run := support.RunChain
	if body.Force {
		run = support.RunChainForce
	}

	in, err := run(ctx, body.ConversationID, body.ClientID)
	if errors.Is(err, support.ErrAlreadyAnswered) {
		writeErr(w, http.StatusConflict, err.Error()+` (qayta ishga tushirish uchun: {"force":true})`)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": err.Error(), "interaction": in,
		})
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// agentScanHandler: POST /api/agent/scan {"pages":6,"limit":100,"max":0}
//
// Suhbatlar ro'yxatini (support.chat.conversation/filter) to'liq ko'rib
// chiqadi va HAR BIR mijoz uchun zanjirni ketma-ket yuritadi —
// `operator_unseen_count` filtri va `batch_size` chegarasisiz.
//
// Ish uzoq davom etadi (har suhbat bir necha sekund), shuning uchun fonda
// bajariladi: javob darhol qaytadi, natija esa navbatda (interactions)
// va logda ko'rinadi.
func agentScanHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pages int `json:"pages"`
		Limit int `json:"limit"`
		Max   int `json:"max"` // 0 — hammasi
	}
	// Bo'sh body ham to'g'ri: hamma qiymat default bo'ladi.
	_ = json.NewDecoder(r.Body).Decode(&body)

	if !support.AgentEnabled() {
		writeErr(w, http.StatusConflict, support.ErrAgentDisabled.Error())
		return
	}
	if support.ScanRunning() {
		writeErr(w, http.StatusConflict, "skanerlash allaqachon ketyapti")
		return
	}

	go func() {
		// So'rov tugagach ish to'xtamasin — o'z konteksti bilan yuradi.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		if _, err := support.ScanOnce(ctx, body.Pages, body.Limit, body.Max); err != nil {
			log.Printf("skaner: %v", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "skanerlash boshlandi — natija navbatda va logda ko'rinadi",
	})
}

// issuesHandler: GET /api/issues?state=open&page=1&limit=20
func issuesHandler(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	page, limit := queryInt(r, "page", 1), queryInt(r, "limit", 20)

	rows, total, err := support.ListIssues(support.DB, state, page, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "page": page, "limit": limit, "items": rows,
	})
}

// issueGetHandler: GET /api/issues/{id}
func issueGetHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r)
	if !ok {
		return
	}
	var is support.OrderIssue
	if !handleFindErr(w, support.DB.First(&is, id).Error, "muammo topilmadi") {
		return
	}
	writeJSON(w, http.StatusOK, is)
}

// issueResolveHandler: POST /api/issues/{id}/resolve {"resolution":"..."}
// Paneldan qo'lda yopish (odatda yechim Telegram guruhdagi reply orqali keladi).
func issueResolveHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := requireID(w, r)
	if !ok {
		return
	}
	claims, ok := requireClaims(w, r)
	if !ok {
		return
	}
	var body struct {
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if body.Resolution == "" {
		writeErr(w, http.StatusBadRequest, "qanday hal qilingani yozilishi kerak")
		return
	}

	var is support.OrderIssue
	if !handleFindErr(w, support.DB.First(&is, id).Error, "muammo topilmadi") {
		return
	}
	if is.State == support.IssueResolved {
		writeErr(w, http.StatusConflict, "bu muammo allaqachon yopilgan")
		return
	}

	if err := support.ResolveIssue(support.DB, &is, body.Resolution,
		claims.Login, support.ResolvedViaPanel); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, is)
}

// issuesDailyHandler: GET /api/stats/issues/daily?days=30
func issuesDailyHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := support.IssueDailyStats(support.DB, queryInt(r, "days", 30))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// issuesReviewHandler: POST /api/issues/review
// Ochiq muammolarni darhol qayta ko'rib chiqadi: adminkada holat
// o'zgarganmi, mijozga javob berilganmi va eslatma vaqti kelganmi.
// Odatda buni fon sikli o'zi bajaradi — bu esa qo'lda tekshirish uchun.
func issuesReviewHandler(w http.ResponseWriter, r *http.Request) {
	if err := support.ReviewOpenIssues(support.DB); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	stats, err := support.GetIssueStats(support.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ko'rib chiqildi", "stats": stats})
}
