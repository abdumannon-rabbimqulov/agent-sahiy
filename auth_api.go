package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"sahiy/support"

	"gorm.io/gorm"
)

// writeJSON javobni JSON qilib yozadi.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// writeErr xato javobi: {"error":"..."}
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// loginHandler: POST /api/auth/login  {"login":"991134543","password":"991134543"}
// Javob: {"token":"...","user":{...}}
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if body.Login == "" || body.Password == "" {
		writeErr(w, http.StatusBadRequest, "login va password majburiy")
		return
	}

	token, user, err := support.Authenticate(body.Login, body.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

// meHandler: GET /api/auth/me — token egasining ma'lumoti.
func meHandler(w http.ResponseWriter, r *http.Request) {
	c, err := support.ClaimsFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	u, err := support.FindUserByLogin(support.DB, c.Login)
	if err != nil {
		writeErr(w, http.StatusNotFound, "foydalanuvchi topilmadi")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// pathID URL'dagi {id} ni oladi.
func pathID(r *http.Request) (uint, bool) {
	n, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

// promtListHandler: GET /api/promts
func promtListHandler(w http.ResponseWriter, r *http.Request) {
	list, err := support.ListPromts(support.DB)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// promtCreateHandler: POST /api/promts  {"title":"...","promt":"..."}
func promtCreateHandler(w http.ResponseWriter, r *http.Request) {
	var p support.Promt
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if p.Title == "" || p.Promt == "" {
		writeErr(w, http.StatusBadRequest, "title va promt majburiy")
		return
	}
	p.ID = 0
	if err := support.CreatePromt(support.DB, &p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// promtGetHandler: GET /api/promts/{id}
func promtGetHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	p, err := support.GetPromt(support.DB, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "promt topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// promtUpdateHandler: PUT /api/promts/{id}  {"title":"...","promt":"..."}
func promtUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	var body struct {
		Title string `json:"title"`
		Promt string `json:"promt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "body JSON emas")
		return
	}
	if body.Title == "" && body.Promt == "" {
		writeErr(w, http.StatusBadRequest, "title yoki promt bering")
		return
	}
	p, err := support.UpdatePromt(support.DB, id, body.Title, body.Promt)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "promt topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// promtDeleteHandler: DELETE /api/promts/{id}
func promtDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "id noto'g'ri")
		return
	}
	err := support.DeletePromt(support.DB, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErr(w, http.StatusNotFound, "promt topilmadi")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "o'chirildi"})
}
