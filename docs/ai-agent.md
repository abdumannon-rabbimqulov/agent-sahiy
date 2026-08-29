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
 auto_reply YOQ  → chat mijozga, help Telegramga darhol ketadi (status: sent)
 auto_reply O'CHIQ → tasdiqlash navbati (status: pending) → admin "Tasdiqlash" bosadi
```

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
ketadi:

```
Suhbat:
MIJOZ: DG60607041 что с этим заказом.

Tizimdagi ma'lumot (faqat shunga tayan, o'zingdan to'qima):
{ "adminka": [ … ], "dashboard": [ … ] }
```

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
| `auto_reply` | `true` — AI javobi tasdiqsiz ketadi; `false` — hammasi navbatda kutadi |
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
| `HISTORY_LIMIT` | 10 | Modelga ko'rsatiladigan oxirgi xabarlar |
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
| `interactions` | Har bir murojaat: mijoz xabari, chat/help javobi, status, tokenlar, xarajat |
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
