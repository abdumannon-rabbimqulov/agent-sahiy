package db_test

import (
	"os"
	"testing"

	"sahiy-agent/internal/db"
	"sahiy-agent/internal/models"
)

func TestMigratePrompts(t *testing.T) {
	dsn := os.Getenv("MIGRATE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIGRATE_TEST_DATABASE_URL berilmagan")
	}
	gdb, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("ulanmadi: %v", err)
	}
	if gdb.Migrator().HasTable("prompt_histories") {
		t.Fatal("prompt_histories o'chirilmagan")
	}
	if !gdb.Migrator().HasColumn(&models.Prompt{}, "id") {
		t.Fatal("id ustuni qo'shilmagan")
	}
	if gdb.Migrator().HasColumn(&models.Prompt{}, "version") {
		t.Fatal("version ustuni qolib ketgan")
	}
	var rows []models.Prompt
	if err := gdb.Order("key").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Key != "base" || rows[0].Content != "eski matn" || rows[0].ID == 0 {
		t.Fatalf("ma'lumot yo'qolgan yoki id berilmagan: %+v", rows)
	}
	// Ikkinchi marta ishga tushirish ham xatosiz (idempotent).
	if err := db.Migrate(gdb); err != nil {
		t.Fatalf("takroriy migratsiya: %v", err)
	}
	// Yangi yozuv id avtomatik o'sishi kerak.
	p := models.Prompt{Key: "cat:yangi", Content: "matn", Enabled: true}
	if err := gdb.Create(&p).Error; err != nil || p.ID == 0 {
		t.Fatalf("yangi yozuv: %v (id=%d)", err, p.ID)
	}
}
