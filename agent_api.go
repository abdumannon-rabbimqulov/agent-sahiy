package main

import (
	"context"
	"encoding/json"
	"errors"
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
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	in, err := support.GetInteraction(support.DB, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// interactionPatchHandler: PATCH /api/interactions/{id}
// Tasdiqdan oldin chat/help matnini tahrirlash.
func interactionPatchHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	var body struct {
		ChatReply *string `json:"chat_reply"`
		HelpText  *string `json:"help_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	in, err := support.GetInteraction(support.DB, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	claims, err := support.ClaimsFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	in, err := support.GetInteraction(support.DB, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	claims, err := support.ClaimsFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
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

// settingsUpdateHandler: PUT /api/settings {"auto_reply":true}
func settingsUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var body map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	allowed := map[string]bool{
		support.SettingAutoReply:   true,
		support.SettingPollEnabled: true,
	}
	for k, v := range body {
		if !allowed[k] {
			writeErr(w, http.StatusBadRequest, "noma'lum sozlama: "+k)
			return
		}
		if err := support.SetSetting(support.DB, k, strconv.FormatBool(v)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
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
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if body.ConversationID <= 0 {
		writeErr(w, http.StatusBadRequest, "conversation_id majburiy")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	in, err := support.RunChain(ctx, body.ConversationID, body.ClientID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": err.Error(), "interaction": in,
		})
		return
	}
	writeJSON(w, http.StatusOK, in)
}
