package support

import (
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Sozlama kalitlari.
const (
	SettingAutoReply   = "auto_reply"   // AI javobi tasdiqsiz ketadimi
	SettingPollEnabled = "poll_enabled" // fon sikli ishlaydimi
)

// settingsCache - bazaga har safar bormaslik uchun qisqa muddatli kesh.
var settingsCache = struct {
	sync.RWMutex
	val map[string]string
	at  time.Time
}{val: map[string]string{}}

// settingsTTL - kesh yangilanish oralig'i.
const settingsTTL = 5 * time.Second

// loadSettings barcha sozlamalarni bazadan o'qiydi.
func loadSettings() map[string]string {
	settingsCache.RLock()
	fresh := time.Since(settingsCache.at) < settingsTTL && settingsCache.val != nil
	val := settingsCache.val
	settingsCache.RUnlock()
	if fresh {
		return val
	}

	out := map[string]string{}
	if DB != nil {
		var rows []Setting
		if err := DB.Find(&rows).Error; err == nil {
			for _, s := range rows {
				out[s.Key] = s.Value
			}
		}
	}
	settingsCache.Lock()
	settingsCache.val, settingsCache.at = out, time.Now()
	settingsCache.Unlock()
	return out
}

// GetSetting kalit bo'yicha qiymat (topilmasa def).
func GetSetting(key, def string) string {
	if v, ok := loadSettings()[key]; ok && v != "" {
		return v
	}
	return def
}

// GetBoolSetting kalitni bool sifatida o'qiydi.
func GetBoolSetting(key string, def bool) bool {
	v := GetSetting(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// SetSetting qiymatni yozadi (yo'q bo'lsa yaratadi) va keshni tozalaydi.
func SetSetting(db *gorm.DB, key, value string) error {
	s := Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	err := db.Save(&s).Error
	settingsCache.Lock()
	settingsCache.at = time.Time{} // keshni eskirtiramiz
	settingsCache.Unlock()
	return err
}

// AllSettings panel uchun barcha sozlamalar (default'lar bilan).
func AllSettings() map[string]bool {
	return map[string]bool{
		SettingAutoReply:   GetBoolSetting(SettingAutoReply, false),
		SettingPollEnabled: GetBoolSetting(SettingPollEnabled, true),
	}
}

// AutoReplyOn - AI javobi tasdiqsiz ketadimi.
func AutoReplyOn() bool { return GetBoolSetting(SettingAutoReply, false) }

// PollEnabled - fon sikli yoqilganmi.
func PollEnabled() bool { return GetBoolSetting(SettingPollEnabled, true) }

// seedSettings boshlang'ich sozlamalarni yozadi (bor bo'lsa tegilmaydi).
func seedSettings(db *gorm.DB) error {
	defs := map[string]string{
		SettingAutoReply:   "false",
		SettingPollEnabled: "true",
	}
	for k, v := range defs {
		var n int64
		if err := db.Model(&Setting{}).Where("key = ?", k).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			if err := db.Create(&Setting{Key: k, Value: v, UpdatedAt: time.Now()}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
