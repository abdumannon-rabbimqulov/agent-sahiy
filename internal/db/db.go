// Package db — GORM ulanishi, avtomatik migratsiya va boshlang'ich ma'lumot.
package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"sahiy-agent/internal/models"
)

// Connect Postgres'ga ulanadi (konteyner ko'tarilguncha kutadi) va
// modellarni avtomatik migratsiya qiladi.
func Connect(dsn string) (*gorm.DB, error) {
	var (
		db  *gorm.DB
		err error
	)
	cfg := &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Warn),
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(dsn), cfg)
		if err == nil {
			var raw *sql.DB
			if raw, err = db.DB(); err == nil {
				raw.SetMaxOpenConns(10)
				raw.SetMaxIdleConns(5)
				raw.SetConnMaxLifetime(time.Hour)
				if err = raw.Ping(); err == nil {
					return db, Migrate(db)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("postgresga ulanib bo'lmadi: %w", err)
}

// Migrate jadvallarni modellarga qarab yaratadi/yangilaydi va seed qiladi.
func Migrate(db *gorm.DB) error {
	if err := migratePrompts(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&models.Interaction{},
		&models.Escalation{},
		&models.ConversationState{},
		&models.Setting{},
		&models.Prompt{},
		&models.PromptBackup{},
	); err != nil {
		return fmt.Errorf("migratsiya: %w", err)
	}
	return migrateEscalationStatus(db)
}

// migratePrompts eski prompt sxemasini yangisiga keltiradi: birlamchi kalit
// `key` o'rniga `id` bo'ldi, versiyalash va tarix esa umuman olib tashlandi.
//
// AutoMigrate mavjud jadvalning birlamchi kalitini o'zgartira olmaydi,
// shuning uchun bu qadam undan OLDIN, xom SQL bilan bajariladi. Funksiya
// idempotent — har ishga tushishda xavfsiz chaqiriladi.
func migratePrompts(db *gorm.DB) error {
	if db.Migrator().HasTable("prompt_histories") {
		if err := db.Migrator().DropTable("prompt_histories"); err != nil {
			return fmt.Errorf("prompt tarixi jadvalini o'chirish: %w", err)
		}
		log.Println("✓ Prompt tarixi (prompt_histories) olib tashlandi")
	}
	if !db.Migrator().HasTable(&models.Prompt{}) {
		return nil // jadval yo'q — AutoMigrate uni yangi sxemada yaratadi
	}
	if db.Migrator().HasColumn(&models.Prompt{}, "id") {
		return nil // allaqachon ko'chirilgan
	}
	stmts := []string{
		`ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_pkey`,
		`ALTER TABLE prompts ADD COLUMN IF NOT EXISTS id BIGSERIAL`,
		`ALTER TABLE prompts ADD PRIMARY KEY (id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_prompts_key ON prompts (key)`,
		`ALTER TABLE prompts DROP COLUMN IF EXISTS version`,
	}
	for _, q := range stmts {
		if err := db.Exec(q).Error; err != nil {
			return fmt.Errorf("prompt jadvalini ko'chirish (%s): %w", q, err)
		}
	}
	log.Println("✓ Prompt jadvali yangi sxemaga ko'chirildi (id — birlamchi kalit)")
	return nil
}

// migrateEscalationStatus eski `resolved` ustunidagi ma'lumotni yangi
// `status` ustuniga ko'chiradi — aks holda eski hal qilingan murojaatlar
// dashboardda "jarayonda" bo'lib qolardi.
func migrateEscalationStatus(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&models.Escalation{}, "resolved") {
		return nil
	}
	if err := db.Exec(
		"UPDATE escalations SET status = ? WHERE resolved AND status <> ?",
		models.StatusStaffSent, models.StatusStaffSent).Error; err != nil {
		return fmt.Errorf("eskalatsiya statusini ko'chirish: %w", err)
	}
	if err := db.Migrator().DropColumn(&models.Escalation{}, "resolved"); err != nil {
		return fmt.Errorf("eski resolved ustunini o'chirish: %w", err)
	}
	log.Println("✓ Eskalatsiya holatlari yangi `status` ustuniga ko'chirildi")
	return nil
}

// SetSetting kalit-qiymatni saqlaydi (upsert).
func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&models.Setting{Key: key, Value: value}).Error
}

// GetSetting kalit qiymatini o'qiydi (bo'lmasa "" va nil).
// First emas, Find — "topilmadi" oddiy holat, logga ogohlantirish yozilmasin.
func GetSetting(db *gorm.DB, key string) (string, error) {
	var out []models.Setting
	if err := db.Where("key = ?", key).Limit(1).Find(&out).Error; err != nil {
		return "", err
	}
	if len(out) == 0 {
		return "", nil
	}
	return out[0].Value, nil
}
