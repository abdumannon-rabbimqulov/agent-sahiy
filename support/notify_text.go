// Telegram guruhga ketadigan xabarlarning YAGONA ko'rinishi.
//
// Guruhga IKKI turdagi xabar boradi: yangi muammoli buyurtma (⚠️) va
// AI "xodim kerak" degani (🆘). Ikkalasi ham bir xil tuzilishda
// bo'lishi kerak — xodim xabarni qayerdan boshlab o'qishni o'ylab
// o'tirmasin:
//
//	<belgi> <sarlavha>
//	Mijoz: <id>            (egasi so'ragandan farq qilsa — "(so'ragan: id)")
//	Suhbat: #<id>
//
//	<tana>
//
//	<reply haqida bir qator>
package support

import (
	"fmt"
	"strings"
)

// guruhSarlavha - hamma xabarning birinchi ikki-uch qatori.
//
// owner - buyurtma/murojaat egasi, client - uni so'ragan odam. Ikkalasi
// bir xil bo'lsa bitta id ko'rsatiladi.
func guruhSarlavha(title string, owner, client, conversationID int64) string {
	var b strings.Builder
	b.WriteString(title + "\n")
	b.WriteString(mijozSatri(owner, client))
	if conversationID > 0 {
		fmt.Fprintf(&b, "Suhbat: #%d\n", conversationID)
	}
	return b.String()
}

// mijozSatri - sarlavhadagi mijoz qatori. Buyurtma egasi bilan uni
// so'ragan odam har doim ham bir emas: mijoz chatda boshqa odamning DG
// raqamini yozgan bo'lishi mumkin. Shunday bo'lsa ikkalasi ham
// ko'rsatiladi — xodim kim bilan gaplashayotganini bilib tursin.
func mijozSatri(owner, client int64) string {
	if owner <= 0 {
		owner = client
	}
	if client > 0 && owner != client {
		return fmt.Sprintf("Mijoz: %d (so'ragan: %d)\n", owner, client)
	}
	return fmt.Sprintf("Mijoz: %d\n", owner)
}

// guruhFooter - xabarning oxirgi qatori: javob qanday beriladi.
// Hamma turda bir xil — reply qilinadi, javob mijozga moslab ketadi.
// ko'plik=true bo'lsa reply yuqoridagi hamma buyurtmaga tegishli.
func guruhFooter(koplik bool) string {
	s := "\nHal bo'lgach shu xabarga REPLY qilib yozing — " +
		"javobingiz mijozga moslab yuboriladi"
	if koplik {
		s += " (reply yuqoridagi buyurtmalarning hammasini yopadi)"
	}
	return s + "."
}
