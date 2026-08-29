package support

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Rollar
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
)

// User - tizim foydalanuvchisi (login/parol bilan kiradi).
type User struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Login     string     `gorm:"size:64;uniqueIndex;not null" json:"login"`
	Password  string     `gorm:"size:255;not null" json:"-"` // bcrypt hash
	Role      string     `gorm:"size:32;not null;default:operator" json:"role"`
	IsActive  bool       `gorm:"not null;default:true" json:"is_active"`
	LastLogin *time.Time `json:"last_login,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SetPassword parolni bcrypt bilan hashlab qo'yadi.
func (u *User) SetPassword(plain string) error {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(h)
	return nil
}

// CheckPassword parol to'g'riligini tekshiradi.
func (u *User) CheckPassword(plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plain)) == nil
}

// FindUserByLogin login bo'yicha foydalanuvchini topadi.
func FindUserByLogin(db *gorm.DB, login string) (*User, error) {
	var u User
	if err := db.Where("login = ?", login).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// EnsureUser login bo'yicha foydalanuvchi bo'lmasa yaratadi (seed).
func EnsureUser(db *gorm.DB, login, password, role string) error {
	var n int64
	if err := db.Model(&User{}).Where("login = ?", login).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	u := User{Login: login, Role: role, IsActive: true}
	if err := u.SetPassword(password); err != nil {
		return err
	}
	return db.Create(&u).Error
}
