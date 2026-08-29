package support

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TokenTTL - JWT amal qilish muddati.
const TokenTTL = 24 * time.Hour

var (
	// ErrBadToken - token yaroqsiz yoki muddati o'tgan.
	ErrBadToken = errors.New("token yaroqsiz")
	// ErrBadCredentials - login yoki parol xato.
	ErrBadCredentials = errors.New("login yoki parol xato")
)

// jwtSecret .env dagi JWT_SECRET ni oladi (bo'sh bo'lsa - default).
func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "sahiy-dev-secret-change-me"
	}
	return []byte(s)
}

// Claims - token ichidagi ma'lumot.
type Claims struct {
	Sub   uint   `json:"sub"`   // user id
	Login string `json:"login"` // login
	Role  string `json:"role"`  // rol
	Exp   int64  `json:"exp"`   // tugash vaqti (unix)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func sign(msg string) string {
	m := hmac.New(sha256.New, jwtSecret())
	m.Write([]byte(msg))
	return b64(m.Sum(nil))
}

// NewToken foydalanuvchi uchun HS256 JWT yasaydi.
func NewToken(u *User) (string, error) {
	head := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(Claims{
		Sub:   u.ID,
		Login: u.Login,
		Role:  u.Role,
		Exp:   time.Now().Add(TokenTTL).Unix(),
	})
	if err != nil {
		return "", err
	}
	body := head + "." + b64(payload)
	return body + "." + sign(body), nil
}

// ParseToken tokenni tekshirib claims qaytaradi.
func ParseToken(tok string) (*Claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, ErrBadToken
	}
	body := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(body)), []byte(parts[2])) {
		return nil, ErrBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrBadToken
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, ErrBadToken
	}
	if time.Now().Unix() > c.Exp {
		return nil, fmt.Errorf("%w: muddati o'tgan", ErrBadToken)
	}
	return &c, nil
}

// Authenticate login/parolni tekshirib token qaytaradi.
func Authenticate(login, password string) (string, *User, error) {
	if DB == nil {
		return "", nil, errors.New("baza ulanmagan")
	}
	u, err := FindUserByLogin(DB, login)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, ErrBadCredentials
	}
	if err != nil {
		// Baza yiqilgan bo'lsa buni "parol xato" deb ko'rsatish chalg'itadi.
		return "", nil, fmt.Errorf("baza: %w", err)
	}
	if !u.CheckPassword(password) {
		return "", nil, ErrBadCredentials
	}
	if !u.IsActive {
		return "", nil, errors.New("foydalanuvchi bloklangan")
	}
	tok, err := NewToken(u)
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	DB.Model(u).Update("last_login", &now)
	u.LastLogin = &now
	return tok, u, nil
}

// ClaimsFromRequest so'rovdagi "Authorization: Bearer <token>" ni o'qiydi.
func ClaimsFromRequest(r *http.Request) (*Claims, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, ErrBadToken
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if tok == "" {
		return nil, ErrBadToken
	}
	return ParseToken(tok)
}

// RequireAuth - himoyalangan endpointlar uchun middleware.
// Claims'ni handler'ga context orqali emas, qayta o'qish orqali beradi
// (ClaimsFromRequest arzon - faqat HMAC tekshiruvi).
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := ClaimsFromRequest(r); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"avtorizatsiya kerak: Authorization: Bearer <token>"}`))
			return
		}
		next(w, r)
	}
}
