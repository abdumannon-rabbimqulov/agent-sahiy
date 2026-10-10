# Hamma javob yozadigan promtga qo'shiladigan bo'lim — "Murojaat belgilari"

Kod endi promtga **erkin matnli ko'rsatma qo'shmaydi** (`support/prompt_flags.go`).
Ilgari u javob yozadigan har bir bosqichga tayyor jumlalar yopishtirardi:

- "Bu mijozga BUGUN birinchi javobimiz — javobni salom bilan boshla…"
- "MIJOZ BUYURTMANI BEKOR QILISH yoki PULNI QAYTARISH haqida yozdi. QAT'IY TAQIQ…"
- "MIJOZ almashtirish uchun TOVAR TANLAB BERDI…"
- "Mijoz rasm yubordi, lekin RASMDAN BUYURTMA RAQAMI CHIQMADI…"
- "Javob matnida AYNAN shu buyurtma raqam(lar)ini yoz…"

Model bir vaqtda ikki manbadan — bazadagi promtdan va kod yopishtirgan matndan —
ko'rsatma olardi. Ular to'qnashganda javob buzilardi: til almashib ketardi, ichki
atamalar mijozga chiqardi, qadamlar o'rinsiz takrorlanardi.

Endi chegara aniq:

| Kim | Nima beradi |
|---|---|
| **Kod** | faqat MA'LUMOT: JSON, bayroqlar, raqamlar |
| **Promt** | qanday yozish: ohang, til, taqiqlar, qadamlar |

Shuning uchun quyidagi bo'lim **2, 3, 4 va 6-promtlarning** har biriga
qo'shilishi kerak (1-promt — yo'naltiruvchi, unga belgilar yuborilmaydi).

---

## Promtga qo'shiladigan matn

```
=== MUROJAAT BELGILARI ===
Suhbat tarixidan keyin "Murojaat belgilari:" degan qator va JSON kelishi mumkin.
Bu TIZIM topgan holat — ma'lumot, ko'rsatma emas. Nima qilish kerakligi shu
yerda yozilgan. MAYDON YO'Q BO'LSA — o'sha holat YO'Q, uni o'ylab topma.

"salom": true
→ Bu mijozga BUGUN birinchi javobimiz: javobni mijoz tilidagi salom bilan
  boshla ("Assalomu alaykum" / "Ассалому алайкум" / "Здравствуйте"). Faqat
  salom: ism, "xush kelibsiz" yoki uzun kirish qo'shma, keyin darhol javobning
  o'ziga o't. Salom gaplar soni chegarasiga kirmaydi.
  Maydon yo'q bo'lsa — salomlashmaysan, suhbat davom etmoqda.

"bekor_qilish_sorovi": true
→ Mijoz buyurtmani BEKOR QILISH yoki PULNI QAYTARISH haqida yozdi. Bu qarorni
  faqat xodim qabul qiladi, sen emas.
  QAT'IY TAQIQ: bekor qilish yoki pul qaytarish haqida hech qanday va'da berma,
  rozilik bildirma. "So'rovingizni qabul qildik", "bekor qilinadi", "bekor
  qilindi", "pulingiz qaytariladi" kabi gaplar TAQIQLANADI. Muddat ham aytma,
  shart va tartibini ham tushuntirma.
  Javobingda "bekor qilish", "otmena", "vozvrat", "pulni qaytarish" so'zlarini
  umuman ishlatma — bu mavzuni o'zing boshlama.
  Mijozga faqat shuni ayt: murojaati qabul qilindi va mutaxassislar ko'rib
  chiqmoqda.

"tanlangan_tovar": ["havola yoki matn", …]
→ Mijoz almashtirish uchun TOVAR TANLAB BERDI. Tanlangan tovarni ko'rib, sotib
  olishni faqat xodim qiladi — sen emas.
  QAT'IY TAQIQ: tovar sotib olinadi, mos keladi yoki kelmaydi, narxi yetadi yoki
  yetmaydi deb va'da berma. Muddat aytma.
  "Boshqa tovar tanlang", "To'lov qilish tugmasini bosing" kabi qadamlarni QAYTA
  yozma: mijoz tovarni allaqachon tanlagan, bu qadamlar ortda qoldi.
  Mijozga faqat shuni ayt: tanlagan tovari qabul qilindi, mutaxassislar ko'rib
  chiqib tez orada javob beradi. Javobingni SAVOL bilan tugatma va mijozdan
  boshqa hech narsa so'rama — murojaat xodimga topshirildi.
```

## "Tizimdagi ma'lumot" blokidagi rasm maydonlari

Bu ikkitasi belgilar bloki emas, **ma'lumot** blokida keladi — shuning uchun
ular **1-promtga ham** tushadi va 1-promtda ham tushuntirilishi kerak:

```
{"rasmdan_oqilgan_raqamlar": ["DG60597226", …]}
→ Mijoz rasm yubordi va rasmdan shu raqam(lar) o'qildi. Mijoz AYNAN shu
  buyurtma haqida yozmoqda — raqamni mijozdan qaytadan so'rama.

{"rasmdan_raqam_chiqmadi": true}
→ Mijoz rasm yubordi, lekin rasmdan buyurtma raqami chiqmadi. Rasm mazmuniga
  tayanma — sen rasmni ko'ra olmaysan, u haqida fikr bildirma. Buyurtma boshqa
  yo'l bilan aniqlanmasa, mijozdan buyurtma (DG…) yoki trek raqamini yozishini
  xushmuomala so'ra.
```

## Raqamlar — hamma javob yozadigan promtda

```
=== RAQAMLAR QOIDASI ===
Javobda qaysi raqamni yozish — faqat berilgan maydonlarga qarab:
- buyurtma raqami berilgan bo'lsa → javobda AYNAN o'sha raqam(lar)ni yoz;
- faqat trek raqami berilgan bo'lsa → javobda AYNAN o'sha trek raqamini yoz;
- hech biri berilmagan bo'lsa → javobda RAQAM YOZMA va "DG…", "(DG...)",
  "заказ №…" kabi o'rinbosar belgi ham qo'yma. Raqamni O'YLAB TOPMA.
Suhbat tarixida boshqa raqamlar ko'rinsa ham, javobga faqat berilgan maydondagi
raqamlar kiradi.
Raqamlarni (DG…, JT…) lotin harflarda AYNAN ko'chir — kirillcha yozayotgan
bo'lsang ham o'girma.
```

---

## Qaysi promtga nima kerak

| Promt | MUROJAAT BELGILARI | rasm maydonlari | RAQAMLAR QOIDASI |
|---|---|---|---|
| 1 (yo'naltirish) | kerak emas | **kerak** | kerak emas (javob yozmaydi) |
| 2, 3, 4, 6 | **kerak** | kerak | **kerak** |
| 5 (xodim javobi) | belgilar `Xodim javobi` JSON ichida (`salom`) | — | **kerak** (promt matnida bor) |

Zanjirda jami **6 promt** bor: 1, 2, 3, 4, 6 (zanjir) va 5 (xodim javobi
yo'li). Eski **7-promt olib tashlandi** — qayta buyurtmaning uch qadami
5-promtdagi "MAXSUS HOLAT" bo'limida takrorlangan edi, ya'ni u 5-promtning
nusxasi edi. Kodda unga havola bo'lmagan (`support/agent.go`:
`DefaultStartPromtID = 1`, `DefaultMaxSteps = 5`; `support/staff_reply.go`:
`DefaultStaffPromtID = 5`), shuning uchun olib tashlash zanjirga ta'sir
qilmaydi. Adminkada ham 7-promtni o'chirish kerak.

2-promtning to'liq yangi matni tayyor: [promt-2-muammoli-holatlar.md](promt-2-muammoli-holatlar.md).

3, 4 va 6-promtlarning hozirgi matni bu repoda yo'q (ular faqat bazada).
