# Tashqi so'rovlar — to'liq ro'yxat

Agent tashqariga chiqadigan **hamma** HTTP so'rovi shu yerda. Har biri uchun:
qaysi manba, to'liq URL, qaysi Go funksiyasi yuboradi, javobdan nima olinadi.

Uchta boshqa-boshqa server bor va ular **alohida login** talab qiladi:

| Belgi | Server | Base URL | env | Standart |
|---|---|---|---|---|
| **S** | Support (yordam xizmati) | `BASE_URL` | `BASE_URL` | `https://api.market.sahiy.uz` |
| **D** | Delivery (O'zbekistondagi yetkazma) | `SERVICE_BASE_URL` | `SERVICE_BASE_URL` | `https://api.sahiy.uz` |
| **A** | Adminka / daigou (Xitoy tomoni) | `USER_BASE_URL` | `USER_BASE_URL` | `https://api.sahiy.uz` |

Pastdagi jadvallarda URL'lar shu base'ga nisbatan yozilgan.

---

## 1. Ma'lumot olinadigan GET so'rovlar

Bularning uchalasi ham `internal/sources` orqali o'tadi va
`/api/source/*` endpointlarimiz aynan shularni chaqiradi
(qarang: `docs/get-endpointlar.md`).

### 1.1. Yetkazma buyurtmalari — **D**

```
GET /api/v2/admin/delivery/orders/filter
    ?page=1&size=20&delivered={true|false}&{track_number|user_id}={qiymat}
```

- Kod: `internal/orders/orders.go` — `FilterPath()`, `fetch()`, `ByTrack()`, `ByUser()`
- Auth: `Authorization: Bearer` (service login, 1.4-band)
- **Bitta qidiruv = ikkita so'rov.** `delivered` majburiy filtr: usiz server
  doim `NO ORDERS FOUND` qaytaradi. Shuning uchun `delivered=false` va
  `delivered=true` alohida so'raladi va natijalar birlashtiriladi.
- Qidiruv maydonlari: `track_number` (track / express raqami) yoki
  `user_id` (= support'dagi `client_id`).
- Javob: `{"ret":..,"msg":..,"data":{"orders":[...],"total_items":N}}`.
  Buyurtma topilmasa `data` obyekt emas, **bo'sh massiv** `[]` bo'lib keladi.
- Olinadigan maydonlar (`orders.Order`): `id`, `order_id`, `user_id`,
  `full_name`, `phone`, `city`, `express_num`, `status`, `delivered`,
  `payment_status`, `payment_fee`, `weight`, `created_at`, `paid_at`,
  `delivered_at`, `comment`, `station.{name,address}`,
  `delivery_address.{branch_name,branch_address}`. Qolgani tashlanadi.
- Modelga: `orders.Summary()` — eng yangi **5 tasi**.

### 1.2. Adminka (daigou) buyurtmalari — **A**

```
GET /api/admin/daigou-orders
    ?page=&size=&status=&keyword=&platform=&user_id=&order_sn=
    &express_num=&begin_date=&end_date=
```

- Kod: `internal/daigou/daigou.go` — `Fetch()`, `Search()`, `ByOrderSN()`,
  `ByExpressNum()`, `ByUser()`
- Auth: `Authorization: Bearer {ADMINKA_TOKEN_BEARER}` (login yo'q, token
  qo'lda `.env` ga qo'yiladi)
- Barcha parametrlar har doim yuboriladi (bo'shlari serverda e'tiborga
  olinmaydi). Agent uchtasidan bittasini to'ldiradi: `order_sn` (DG...),
  `express_num` (track) yoki `user_id`.
- Sahifalash: `page` berilmasa `meta.last_page` gacha aylanadi, lekin
  **eng ko'pi 5 sahifa** (`maxPages`).
- Javob shakli **barqaror emas**: buyurtmalar `data`, `data.data`,
  `data.list`, `data.rows`, `data.items` ichida bo'lishi mumkin — shuning
  uchun xom `map` sifatida o'qiladi va maydonlar nom/yo'l bo'yicha qidiriladi
  (`FindOrders`, `Pick`).
- Olinadigan maydonlar (`daigou` dagi `rows` jadvali — terminal, HTTP javobi
  va model uchun yagona manba): `order_sn`, `user_id`, `status`, `amount`,
  `receiver_name`, `province`, `area`, `sub_area`, `street`, `express_line`,
  `express_num`, `package_name`, `quantity`, `created_at`, `shipped_at`,
  `packed_at`, `in_storage_at`, `client_type`.
- Modelga: `daigou.Summary()` — yangisidan eskisiga saralanib, **5 tasi**.

### 1.3. Suhbat xabarlari — **S**

```
GET /api/v1/support.chat.message/conversation/{conversation_id}?page=1&limit={N}
```

- Kod: `internal/support/messages.go` — `FetchMessages()`
- Auth: `Authorization: Bearer` (admin login, 1.4-band)
- `limit` — `HISTORY_LIMIT` dan kam bo'lmaydi (aks holda oxirgi N ta xabar
  to'liq yig'ilmaydi).
- Javob: `{"data":[ ... ]}`.
- Olinadigan maydonlar (`support.Message`): `id`, `sender_id`, `sender_type`
  (`client`/`agent`), `conversation_id`, `message`, `content`, `status`,
  `support_field`, `seen_at`, `created_at`, `updated_at`.
- Rasm: server `content` da `"image"` deb yozadi, URL esa `message` da keladi.
- Modelga: `Window()` bilan kesilgan oyna → `TranscriptAfter()`.

### 1.4. Loginlar (token olish)

Ikkalasi ham POST, lekin GET'lar shularsiz ishlamaydi.

| Server | So'rov | Kod | Token qayerda |
|---|---|---|---|
| **S** | `POST /api/v1/admins/login` | `internal/auth/auth.go` | `token.json` |
| **D** | `POST /api/v2/service/user/login` | `internal/service/service.go` | `service-token.json` |

Muddat: **S** — JWT `exp` dan 60s ayirib; **D** — `expires_in` (standart 24
soat) dan 5 daqiqa ayirib. 401 kelsa fayl o'chiriladi va qayta login qilinadi.
Batafsil: `docs/model-va-cache.md`.

### 1.5. Suhbatlar ro'yxati (POST, lekin o'qish uchun) — **S**

```
POST /api/v1/support.chat.conversation/filter?page={p}&limit={n}
body: {"type":"client","state":[1,2,3],"client_id":N}
```

- Kod: `internal/support/support.go` — `FetchConversations()`, `SearchByClient()`
- Server bu ro'yxatni GET emas, POST bilan beradi — o'zgartirmaydi, faqat
  filtrlaydi.
- Olinadigan maydonlar (`support.Conversation`): `id`, `client_id`,
  `seller_id`, `agent_id`, `title`, `message`, `conversation_type`, `status`,
  `state`, `resolution_state`, `unseen_count`, `operator_unseen_count`,
  `created_at`, `seller_name`, `client_name`.

---

## 2. Yozadigan so'rovlar (agentning javob qaytarish yo'li)

Bular ma'lumot olmaydi — ro'yxat to'liq bo'lishi uchun keltirilgan.
**`internal/sources` va `/api/source/*` bularning birortasini chaqirmaydi.**

| Metod | URL | Server | Kod | Nima qiladi |
|---|---|---|---|---|
| POST | `/api/v2/chat/send` | S | `internal/support/send.go` | Mijozga javob yuboradi |
| PUT | `/api/v1/support.chat.message/read?ids=1,2,3` | S | `internal/support/read.go` | "O'qildi" belgisi |
| PUT | `/api/v1/support.chat.conversation/resolution/{id}` | S | `internal/support/actions.go` | Suhbat holatini yopadi |
| DELETE | `/api/v1/support.chat.message/{id}` | S | `internal/support/actions.go` | Xabarni o'chiradi |
| POST | `/api/v1/storage.upload` | S | `internal/support/storage.go` | Fayl/rasm yuklaydi |

---

## 3. Model serverlari

| Metod | URL | Kod | Izoh |
|---|---|---|---|
| POST | `{OLLAMA_URL}/api/chat` | `internal/ollama/ollama.go` | Asosiy model (`llama3.1:8b`), standart `http://localhost:11434` |
| GET | `{OLLAMA_URL}/api/tags` | `internal/ollama/ollama.go` | Ishga tushganda bir marta: server tirikmi, model yuklanganmi |
| POST | `{GROQ_BASE_URL}/chat/completions` | `internal/groq/groq.go` | Zaxira model (`openai/gpt-oss-120b`), standart `https://api.groq.com/openai/v1` |

Batafsil: `docs/model-va-cache.md`.

Telegram MTProto orqali ishlaydi (`gotd`, `internal/userbot`) — HTTP so'rov emas.

---

## 4. Qisqacha xarita

```
so'rov                                     →  bizning endpoint
────────────────────────────────────────────────────────────────
D  GET  delivery/orders/filter (×2)        →  GET /api/source/delivery
A  GET  admin/daigou-orders (≤5 sahifa)    →  GET /api/source/daigou
S  GET  support.chat.message/conversation  →  GET /api/source/support
S  POST support.chat.conversation/filter   ┘  (client_id bo'yicha suhbat topish)

hammasi birga                              →  GET /api/source/all
```
