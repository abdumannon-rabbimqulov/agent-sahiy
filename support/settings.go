package support

import (
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Sozlama kalitlari.
const (
	SettingAgentEnabled = "agent_enabled" // AI agent umuman ishlaydimi
	SettingAutoReply    = "auto_reply"    // AI javobi tasdiqsiz ketadimi
	SettingPollEnabled  = "poll_enabled"  // fon sikli ishlaydimi
	SettingAutoResolve  = "auto_resolve"  // javobdan keyin suhbat yopiladimi

	// Tezlik sozlamalari — panel orqali o'zgartiriladi, darhol kuchga
	// kiradi (fon sikli har aylanishda qaytadan o'qiydi).
	SettingPollInterval = "poll_interval_sec" // sikllar orasidagi oraliq
	SettingBatchSize    = "batch_size"        // bir siklda nechta suhbat
	SettingChatDelay    = "chat_delay_sec"    // suhbatlar orasidagi tanaffus
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

// GetIntSetting kalitni butun son sifatida o'qiydi (bo'lmasa def).
// min/max — ruxsat etilgan oraliq; undan tashqarisi kesiladi.
func GetIntSetting(key string, def, min, max int) int {
	v := GetSetting(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
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
func AllSettings() map[string]any {
	return map[string]any{
		SettingAgentEnabled: AgentEnabled(),
		SettingAutoReply:    AutoReplyOn(),
		SettingPollEnabled:  PollEnabled(),
		SettingAutoResolve:  AutoResolveOn(),
		SettingPollInterval: PollInterval(),
		SettingBatchSize:    BatchSize(),
		SettingChatDelay:    ChatDelay(),
	}
}

// AutoResolveOn - mijozga javob ketgandan keyin suhbat "hal qilindi"
// holatiga o'tkaziladimi.
func AutoResolveOn() bool { return GetBoolSetting(SettingAutoResolve, true) }

// PollInterval - fon sikllari orasidagi oraliq (sekund).
// Panelda berilmagan bo'lsa .env dagi POLL_INTERVAL_SEC ishlatiladi.
func PollInterval() int {
	return GetIntSetting(SettingPollInterval,
		envInt("POLL_INTERVAL_SEC", DefaultPollInterval), 10, 3600)
}

// BatchSize - bitta siklda nechta suhbat ishlanadi. Qolganlari
// YO'QOLMAYDI — keyingi siklda navbat bilan olinadi.
func BatchSize() int {
	return GetIntSetting(SettingBatchSize, envInt("RATE_LIMIT_COUNT", 5), 1, 50)
}

// ChatDelay - ketma-ket suhbatlar orasidagi tanaffus (sekund).
// Agentni sekinlashtirish uchun: modelning tezlik chegarasiga
// urilmaslik va tashqi API'larni bosmaslik.
func ChatDelay() int {
	return GetIntSetting(SettingChatDelay, envInt("CHAT_DELAY_SEC", 0), 0, 600)
}

// AgentEnabled - AI agent ishlaydimi. O'chirilsa zanjir umuman
// yurmaydi: na fon sikli, na qo'lda ishga tushirish model'ga bormaydi.
// Navbatdagi tayyor javoblarni tasdiqlash esa ishlayveradi.
func AgentEnabled() bool { return GetBoolSetting(SettingAgentEnabled, true) }

// AutoReplyOn - AI javobi tasdiqsiz ketadimi.
func AutoReplyOn() bool { return GetBoolSetting(SettingAutoReply, false) }

// PollEnabled - fon sikli yoqilganmi.
func PollEnabled() bool { return GetBoolSetting(SettingPollEnabled, true) }

// seedSettings boshlang'ich sozlamalarni yozadi (bor bo'lsa tegilmaydi).
func seedSettings(db *gorm.DB) error {
	defs := map[string]string{
		SettingAgentEnabled: "true",
		SettingAutoReply:    "false",
		SettingPollEnabled:  "true",
		SettingAutoResolve:  "true",
		SettingPollInterval: strconv.Itoa(envInt("POLL_INTERVAL_SEC", DefaultPollInterval)),
		SettingBatchSize:    strconv.Itoa(envInt("RATE_LIMIT_COUNT", 5)),
		SettingChatDelay:    strconv.Itoa(envInt("CHAT_DELAY_SEC", 0)),
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
