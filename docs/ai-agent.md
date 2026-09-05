# AI agent, promt zanjiri va admin panel

Bu hujjat agentning ishlash tartibini va promt yozish qoidasini tushuntiradi.
Endpointlarning to'liq ro'yxati: `http://localhost:8080/docs`.

## 1. Umumiy oqim

```
Mijoz xabari (support chat)
        │
        ▼
 poller (har POLL_INTERVAL_SEC) ─ yoki ─ POST /api/agent/run
        │
        ▼
 promt #1 ──► Groq ──► JSON
        │                │
        │                ├── dashboard/adminka: true → kod tizimdan ma'lumot oladi
        │                ├── chat: mijozga javob
        │                ├── help: xodimlar guruhiga (Telegram)
        │                └── promt: keyingi promt id (null → tugadi)
        ▼
 keyingi promt (eng ko'pi AGENT_MAX_STEPS = 5 bosqich)
        │
        ▼
 help → Telegram guruhga DARHOL ketadi (tasdiq kutmaydi)
 chat → auto_reply YOQ ? mijozga darhol : tasdiqlash navbati (pending)
        │
        ▼
 admin "Tasdiqlash" bosadi → chat mijozga
        │
        ▼
 chat mijozga YETIB BORSA → o'sha xabarlar "o'qilgan" deb belgilanadi
```

## 1.0. Tasdiqlash nimaga tegishli

Tasdiqlash **faqat mijozga ketadigan `chat` javobiga** tegishli:

| Natija | Qayerga | Tasdiq kerakmi |
|---|---|---|
| `chat` | Mijozga (support chat) | `auto_reply` o'chiq bo'lsa — ha |
| `help` | Xodimlar guruhiga (Telegram) | **Yo'q** — zanjir tugashi bilan darhol ketadi |

Sabab: `help` mijozga ko'rinmaydi, xodimlar esa muammodan imkon qadar tez
xabardor bo'lishi kerak. Zanjir yarim yo'lda xato bilan to'xtasa ham, `help`
matni bo'lsa yuboriladi.

Faqat `help` qaytgan murojaat (mijozga yoziladigan matn yo'q) tasdiqlash
navbatiga umuman tushmaydi — status darhol `sent` bo'ladi.

## 1.1. Xabarlar qachon "o'qilgan" bo'ladi

Mijoz xabari **faqat unga javob berilganda** o'qilgan deb belgilanadi
(`PUT /api/v1/support.chat.message/read?ids=…`). Ya'ni:

| Holat | O'qilgan bo'ladimi |
|---|---|
| `chat` javobi mijozga yuborildi | ✅ ha |
| `help` ketdi, lekin `chat` yo'q | ❌ yo'q — mijozga javob berilmagan |
| Javob navbatda kutmoqda (`pending`) | ❌ yo'q |
| Admin rad etdi (`rejected`) | ❌ yo'q |
| Yuborishda xato bo'ldi (`failed`) | ❌ yo'q |

Belgilanadigan xabarlar — **oxirgi xodim javobidan keyin kelgan** mijoz
xabarlari (`interactions.message_ids`, masalan `"103,104"`). Ilgari javob
berilgan xabarlar qayta belgilanmaydi.

Belgilash o'zi xato bersa javob baribir yuborilgan hisoblanadi — murojaat
`failed` bo'lmaydi, logda ogohlantirish qoladi.

## 1.2. Suhbat qachon yopiladi

Mijozga javob yetib borgach suhbat support tizimida **"hal qilindi"**
holatiga o'tkaziladi (`PUT .../conversation/resolution/{id}`,
`{"resolution_state": 2}`). Tartib bitta joyda — `DeliverChat`:

```
xabar yuborildi → xabarlar o'qilgan deb belgilandi → suhbat yopildi
```

| Holat | Yopiladimi |
|---|---|
| `chat` mijozga yetib bordi (avto yoki tasdiqlangan) | ✅ ha |
| Xodimning Telegramdagi javobi mijozga yetkazildi | ✅ ha (u ham `DeliverChat` orqali ketadi) |
| Faqat `help` ketdi, mijozga xabar yo'q | ❌ yo'q |
| Javob navbatda kutmoqda / rad etilgan | ❌ yo'q |

Mijoz yopilgan suhbatga yana yozsa — support tizimining o'zi uni qayta
ochadi, biz aralashmaymiz.

Yopish xatosi javobni buzmaydi: murojaat `sent` bo'lib qolaveradi, xato
logga tushadi (`chat_resolved` esa `false` qoladi).

Panel sozlamasi: **"Javobdan keyin suhbatni yopish"** (`auto_resolve`,
default yoqilgan).

## 2. Promt yozish: model qaytaradigan JSON

Har bir promt matnida modeldan **faqat JSON** qaytarishni talab qiling.
Kod quyidagi kalitlarni tushunadi, qolganlari e'tiborsiz qoladi:

| Kalit | Turi | Kod nima qiladi |
|---|---|---|
| `dashboard` | bool | `true` → yetkazma (delivery) ma'lumoti olinadi va **keyingi bosqichga** beriladi |
| `adminka` | bool | `true` → daigou (Xitoy tomoni) buyurtmalari olinadi |
| `order_sn` | massiv | Adminka qidiruvi uchun DG raqamlari. Bo'sh bo'lsa mijozning barcha buyurtmalari |
| `express_num` | massiv | Yetkazma qidiruvi uchun trek raqamlari. Bo'sh bo'lsa mijozning barcha yetkazmalari |
| `chat` | satr | **Mijozga** yuboriladigan javob |
| `help` | satr | **Telegram guruhga** yuboriladigan matn (xodim aralashuvi kerak) |
| `tushunmadim` | bool | `true` → model mijoz muammosini tushunmadi. Kod savol berishdan oldin buyurtmalarni o'zi tekshiradi (pastga qarang) |
| `promt` | son yoki `null` | Keyingi promt `id`. `null` — zanjir tugadi |

Namuna:

```json
{
  "dashboard": true,
  "adminka": true,
  "order_sn": ["DG60607041"],
  "express_num": [],
  "chat": "",
  "help": "",
  "promt": 2
}
```

Qoidalar:
- `chat` va `help` bo'sh bo'lsa — o'sha harakat qilinmaydi.
- Ikkalasi ham bo'sh bo'lib zanjir tugasa — status `failed` (panelda ko'rinadi).
- `promt` mavjud bo'lmagan id yoki o'zini ko'rsatsa — zanjir to'xtaydi va xato yoziladi.
- Model matn ichida JSON qaytarsa yoki ```` ```json ```` ramkasiga o'rasa ham o'qiladi.

## 3. Zanjir qanday quriladi

Zanjir **promt #1** dan boshlanadi (`START_PROMPT_ID`). Har bosqichda modelga
suhbatning **oxirgi 10 ta xabari** JSON ro'yxat bo'lib ketadi — har biri
kim yozgani (`type`) bilan:

```
Suhbatning oxirgi xabarlari (eskisidan yangisiga). "type": "client" — mijoz
yozgan, "type": "agent" — biz yozgan javob:
[
  { "type": "client", "message": "DG60607041 что с этим заказом." },
  { "type": "agent",  "message": "Tekshiryapmiz…" }
]

Tizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):
{ "adminka": [ … ], "dashboard": [ … ] }
```

### Salom kuniga bir marta

Har bosqichdagi promt oxirida bitta qator salomlashish ko'rsatmasi ketadi
(`support/greeting.go`). Kod suhbat tarixiga qaraydi: **bugun** biz tomondan
(agent yoki xodim) hech narsa yozilmagan bo'lsa — modelga javobni salom
bilan boshlash aytiladi, aks holda "salomlashma, mavzuga o't" deyiladi.

Salom matni **bitta emas**: ko'rsatmada uchala variant birga beriladi
("Assalomu alaykum" / "Ассалому алайкум" / "Здравствуйте") va qaysi birini
olishni model mijozning tiliga qarab o'zi tanlaydi. Kod tilni aniqlamaydi —
ilgari faqat o'zbekcha lotin salom ketardi va ruscha yozgan mijozga ham
javob o'zbekchaga burilib ketardi. "Sizga qanday yordam bera olaman?"
savoli ham xuddi shunday uch tilda beriladi. Shu bilan yangi kun salom bilan boshlanadi, kun davomidagi
keyingi javoblarda esa salom takrorlanmaydi.

Salom — javobning **boshi**, o'zi emas: ko'rsatmada model salomdan keyin
mijoz muammosiga javob yozishi kerakligi ham aytiladi.

Xabar sanasi o'qib bo'lmasa u hisobga olinmaydi — shubhali holatda salom
beriladi (ortiqcha salom, tushib qolganidan yaxshiroq).

### Model tushunmasa — avval kod tekshiradi

Model mijoz nima so'rayotganini tushunmasa, darhol "buyurtma raqamingizni
yuboring" demaydi. Tartib shunday (`agent.go`, `runChain`):

1. Model javobiga `"tushunmadim": true` qo'yadi (eski promtlar uchun
   zaxira: chat matnining o'zi "Sizga qanday yordam bera olaman?" bo'lsa
   ham shu deb tushuniladi — `AgentJSON.IsUnclear`).
2. **Kod o'zi** adminka va dashboardni ko'radi — mijozning barcha
   buyurtmalari (suhbatdan topilgan raqamlar bilan birga).
3. **Kelmagan buyurtma bor bo'lsa** (to'langan, lekin yakunlanmagan
   adminka buyurtmasi yoki filialda olinmagan yetkazma —
   `HasPendingOrders`) o'sha ma'lumot **modelga qaytadan** beriladi:
   xuddi shu promt, endi buyurtmalar bilan. Mijoz katta ehtimol aynan
   o'sha buyurtma haqida yozgan bo'ladi.
4. **Kelmagan buyurtma yo'q bo'lsa** javob o'z holicha qoladi — ya'ni
   mijozdan buyurtma raqami so'raladi.

Fallback bir marta ishlaydi (`probed`) va oxirgi bosqichda umuman
ishlamaydi — qayta so'rashga bosqich qolmasligi kerak. Logda ko'rinadi:

```
agent: suhbat 60307 — model tushunmadi, kelmagan buyurtma topildi: qayta so'raldi
agent: suhbat 60307 — model tushunmadi, kelmagan buyurtma yo'q: buyurtma raqami so'raladi
```

Model shu bosqichda allaqachon `adminka`/`dashboard` so'ragan bo'lsa,
ma'lumot ikkinchi marta olinmaydi — o'sha natija ishlatiladi.

### Tizimdagi ma'lumot — saralangan

Modelga xom javob berilmaydi (bitta buyurtma ~10 KB). `support/context.go`
faqat javob yozish uchun kerakli maydonlarni qoldiradi — token ham
tejaladi, model ham chalkashmaydi:

```json
{
  "mijoz_turi": "B2C (oddiy mijoz)",
  "adminka": [
    { "order_sn": "DG60607041", "status_label": "sotib olingan, to'langan",
      "paid": true, "paid_at": "21-avgust", "days_since_paid": 8,
      "problem": true, "tekshiruvda": true,
      "express_num": "JT3172404674793", "shipped_at": "24-avgust" }
  ],
  "yetkazma": {
    "olinmagan": [
      { "express_num": "JT3172404674793", "filial": "Chirchiq",
        "manzil": "Toshkent viloyati, Chirchiq shahri…", "kelgan": "17-avgust" }
    ],
    "yetkazilmoqda": [
      { "express_num": "JT3172404674793", "filial": "SAHIY JIZZAX",
        "berilgan": "2-sentabr", "kun": 1 }
    ],
    "tekshirish_kerak": [
      { "express_num": "JT7639647959095", "filial": "SAHIY JIZZAX",
        "berilgan": "26-iyul", "kun": 40 }
    ]
  }
}
```

| Maydon | Qayerdan | Nima uchun |
|---|---|---|
| `mijoz_turi` | `skus[0].sku_info.B2C_percentage` (noldan katta → B2C, nol → B2B, buyurtma yo'q → noma'lum) | Yetkazish tarifini to'g'ri tushuntirish (promt #4) |
| `olinmagan` | yetkazmada `delivered: false` | Mijozga qaysi filialda ekanini aytish |
| `yetkazilmoqda` | `delivered: true`, **3 kungacha** | "Yo'lda, kuryer bog'lanadi" deb aytish |
| `tekshirish_kerak` | `delivered: true`, **3 kundan oshgan** | Holati noaniq — mijozdan so'rash va xodimga topshirish |

Sanalar odam o'qiydigan ko'rinishga o'tkaziladi ("21-avgust"), manzil,
ism va summa kabi maydonlar umuman yuborilmaydi. Har ro'yxatdan eng
ko'pi 5 ta yozuv ketadi (`MaxDeliveryRows`), yangisidan eskisiga.

### `delivered: true` — "yetkazildi" DEGANI EMAS

Bu maydon buyurtma **yetkazmaga berilganini** bildiradi, mijozning
qo'liga tekkanini emas. Shuning uchun (`support/context.go`, `DeliveryDays = 3`):

- **3 kungacha** — buyurtma yo'lda, `yetkazilmoqda` ga tushadi;
- **3 kundan oshsa** — holat noaniq: mijoz olgan bo'lishi ham, telefoni
  o'chiq bo'lgani uchun kuryer qaytargan bo'lishi ham mumkin. Bunday
  yozuv `tekshirish_kerak` ga tushadi, promt #3 esa mijozdan qo'liga
  tegdimi deb so'raydi va `help` orqali xodimga topshiradi;
- `delivered_at` **o'qilmasa** ham `tekshirish_kerak` ga tushadi —
  bilmaganni "yetkazildi" deb aytmaymiz.

Ilgari 3 kundan eski yozuvlar modelga **umuman ko'rsatilmasdi** (yashirilardi),
3 kun ichidagilari esa "topshirilgan" deb ko'rsatilardi — ikkalasi ham
noto'g'ri edi.

Uchala ro'yxat ham "mijoz hali qo'liga olmagan" hisoblanadi: model
"tushunmadim" deganda kod shu buyurtmalarni topib, unga qaytadan beradi.

`type` qiymatlari: **`client`** — mijoz yozgan, **`agent`** — biz tomondan
(AI yoki xodim) yuborilgan. Bo'sh matnli xabarlar tashlanadi. Modelga
10 tadan ortiq xabar hech qachon ketmaydi — `HISTORY_LIMIT` bilan faqat
kamaytirish mumkin.

Mijoz rasm yuborsa (xabar matni — havola), modelga `[rasm yuborildi]`
bo'lib ketadi: model rasmni ko'ra olmaydi, uzun havola esa token yeydi.
Promt #2 shu belgini ko'rsa rasmni qayta so'ramaydi.

### Xayrlashish ("rahmat", "hop")

Mijozning oxirgi so'zi minnatdorchilik yoki rozilik bo'lsa
(`support/farewell.go`), zanjir **umuman yurmaydi**: savol yo'q, tizimdan
ma'lumot ham kerak emas. Javob kodda tayyor va mijozning tilida beriladi:

| Xabar | Javob |
|---|---|
| `rahmat`, `hop`, `mayli`, `ok` | `FarewellUzLat` |
| `раҳмат`, `хоп`, `майли` | `FarewellUzCyr` |
| `спасибо`, `хорошо`, `понял` | `FarewellRU` |

Til mijoz yozgan **o'sha so'zdan** aniqlanadi — taxmin yo'q. Tili
bilinmaydigan so'z ("ok", "👍") uchun o'zbekcha lotin, kirill yozuvda esa
rus tili olinadi.

`IsClosingMessage` qattiq: xabar **faqat** shu so'zlardan (va "katta",
"большое" kabi to'ldiruvchilardan) iborat bo'lishi kerak. Savol belgisi,
raqam, notanish so'z yoki 5 tadan ortiq so'z bo'lsa — bu odatdagi murojaat
va zanjir yuradi. Ya'ni "rahmat, lekin qachon keladi?" modelga ketadi.
Rasm ham yakunlash hisoblanmaydi.

Panelda bunday murojaat 1 bosqich va **0 token** bilan ko'rinadi
(bosqich nomi — "Xayrlashish (model chaqirilmadi)"). Yuborish odatdagi
qoida bo'yicha: `auto_reply` yoqilgan bo'lsa darhol, aks holda tasdiq
kutadi.

### Rasmdan raqam o'qish

Mijoz ko'pincha buyurtma raqamini yozmay, skrinshot yoki chek rasmini
tashlaydi. Asosiy model rasmni ko'rmaydi, shuning uchun rasm alohida
o'qiladi — **tesseract (OCR) bilan, modelsiz** (`support/ocr.go`).

Buyurtma (`DG…`) va trek raqamlari bosma matn: OCR ularni bepul, lokal va
~0.1 soniyada o'qiydi. Rasm vaqtincha faylga yuklanadi, `tesseract <fayl>
stdout` chaqiriladi, matndan raqamlar `numbers.go` regexp'lari bilan
ajratiladi, fayl o'chiriladi. **Token sarflanmaydi.**

Ko'ruvchi (vision) modelga **umuman borilmaydi**: rasm hech qachon LLM ga
yuborilmaydi. OCR raqam topmasa — raqam mijozdan matn bilan so'raladi.

`OCR_ENABLED=false` bo'lsa rasmga umuman qaralmaydi. tesseract
o'rnatilmagan bo'lsa (`ErrNoOCR`) zanjir to'xtamaydi — raqam mijozdan
so'raladi. Docker image'da tesseract bor (`Dockerfile`).

Funksiyalar:

| Funksiya | Nima qiladi |
|---|---|
| `ReadImageOCR(ctx, url)` | Rasmni yuklab, tesseract bilan o'qiydi |
| `OCRAvailable()` | tesseract shu mashinada bormi |
| `ClientImageLinks(msgs)` | Mijoz yuborgan rasm havolalari, eng oxirgisi birinchi |
| `HasClientImage(msgs)` | Mijoz rasm yuborganmi |
| `ReadNumbersFromMessages(ctx, msgs)` | Rasmlarni o'qiydi; `(raqamlar, topildimi)` qaytaradi |

Zanjirda (`agent.go`) rasm **faqat mijoz matnda raqam yozmaganda**
o'qiladi — matnda raqam bo'lsa rasm ortiqcha ish. Natija ikki xil:

- **topildi** (`ok == true`) → raqamlar `chatSN`/`chatEx` ga qo'shiladi va
  birinchi promtga "rasmdan o'qilgan raqamlar" qatori bilan kiriladi
  (model raqamni qayta so'ramaydi);
- **topilmadi** (`ok == false`) → "Tizimdagi ma'lumot" blokiga
  "RASMDAN BUYURTMA RAQAMI CHIQMADI" ko'rsatmasi tushadi, model rasm
  mazmuniga tayanmaydi va raqamni mijozdan so'raydi. Sabab uchta bo'lishi
  mumkin (rasm yo'q, o'qilmadi, ichida raqam yo'q) — keyingi qadam
  uchalasida bir xil, o'qish xatolari logga yoziladi.

Har bir rasm o'qish **suhbat tafsilotida alohida bosqich** bo'lib ko'rinadi
(promt raqamisiz, nomi — "Rasmni o'qish — tesseract eng"): o'qilgan
rasmning o'zi, OCR ning xom matni va natija ("TOPILDI: DG…" yoki
"RASMDAN BUYURTMA RAQAMI CHIQMADI"). Token sarflanmagani uchun bosqichda
token soni ko'rsatilmaydi.

Sozlama: `MAX_IMAGES` (default 2) — bitta suhbatda nechta rasm o'qiladi,
eng oxirgisidan boshlab. Qolganlari: `OCR_ENABLED`, `OCR_LANGS`,
`OCR_TIMEOUT_SEC`, `OCR_FETCH_TIMEOUT_SEC`, `TESSERACT_BIN`.

Xabar **sanasi yuborilmaydi**: tartib yetarli, sana esa token sarflaydi va
model javobida chalkashlik keltiradi. Haqiqiy sanalar (buyurtma yaratilgan,
jo'natilgan) "Tizimdagi ma'lumot" blokida keladi.

Til haqidagi ko'rsatma modelga **kod tomonidan qo'shilmaydi** — uni
promtning o'zi aytadi (har bir promtning eng boshida "TIL QOIDASI" bloki
turadi). Blok ikki maydonni ajratib aytadi: **`chat` — mijozning tilida**,
**`help` — har doim o'zbekcha** (u xodimlar guruhiga ketadi). Ilgari bu
ajratilmagandi va "ikki tilni aralashtirma" qoidasi `help` ning o'zbekchasi
bilan qo'shilib, ruscha mijozga o'zbekcha javob yozilib qolardi — ayniqsa
2-promtda, chunki u yerda `help` HAR DOIM to'ldiriladi. Ilgari kod alifboni o'zi aniqlab qo'shardi, lekin mijozning
oxirgi xabari rasm bo'lganda (`[rasm yuborildi]`) noto'g'ri til
tanlanardi.

"Tizimdagi ma'lumot" bloki faqat oldingi bosqichda `dashboard`/`adminka`
so'ralgan bo'lsa qo'shiladi. Ya'ni odatiy ikki bosqich:

1. **#1 — kategoriya va ma'lumot so'rash**: mijoz nima so'rayotganini aniqlaydi,
   `dashboard`/`adminka` va raqamlarni qaytaradi, `promt: 2`.
2. **#2 — javob yozish**: kelgan ma'lumotga tayanib `chat` yozadi, `promt: null`.

Ko'proq bosqich kerak bo'lsa (masalan alohida "muammoli buyurtma" yoki
"pul qaytarish" promti) — yangi promt yarating va oldingi promtda uning
id'sini ko'rsating.

## 3.1. Muammoli buyurtmalar

Adminka status kodlari:

| status | ma'nosi |
|---|---|
| 3 | sotib olingan, to'langan |
| 4 | kiritish uchun kutilmoqda |
| 6 | yakunlangan |

**Qoida:** buyurtma **to'langan** (`pay_status = 1`) bo'lib, status 3 yoki 4
bo'lib, **to'lov sanasidan (`paid_at`) `PROBLEM_DAYS` (3) kundan ko'p**
o'tgan bo'lsa — muammoli.

`paid_at` bo'sh bo'lsa (eski yozuvlar) `created_at` ga qaytiladi.
**To'lanmagan buyurtma hech qachon muammoli hisoblanmaydi** — u kutib
turishi normal, xodim aralashuvi kerak emas.

Bu qarorni **kod** chiqaradi, model emas: modelga sana ayirmasini ishonib
bo'lmaydi. Modelga tayyor maydonlar beriladi:

```json
{ "order_sn": "DG60645244", "status_label": "sotib olingan, to'langan",
  "paid": true, "paid_at": "2026-08-21 10:00:00",
  "days_since_paid": 9, "problem": true, "tekshiruvda": true }
```

Nima bo'ladi:

| Holat | Mijozga | Guruhga |
|---|---|---|
| `paid: false` | "to'lov hali o'tmagan" | — |
| `problem: true` | "buyurtmangiz tekshirilmoqda" | ⚠️ muammo xabari (avtomatik, tasdiqsiz) |
| status 3/4, `problem: false` | "tez orada yo'lga chiqadi" | — |

**Bitta mijoz — bitta xabar.** Bir mijozning bir necha buyurtmasi birdan
muammoli bo'lsa, guruhga ularning har biri uchun alohida emas, hammasi
raqamlangan ro'yxat bo'lib **bitta** xabarda ketadi (mijoz va suhbat
raqami sarlavhada bir marta yoziladi). Takroriy eslatmalar ham xuddi
shunday: eslatma vaqti kelgan buyurtmalar mijoz bo'yicha guruhlanib
bitta xabarga yig'iladi.

**Hal qilish — Telegram guruhdagi reply orqali.** Bot yozgan xabarga xodim
reply qilsa, o'sha matn yechim bo'lib saqlanadi (`resolved_via: telegram`,
`resolved_by: @username`) va bot "✅ … hal qilindi" deb tasdiqlaydi.
Xabarda bir nechta buyurtma bo'lsa, bitta reply **hammasini** yopadi va
mijozga ham bitta javob tayyorlanadi (promt #5 ga hamma buyurtma raqami
birga uzatiladi).

**Xodim javobi mijozga ham yetadi.** Reply matni promt #5 orqali mijoz
tiliga moslab qayta yoziladi (ichki atamalarsiz, xushmuomala) va odatdagi
qoida bo'yicha ketadi: `auto_reply` yoqiq bo'lsa darhol mijozga, aks
holda tasdiqlash navbatiga. Bunday javoblar panelda `source: telegram`
belgisi bilan ko'rinadi. Guruhdagi tasdiqda holat ham yoziladi
("mijozga yuborildi" / "admin tasdig'i kutilmoqda").

**Javobda buyurtma raqami bo'ladi.** Modelga "javob matnida `order_sn`
ni albatta yoz" deb aytiladi, model tashlab ketsa esa kod o'zi qo'shadi
(`WithOrderSN`): matnda yo'q raqamlar boshiga qo'yiladi —
`DG60607041 — Buyurtmangiz ertaga jo'natiladi.` Matnda allaqachon bor
raqam takrorlanmaydi. Mijoz javob qaysi buyurtmasi haqida ekanini bilishi
kerak, ayniqsa bitta xabarda bir nechta buyurtma yopilganda.

LLM ishlamay qolsa javob YO'QOLMAYDI: xodim matni o'z holicha qoralama
bo'lib navbatga tushadi — u ham buyurtma raqami bilan.
Guruh javoblari `TG_POLL_SEC` (30 s) da bir marta `getUpdates` bilan
o'qiladi; oxirgi `update_id` `settings` jadvalida saqlanadi.

**Takroriy eslatma** — har siklda `ReviewOpenIssues` ochiq muammolarni
qayta ko'radi va faqat shundan keyin eslatma yuboradi:

1. **Xodim mijozga chatda javob berganmi** → bergan bo'lsa muammo
   yopiladi (`resolved_via: chat`) va eslatma yuborilmaydi. AI agentning
   o'z javobi (`AGENT_SENDER_ID` bilan yozilgan) bunga kirmaydi —
   "tekshirilmoqda" degan javob muammoni hal qilmaydi.
2. Adminkadagi holat o'zgarganmi → o'zgargan bo'lsa avtomatik yopiladi
   (`resolved_via: auto`, guruhga "✅ holat o'zgardi" deb yoziladi).
3. `ISSUE_REMIND_HOURS` (24 soat) o'tgan bo'lsa — eslatma yuboriladi;
   reply endi yangi xabarga qilinadi.

**Yopilgan muammo qayta ko'tarilmaydi** — buyurtma hali ham qotib tursa
ham. Faqat adminkadagi **holat o'zgargan** bo'lsa (masalan 3 → 4) yangi
muammo ochiladi. Shu sababli guruhga bir xil buyurtma haqida takror
xabar ketmaydi.

Qo'lda: `POST /api/issues/review`, paneldan yopish:
`POST /api/issues/{id}/resolve`.

Kunlik hisobot: `GET /api/stats/issues/daily` + `/api/stats` dagi
`issues_open`, `issues_opened_today`, `issues_resolved_today`,
`issues_avg_hours` — dashboardda "Bugungi hisobot" bloki.

Xuddi shu blokda murojaatlarning bugungi kesimi ham bor: `total_today`
(bugun kelgan), `sent_today` (AI o'zi yuborgan), `approved_today` (admin
tasdiqlab yuborgan), `rejected_today`. **Tasdiqlash `sent_at` bo'yicha
sanaladi, `created_at` bo'yicha emas** — kecha kelgan murojaatni bugun
tasdiqlasangiz, u bugungi ishga kiradi. Kunlik jadval (`/api/stats/daily`)
esa aksincha, murojaat **kelgan** kun bo'yicha guruhlanadi — shuning uchun
bu ikki raqam bir-biriga teng bo'lmasligi normal.

### Javob berilgan suhbat qayta ishlanmaydi

Suhbatdagi oxirgi so'z **biz tomondan** bo'lsa, zanjir umuman
yurmaydi (`ErrAlreadyAnswered`): mijoz javob kutmayapti, qayta ishlash
esa behuda token va takroriy javob (hatto muammoni qayta ko'tarish)
demakdir. Bu tekshiruv `RunChain` ning ichida — fon sikli ham,
`POST /api/agent/run` ham unga bo'ysunadi.

Qo'lda majburan ishga tushirish kerak bo'lsa:
`POST /api/agent/run {"conversation_id":123,"force":true}`.

## 4. Sozlamalar

Panel orqali (`PUT /api/settings`, darhol kuchga kiradi — qayta ishga
tushirish shart emas):

Agentni tezda to'xtatish:

```sh
curl -X PUT http://localhost:8080/api/settings \
  -H "Authorization: Bearer $TOKEN" -d '{"agent_enabled":false}'
```


| Sozlama | Ma'nosi |
|---|---|
| `agent_enabled` | **AI agentni to'xtatish tugmasi.** `false` — zanjir umuman yurmaydi: fon sikli ham, `/api/agent/run` ham modelga bormaydi, token sarflanmaydi, bazaga yangi yozuv qo'shilmaydi. Navbatdagi tayyor javoblarni tasdiqlash va yuborish ishlayveradi |
| `auto_reply` | `true` — mijozga javob (chat) tasdiqsiz ketadi; `false` — chat navbatda kutadi. `help` ga ta'sir qilmaydi |
| `poll_enabled` | Fon siklini yoqish/o'chirish |
| `auto_resolve` | Javobdan keyin suhbatni "hal qilindi" holatiga o'tkazish (default yoqilgan) |
| `poll_interval_sec` | Sikllar orasidagi oraliq, 10–3600 s (sekinlashtirish uchun oshiring) |
| `batch_size` | Bitta siklda nechta suhbat, 1–50 (qolganlari keyingi siklda) |
| `chat_delay_sec` | Suhbatlar orasidagi tanaffus, 0–600 s |

**Tezlikni sekinlashtirish** panel orqali: Sozlamalar → "Ishlash tezligi".
Masalan `poll_interval_sec=300`, `batch_size=2`, `chat_delay_sec=20` —
har 5 daqiqada 2 ta suhbat, orasida 20 soniya tanaffus. O'zgarish darhol
kuchga kiradi, API'ni qayta ishga tushirish shart emas. `.env` dagi
`POLL_INTERVAL_SEC` / `RATE_LIMIT_COUNT` endi faqat boshlang'ich qiymat.

**`batch_size` ni oshirish har doim ham tezlashtirmaydi.** U faqat
*yuqori chegara*: poller avval ro'yxatni filtrlaydi (`operator_unseen_count
> 0` va oxirgi xabar vaqti o'zgargan bo'lsa), keyin shundan `batch_size`
tasini oladi. Filtrdan 3 ta suhbat o'tsa, `batch_size=20` ham 3 tani
ishlaydi. Har siklda logga aynan shu yoziladi:

```
poller: 3 ta javobsiz suhbat, shundan 3 tasi ishlanadi
```

Birinchi son doim kichik bo'lsa — muammo `batch_size` da emas, filtrda.

### Hamma mijozni ketma-ket ko'rib chiqish — `POST /api/agent/scan`

Filtrni chetlab o'tib, ro'yxatdagi **hamma** suhbatni ko'rish uchun:

```bash
curl -X POST http://localhost:8080/api/agent/scan \
  -H "Authorization: Bearer $TOKEN" -d '{"max":50}'
```

`support.chat.conversation/filter` dan suhbatlar olinadi (`pages`/`limit`
berilmasa `CHATS_PAGES`/`CHATS_LIMIT`), eng yangi xabardan boshlab
**ketma-ket** har biri uchun zanjir yuritiladi. `operator_unseen_count`
filtri ham, `batch_size` chegarasi ham qo'llanmaydi; `max` (0 — hammasi)
bilan cheklanadi va `chat_delay_sec` tanaffusi saqlanadi.

Oxirgi so'z biz tomondan bo'lgan suhbat modelga **bormaydi**
(`ErrAlreadyAnswered`) — token sarflanmaydi. Ish uzoq davom etadi, shuning
uchun fonda bajariladi: so'rov darhol `202` qaytaradi, natija navbatda va
logda ko'rinadi:

```
skaner: 128 ta suhbat olindi, 50 tasi ko'riladi
skaner: tugadi — 12 zanjir, 36 javob berilgan, 2 xato (jami 128)
```

Bir vaqtda faqat bitta skaner yuradi (ikkinchi so'rov `409` oladi).

`.env` orqali (qayta ishga tushirish kerak):

| Kalit | Default | Ma'nosi |
|---|---|---|
| `GROQ_API_KEY` | — | **Majburiy.** Groq kaliti |
| `GROQ_MODEL` | `openai/gpt-oss-120b` | Model JSON rejimini qo'llashi shart |
| `GROQ_MAX_TOKENS` | 800 | Javobning eng ko'p tokeni |
| `GROQ_PRICE_IN` / `GROQ_PRICE_OUT` | — | 1 mln token uchun USD (xarajat hisobi) |
| `START_PROMPT_ID` | 1 | Zanjir qaysi promtdan boshlanadi |
| `AGENT_MAX_STEPS` | 5 | Eng ko'p bosqich |
| `HISTORY_LIMIT` | 10 | Modelga ketadigan oxirgi xabarlar (10 dan oshmaydi) |
| `POLL_INTERVAL_SEC` | 60 | Fon sikli oralig'i |
| `CHATS_LIMIT` | 30 | Bir siklda ko'riladigan suhbatlar |
| `RATE_LIMIT_COUNT` | 5 | Bir siklda ishlanadigan suhbatlar (qolgani keyingi siklda) |
| `AGENT_SENDER_ID` | — | **Majburiy.** Agent chatda qaysi id bilan yozadi |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_GROUP_ID` | — | `help` va muammoli buyurtmalar shu guruhga ketadi |
| `TG_POLL_SEC` | 30 | Guruhdagi reply'lar necha soniyada tekshiriladi |
| `HTTP_RETRIES` | 2 | Vaqtinchalik xatolarda (429, 5xx, Cloudflare 522) qo'shimcha urinishlar |
| `HTTP_RETRY_DELAY_MS` | 2000 | Urinishlar orasidagi birinchi tanaffus (har safar ikkilanadi) |
| `HTTP_RETRY_MAX_MS` | 20000 | Kutishning yuqori chegarasi (`Retry-After` uzun bo'lsa ham) |
| `PROBLEM_DAYS` | 3 | To'lovdan necha kun o'tsa muammoli |
| `PROBLEM_STATUSES` | `3,4` | Kuzatiladigan statuslar |
| `ISSUE_REMIND_HOURS` | 24 | Eslatma oralig'i |
| `CORS_ORIGIN` | `http://localhost:5173` | Frontend manzili |

## 5. Baza jadvallari

| Jadval | Nima saqlaydi |
|---|---|
| `promts` | Promt matnlari (zanjir id bo'yicha yuradi) |
| `interactions` | Har bir murojaat: mijoz xabari, chat/help javobi, status, tokenlar, xarajat, `message_ids` va `read_marked` |
| `agent_steps` | Zanjirning har bosqichi: modelga ketgan matn va asl javob |
| `conversation_states` | Poller qaysi suhbatni qayergacha ishlagani |
| `order_issues` | Muammoli buyurtmalar: qachon aniqlangan, guruhga necha marta yozilgan, qanday va kim tomonidan hal qilingan |
| `settings` | `agent_enabled`, `auto_reply`, `poll_enabled`, `tg_update_offset` |
| `users` | Panel foydalanuvchilari (bcrypt parol) |

## 5.1. Vaqtinchalik uzilishlar

Sahiy API'lari Cloudflare orqasida va vaqti-vaqti bilan **522** (origin
javob bermayapti) qaytaradi; Groq bepul tarifda **429** (tezlik chegarasi)
beradi. Shunday javoblarda kod avtomatik qayta uriniladi
(`HTTP_RETRIES`, tanaffus har safar ikkilanadi: 2s → 4s). Server
`Retry-After` sarlavhasini yuborsa (Groq 429 da yuboradi) o'shanga
amal qilinadi, lekin `HTTP_RETRY_MAX_MS` dan oshmaydi — juda uzoq
kutgandan ko'ra murojaatni keyingi siklga qoldirgan ma'qul.

Qayta urinilmaydi:
- **4xx** (401 kabi) — token yaroqsiz bo'lsa takrorlash foydasiz;
- **xabar yuborish** (`/api/v2/chat/send`, Telegram `sendMessage`) —
  birinchi urinish aslida o'tib ketgan bo'lsa mijozga ikki marta xabar
  ketib qolishi mumkin.

## 6. Tashqi so'rovlar

| Yo'nalish | So'rov |
|---|---|
| Suhbatlar | `POST {BASE_URL}/api/v1/support.chat.conversation/filter` |
| Xabarlar | `GET {BASE_URL}/api/v1/support.chat.message/conversation/{id}` |
| **Javob yuborish** | `POST {BASE_URL}/api/v2/chat/send` |
| **O'qilgan deb belgilash** | `PUT {BASE_URL}/api/v1/support.chat.message/read?ids=1,2,3` |
| **Suhbatni yopish** | `PUT {BASE_URL}/api/v1/support.chat.conversation/resolution/{id}` |
| Daigou buyurtmalar | `GET {USER_BASE_URL}/api/admin/daigou-orders` |
| Yetkazma | `POST {SERVICE_BASE_URL}/api/v2/admin/delivery/orders/filter` |
| AI | `POST {GROQ_BASE_URL}/chat/completions` |
| Telegram (yuborish) | `POST https://api.telegram.org/bot{TOKEN}/sendMessage` |
| Telegram (javoblarni o'qish) | `GET https://api.telegram.org/bot{TOKEN}/getUpdates` |

## 7. O'lik `.env` kalitlari

Quyidagilarni hech qaysi kod o'qimaydi (eski versiyalardan qolgan):
`AI_PROVIDER`, `OLLAMA_*`, `AI_PRICE_*`, `AI_BUDGET_USD`, `ESCALATE_MARKER`,
`AUTO_REPLY` (endi bazadagi `auto_reply` sozlamasi), `CONTEXT_BEFORE`,
`MAX_MESSAGE_AGE_HOURS`, `API_ID`, `API_HASH`, `TG_PHONE`, `TG_SESSION`,
`ALLOWED_GROUPS`, `BACKFILL`, `WEB_PORT`.
