// Package db — GORM ulanishi, avtomatik migratsiya va boshlang'ich ma'lumot.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
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
	if err := db.AutoMigrate(
		&models.Category{},
		&models.Interaction{},
		&models.Escalation{},
		&models.ChatImage{},
		&models.ConversationState{},
		&models.Setting{},
	); err != nil {
		return fmt.Errorf("migratsiya: %w", err)
	}
	return seedCategories(db)
}

// seedCategories jadval bo'sh bo'lsa birinchi kategoriyani qo'shadi.
func seedCategories(db *gorm.DB) error {
	var n int64
	if err := db.Model(&models.Category{}).Count(&n).Error; err != nil {
		return fmt.Errorf("kategoriyalarni sanash: %w", err)
	}
	if n > 0 {
		return nil
	}

	seed := []models.Category{
		{
			Name:        "Yetkazib berish",
			Description: "yetkazib berish, punktlar, muddat, og'irlik chegarasi, dostavka narxi",
			Content: strings.Join([]string{
				"- O'zbekistonning barcha viloyatlarida punktlar mavjud (batafsil ma'lumot Sahiy ilovasi profil sahifasining quyi qismida bor).",
				"- Toshkent shahri va Toshkent viloyatiga uyingizgacha bepul yetkazib beriladi.",
				"- Bitta buyurtma 7 kg dan oshmasligi kerak. Agar 7 kg dan oshsa, narxini mutaxassislar aytadi.",
			}, "\n"),
		},
	}
	if err := db.Create(&seed).Error; err != nil {
		return fmt.Errorf("kategoriya seed: %w", err)
	}
	log.Println("✓ Boshlang'ich kategoriya qo'shildi (id=1 Yetkazib berish)")
	return nil
}

// SetSetting kalit-qiymatni saqlaydi (upsert).
func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&models.Setting{Key: key, Value: value}).Error
}

// GetSetting kalit qiymatini o'qiydi (bo'lmasa "" va nil).
func GetSetting(db *gorm.DB, key string) (string, error) {
	var s models.Setting
	err := db.First(&s, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return s.Value, err
}
