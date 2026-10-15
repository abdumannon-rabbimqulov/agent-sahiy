// Tizim yiqilganda xodimlar guruhiga ogohlantirish.
//
// Nega kerak: panelda holat ko'rinadi (GET /api/health), lekin panelni
// hamma vaqt ochib turmaydi. Adminka tokeni tugasa agent ishlashda
// davom etadi — faqat buyurtma ma'lumotini ko'rmay qoladi, ya'ni
// buzilish JIMGINA sodir bo'ladi va bir necha kun sezilmasligi mumkin.
//
// Qoida: holat o'zgarganda bir marta xabar ketadi, keyin muammo
// turgan ekan — HEALTH_REPEAT_HOURS da bir eslatiladi, tuzalganda esa
// "tuzaldi" xabari. Takrorlanmasligi uchun oxirgi yuborilgan holat
// bazada saqlanadi (settings jadvali), shuning uchun qayta ishga
// tushirish xabarni qaytadan yog'dirmaydi.
package support

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Sozlamalar (.env).
const (
	DefaultHealthCheckSec   = 300 // tekshirish oralig'i
	DefaultHealthRepeatHour = 12  // muammo turgan bo'lsa qayta eslatish
)

// healthAlertPrefix - settings jadvalidagi kalit boshlanishi:
// `health_alert_adminka` = "fail|2026-10-14T10:00:00+05:00".
const healthAlertPrefix = "health_alert_"

// HealthCheckSec - .env dagi HEALTH_CHECK_SEC (standart 300).
func HealthCheckSec() int { return envInt("HEALTH_CHECK_SEC", DefaultHealthCheckSec) }

// HealthRepeatHours - muammo tuzalmasa necha soatda bir eslatiladi.
// 0 — eslatma O'CHADI: xabar faqat holat o'zgarganda ketadi.
//
// envInt bu yerda ishlamaydi: u 0 ni ham xato deb hisoblab standart
// qiymatni qaytaradi, ya'ni eslatmani o'chirib bo'lmas edi.
func HealthRepeatHours() int {
	v := strings.TrimSpace(envStr("HEALTH_REPEAT_HOURS", ""))
	if v == "" {
		return DefaultHealthRepeatHour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return DefaultHealthRepeatHour
	}
	return n
}

// HealthAlertsOn - ogohlantirish yoqilganmi (.env dagi HEALTH_ALERTS,
// standart yoqiq). O'chirish: HEALTH_ALERTS=false.
func HealthAlertsOn() bool {
	v := strings.ToLower(strings.TrimSpace(envStr("HEALTH_ALERTS", "true")))
	return v != "false" && v != "0" && v != "no"
}

// StartHealthAlerter fon siklini ishga tushiradi.
//
// Sikl `agent_enabled` o'chirilganda ham yuradi: tizim yiqilgani
// agentning yoqilgan-o'chirilganiga bog'liq emas va xodim buni bilishi
// kerak.
func StartHealthAlerter(ctx context.Context) {
	if !HealthAlertsOn() {
		log.Println("tizim holati: ogohlantirish o'chirilgan (HEALTH_ALERTS=false)")
		return
	}
	log.Printf("tizim holati: har %ds da tekshiriladi, muammo turganda %d soatda bir eslatiladi",
		HealthCheckSec(), HealthRepeatHours())

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("tizim holati: sikl to'xtadi")
				return
			case <-time.After(time.Duration(HealthCheckSec()) * time.Second):
			}
			alertHealthOnce()
		}
	}()
}

// alertHealthOnce bitta tekshiruv: holat o'zgarganlari haqida xabar
// yuboradi.
func alertHealthOnce() {
	rep := Health(true)
	for _, s := range rep.Systems {
		// Telegramning o'zi yiqilgan bo'lsa xabar ham ketmaydi —
		// bunisi faqat logda qoladi.
		if s.Key == "telegram" {
			if s.Status != HealthOK {
				log.Printf("tizim holati: %s — %s", s.Name, s.Message)
			}
			continue
		}
		prev, at := lastHealthAlert(s.Key)
		if text, ok := healthAlertText(s, prev, at); ok {
			if _, err := SendTelegramMessage(text, 0); err != nil {
				log.Printf("tizim holati: xabar yuborilmadi (%s): %v", s.Key, err)
				continue // holat yozilmaydi: keyingi siklda yana urinadi
			}
			rememberHealthAlert(s)
		}
	}
}

// healthAlertText - shu tizim uchun hozir xabar yuborish kerakmi va
// matni qanday. `prev` va `at` — oxirgi yuborilgan xabarning holati va
// vaqti (lastHealthAlert).
//
// Xabar uch holatda ketadi:
//  1. holat yomonlashdi (ok → warn/fail) yoki warn → fail;
//  2. muammo turgan ekan va oxirgi xabardan HEALTH_REPEAT_HOURS o'tdi;
//  3. tuzaldi (warn/fail → ok) — bir marta.
func healthAlertText(s SystemCheck, prev string, at time.Time) (string, bool) {
	if s.Status == HealthOK {
		if prev == "" || prev == HealthOK {
			return "", false // avval ham yaxshi edi — xabar shart emas
		}
		return fmt.Sprintf("✅ %s — tuzaldi\n%s", s.Name, s.Message), true
	}

	if prev != s.Status {
		return healthProblemText(s, false), true
	}
	// Holat o'zgarmagan: faqat eslatma vaqti kelganda.
	if repeat := time.Duration(HealthRepeatHours()) * time.Hour; repeat > 0 &&
		!at.IsZero() && time.Since(at) >= repeat {
		return healthProblemText(s, true), true
	}
	return "", false
}

// healthProblemText - muammo haqidagi xabar matni.
func healthProblemText(s SystemCheck, repeat bool) string {
	head := "🔴"
	if s.Status == HealthWarn {
		head = "⚠️"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", head, s.Name)
	if repeat {
		b.WriteString(" — muammo hali tuzalmagan")
	} else if s.Status == HealthFail {
		b.WriteString(" — ishlamayapti")
	}
	fmt.Fprintf(&b, "\n%s", s.Message)
	if s.Affects != "" {
		fmt.Fprintf(&b, "\n\nIshlamay qoladi: %s", s.Affects)
	}
	if s.Manual {
		b.WriteString("\n\nToken qo'lda yangilanadi: .env ni to'g'rilab, api konteynerini qayta ishga tushirish kerak.")
	}
	return b.String()
}

// lastHealthAlert - shu tizim uchun oxirgi yuborilgan holat va vaqti.
func lastHealthAlert(key string) (string, time.Time) {
	raw := GetSetting(healthAlertPrefix+key, "")
	status, ts, _ := strings.Cut(raw, "|")
	at, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		at = time.Time{}
	}
	return status, at
}

// rememberHealthAlert - yuborilgan xabarni belgilab qo'yadi.
func rememberHealthAlert(s SystemCheck) {
	if DB == nil {
		return
	}
	val := s.Status + "|" + time.Now().Format(time.RFC3339)
	if err := SetSetting(DB, healthAlertPrefix+s.Key, val); err != nil {
		log.Printf("tizim holati: holatni yozish (%s): %v", s.Key, err)
	}
}
