package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"sahiy/support"
)

// collectStatusHandler: GET /api/collect/status — jarayon holati.
func collectStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, support.CollectStatusNow())
}

// collectSummaryHandler: GET /api/collect/summary — tur kartalari.
func collectSummaryHandler(w http.ResponseWriter, r *http.Request) {
	s, err := support.GetCollectSummary()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// collectOrdersHandler: GET /api/collect/orders?type=&bucket=&ai_type=&page=&limit=
func collectOrdersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, total, err := support.ListCollectOrders(q.Get("type"), q.Get("bucket"), q.Get("ai_type"),
		queryInt(r, "page", 1), queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// collectClientsHandler: GET /api/collect/clients?ai_type=&arrived_after=1&page=&limit=
func collectClientsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, total, err := support.ListCollectClients(q.Get("ai_type"), q.Get("arrived_after") == "1",
		queryInt(r, "page", 1), queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// collectChatsHandler: GET /api/collect/chats?state=no_numbers&page=&limit=
func collectChatsHandler(w http.ResponseWriter, r *http.Request) {
	items, total, err := support.ListCollectChats(r.URL.Query().Get("state"),
		queryInt(r, "page", 1), queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// collectRecentHandler: GET /api/collect/recent?limit=30 — jonli lenta.
func collectRecentHandler(w http.ResponseWriter, r *http.Request) {
	items, err := support.RecentCollect(queryInt(r, "limit", 30))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// collectStartHandler: POST /api/collect/start {"max":50} — max 0 yoki
// berilmasa butun navbat.
func collectStartHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Max int `json:"max"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if !support.CollectMode() {
		writeErr(w, http.StatusConflict, "yig'ish rejimi o'chiq")
		return
	}
	if err := support.RequestCollectRun(body.Max); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, support.CollectStatusNow())
}

// collectStopHandler: POST /api/collect/stop
func collectStopHandler(w http.ResponseWriter, r *http.Request) {
	if err := support.StopCollect(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, support.CollectStatusNow())
}

// collectOrderDeleteHandler: DELETE /api/collect/orders/{sn} — buyurtmani
// muammolar hisobotidan o'chiradi.
func collectOrderDeleteHandler(w http.ResponseWriter, r *http.Request) {
	sn := r.PathValue("sn")
	if sn == "" {
		writeErr(w, http.StatusBadRequest, "buyurtma raqami berilmagan")
		return
	}
	if err := support.DismissCollectOrder(sn); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "o'chirildi"})
}

// collectChatDeleteHandler: DELETE /api/collect/chats/{id} — raqamsiz
// murojaatni hisobotdan o'chiradi.
func collectChatDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "suhbat id noto'g'ri")
		return
	}
	if err := support.DismissCollectChat(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "o'chirildi"})
}
