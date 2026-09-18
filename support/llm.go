// AI provayderni tanlash: Groq yoki DeepSeek. Ikkalasi ham bir xil
// LLM interfeysini bajaradi — zanjir qaysi provayder ishlatilayotganini
// bilishi shart emas.
package support

import (
	"context"
	"errors"
)

// LLM - agent zanjiri ishlatadigan minimal interfeys. Groq (groq.go) va
// Deepseek (deepseek.go) ikkalasi ham shuni bajaradi.
type LLM interface {
	Ready() bool
	Generate(ctx context.Context, system, user string) (string, Usage, error)
}

// SettingAIProvider - panelda tanlangan provayder ("groq"/"deepseek").
const SettingAIProvider = "ai_provider"

// AIProvider - hozir tanlangan provayder. Berilmagan yoki noma'lum
// qiymat bo'lsa Groq ishlatiladi (standart).
func AIProvider() string {
	v := GetSetting(SettingAIProvider, ProviderGroq)
	if v != ProviderDeepSeek {
		return ProviderGroq
	}
	return v
}

// ErrNoLLMKey - tanlangan provayderning kaliti berilmagan.
var ErrNoLLMKey = errors.New("tanlangan AI provayder uchun kalit berilmagan")

// ActiveLLM - .env va panel sozlamasiga qarab tayyor klient qaytaradi.
func ActiveLLM() LLM {
	if AIProvider() == ProviderDeepSeek {
		return DeepseekFromEnv()
	}
	return GroqFromEnv()
}
