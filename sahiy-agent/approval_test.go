package main

import (
	"testing"

	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
	"sahiy-agent/internal/models"
	"sahiy-agent/internal/settings"
)

// TestDeliverToClientNeedsApproval — loyihadagi eng muhim qoida:
// avto-javob o'chiq bo'lsa mijozga HECH NARSA yuborilmaydi.
//
// Qoida ilgari ikkita joyda takrorlangan edi; endi u deliverToClient
// ichida yagona nusxada va shu test uni qulflab turadi.
func TestDeliverToClientNeedsApproval(t *testing.T) {
	newApp := func(auto bool) (*app, *int, *string) {
		calls, sent := 0, ""
		a := &app{
			// cfg.AutoReply — .env dagi zaxira qiymat (false); bazadagi
			// sozlama undan ustun turadi.
			cfg: &config.Config{},
			set: settings.NewMemory(map[string]bool{settings.AutoReply: auto}),
			send: func(_ *client.Client, _, _ int64, _, text string) ([]byte, error) {
				calls++
				sent = text
				return nil, nil
			},
		}
		return a, &calls, &sent
	}

	t.Run("avto-javob o'chiq — yuborilmaydi", func(t *testing.T) {
		a, calls, _ := newApp(false)
		in := &models.Interaction{}
		tr := &trace{}

		a.deliverToClient(nil, 1, 2, "mijozga javob", in, tr)

		if *calls != 0 {
			t.Errorf("tasdiqsiz %d ta xabar yuborildi — qoida buzilgan", *calls)
		}
		if in.Sent {
			t.Error("in.Sent true — yuborilmagan javob yuborilgan deb yozildi")
		}
		if in.Status != models.StatusAIDraft {
			t.Errorf("status %q, %q kutilgandi", in.Status, models.StatusAIDraft)
		}
	})

	t.Run("avto-javob yoqiq — yuboriladi", func(t *testing.T) {
		a, calls, sent := newApp(true)
		in := &models.Interaction{}

		a.deliverToClient(nil, 1, 2, "mijozga javob", in, &trace{})

		if *calls != 1 {
			t.Errorf("%d ta chaqiruv, 1 kutilgandi", *calls)
		}
		if *sent != "mijozga javob" {
			t.Errorf("yuborilgan matn: %q", *sent)
		}
		if !in.Sent || in.Status != models.StatusAISent {
			t.Errorf("sent=%v status=%q", in.Sent, in.Status)
		}
	})

	t.Run("sozlama umuman yo'q — yuborilmaydi", func(t *testing.T) {
		// Default xavfsiz tomonga og'ishi kerak: kalit bazada bo'lmasa
		// ham javob mijozga ketmasin.
		a := &app{cfg: &config.Config{}, set: settings.NewMemory(nil)}
		calls := 0
		a.send = func(*client.Client, int64, int64, string, string) ([]byte, error) {
			calls++
			return nil, nil
		}
		in := &models.Interaction{}

		a.deliverToClient(nil, 1, 2, "javob", in, &trace{})

		if calls != 0 {
			t.Error("sozlamasiz holatda xabar yuborildi")
		}
		if in.Status != models.StatusAIDraft {
			t.Errorf("status %q", in.Status)
		}
	})

	t.Run("yuborish xatosi — failed", func(t *testing.T) {
		a := &app{cfg: &config.Config{},
			set: settings.NewMemory(map[string]bool{settings.AutoReply: true})}
		a.send = func(*client.Client, int64, int64, string, string) ([]byte, error) {
			return nil, errSend
		}
		in := &models.Interaction{}

		a.deliverToClient(nil, 1, 2, "javob", in, &trace{})

		if in.Sent {
			t.Error("xatoga qaramay Sent true")
		}
		if in.Status != models.StatusFailed {
			t.Errorf("status %q, %q kutilgandi", in.Status, models.StatusFailed)
		}
	})
}

var errSend = errTest("yuborilmadi")

type errTest string

func (e errTest) Error() string { return string(e) }
