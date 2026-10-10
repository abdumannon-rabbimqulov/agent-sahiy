package support

import "testing"

func TestWantsCancel(t *testing.T) {
	cases := []struct {
		name string
		msgs []Message
		want bool
	}{
		{
			name: "mijoz hozir so'radi",
			msgs: []Message{
				{SenderType: "agent", Message: "Buyurtmangiz tekshirilmoqda."},
				{SenderType: "client", Message: "pulimni qaytarib bering"},
			},
			want: true,
		},
		{
			name: "ketma-ket bir necha xabar",
			msgs: []Message{
				{SenderType: "agent", Message: "Tekshiramiz."},
				{SenderType: "client", Message: "atkaz qivorila"},
				{SenderType: "client", Message: "keremas zakaz"},
			},
			want: true,
		},
		{
			// Asosiy xato: mijoz bir marta so'ragan, biz javob berganmiz,
			// endi u shunchaki rahmat aytyapti — guruhga ogohlantirish
			// ketmasligi kerak.
			name: "so'rovga javob berilgan, endi rahmat",
			msgs: []Message{
				{SenderType: "client", Message: "pulimni qaytaring"},
				{SenderType: "agent", Message: "Murojaatingiz tekshirilmoqda."},
				{SenderType: "client", Message: "raxmat"},
			},
			want: false,
		},
		{
			name: "eski so'rov, mijoz boshqa narsa so'rayapti",
			msgs: []Message{
				{SenderType: "client", Message: "otmena qiling"},
				{SenderType: "agent", Message: "Xodimlar ko'rib chiqmoqda."},
				{SenderType: "client", Message: "?"},
			},
			want: false,
		},
		{
			// Bizning javobimizdagi so'z qoidani ishga tushirmasligi kerak.
			name: "so'z faqat bizning javobimizda",
			msgs: []Message{
				{SenderType: "agent", Message: "Bekor qilish bo'yicha xodim bog'lanadi."},
				{SenderType: "client", Message: "mayli"},
			},
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WantsCancel(c.msgs); got != c.want {
				t.Fatalf("WantsCancel = %v, kerak %v", got, c.want)
			}
		})
	}
}
