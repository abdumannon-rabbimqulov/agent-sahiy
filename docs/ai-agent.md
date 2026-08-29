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
  { "type": "client", "message": "DG60607041 что с этим заказом.",
    "created_at": "2026-08-29T10:16:30Z" },
  { "type": "agent",  "message": "Tekshiryapmiz…",
    "created_at": "2026-08-29T10:18:00Z" }
]

Tizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):
{ "adminka": [ … ], "dashboard": [ … ] }
```

`type` qiymatlari: **`client`** — mijoz yozgan, **`agent`** — biz tomondan
(AI yoki xodim) yuborilgan. Bo'sh matnli xabarlar tashlanadi. Modelga
10 tadan ortiq xabar hech qachon ketmaydi — `HISTORY_LIMIT` bilan faqat
kamaytirish mumkin.

"Tizimdagi ma'lumot" bloki faqat oldingi bosqichda `dashboard`/`adminka`
so'ralgan bo'lsa qo'shiladi. Ya'ni odatiy ikki bosqich:

1. **#1 — kategoriya va ma'lumot so'rash**: mijoz nima so'rayotganini aniqlaydi,
   `dashboard`/`adminka` va raqamlarni qaytaradi, `promt: 2`.
2. **#2 — javob yozish**: kelgan ma'lumotga tayanib `chat` yozadi, `promt: null`.

Ko'proq bosqich kerak bo'lsa (masalan alohida "muammoli buyurtma" yoki
"pul qaytarish" promti) — yangi promt yarating va oldingi promtda uning
id'sini ko'rsating.

## 4. Sozlamalar

Panel orqali (`PUT /api/settings`, darhol kuchga kiradi):

| Sozlama | Ma'nosi |
|---|---|
| `auto_reply` | `true` — mijozga javob (chat) tasdiqsiz ketadi; `false` — chat navbatda kutadi. `help` ga ta'sir qilmaydi |
| `poll_enabled` | Fon siklini yoqish/o'chirish |

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
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_GROUP_ID` | — | `help` shu guruhga ketadi |
| `CORS_ORIGIN` | `http://localhost:5173` | Frontend manzili |

## 5. Baza jadvallari

| Jadval | Nima saqlaydi |
|---|---|
| `promts` | Promt matnlari (zanjir id bo'yicha yuradi) |
| `interactions` | Har bir murojaat: mijoz xabari, chat/help javobi, status, tokenlar, xarajat, `message_ids` va `read_marked` |
| `agent_steps` | Zanjirning har bosqichi: modelga ketgan matn va asl javob |
| `conversation_states` | Poller qaysi suhbatni qayergacha ishlagani |
| `settings` | `auto_reply`, `poll_enabled` |
| `users` | Panel foydalanuvchilari (bcrypt parol) |

## 6. Tashqi so'rovlar

| Yo'nalish | So'rov |
|---|---|
| Suhbatlar | `POST {BASE_URL}/api/v1/support.chat.conversation/filter` |
| Xabarlar | `GET {BASE_URL}/api/v1/support.chat.message/conversation/{id}` |
| **Javob yuborish** | `POST {BASE_URL}/api/v2/chat/send` |
| **O'qilgan deb belgilash** | `PUT {BASE_URL}/api/v1/support.chat.message/read?ids=1,2,3` |
| Daigou buyurtmalar | `GET {USER_BASE_URL}/api/admin/daigou-orders` |
| Yetkazma | `POST {SERVICE_BASE_URL}/api/v2/admin/delivery/orders/filter` |
| AI | `POST {GROQ_BASE_URL}/chat/completions` |
| Telegram | `POST https://api.telegram.org/bot{TOKEN}/sendMessage` |

## 7. O'lik `.env` kalitlari

Quyidagilarni hech qaysi kod o'qimaydi (eski versiyalardan qolgan):
`AI_PROVIDER`, `OLLAMA_*`, `AI_PRICE_*`, `AI_BUDGET_USD`, `ESCALATE_MARKER`,
`AUTO_REPLY` (endi bazadagi `auto_reply` sozlamasi), `CONTEXT_BEFORE`,
`MAX_MESSAGE_AGE_HOURS`, `API_ID`, `API_HASH`, `TG_PHONE`, `TG_SESSION`,
`ALLOWED_GROUPS`, `BACKFILL`, `WEB_PORT`.
