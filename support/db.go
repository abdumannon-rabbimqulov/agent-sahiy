package support

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB - global ulanish (InitDB dan keyin ishlatiladi).
var DB *gorm.DB

// DSN - .env dagi DB_* o'zgaruvchilaridan postgres ulanish satrini yasaydi.
// DB_DSN to'liq berilgan bo'lsa, o'sha ishlatiladi.
func DSN() string {
	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		return dsn
	}
	get := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		get("DB_HOST", "localhost"),
		get("DB_PORT", "5432"),
		get("DB_USER", "sahiy"),
		get("DB_PASSWORD", "sahiy"),
		get("DB_NAME", "sahiy"),
		get("DB_SSLMODE", "disable"),
		get("DB_TIMEZONE", "Asia/Tashkent"),
	)
}

// InitDB postgresga ulanadi, jadvallarni migratsiya qiladi va admin userni
// seed qiladi. Baza hali ko'tarilmagan bo'lishi mumkin (docker compose) —
// shuning uchun ulanish bir necha marta qayta uriniladi.
func InitDB() (*gorm.DB, error) {
	dsn := DSN()

	var db *gorm.DB
	var err error
	for i := 1; i <= 10; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			break
		}
		log.Printf("baza kutilmoqda (%d/10): %v", i, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, err
	}

	// Ulanishlar hovuzi (postgres uchun cheklab qo'yamiz).
	if sqlDB, e := db.DB(); e == nil {
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	if err := db.AutoMigrate(
		&User{}, &Promt{},
		&Interaction{}, &AgentStep{}, &ConversationState{}, &Setting{},
	); err != nil {
		return nil, err
	}

	// Global sozlamalar (auto_reply=false — hamma javob tasdiq kutadi).
	if err := seedSettings(db); err != nil {
		return nil, err
	}

	// Boshlang'ich admin: .env dagi ADMIN_USER/ADMIN_PASS, bo'lmasa 991134543.
	login := os.Getenv("ADMIN_USER")
	pass := os.Getenv("ADMIN_PASS")
	if login == "" || pass == "" {
		login, pass = "991134543", "991134543"
	}
	if err := EnsureUser(db, login, pass, RoleAdmin); err != nil {
		return nil, err
	}
	log.Println("baza tayyor (postgres) | admin login:", login)

	DB = db
	return db, nil
}
