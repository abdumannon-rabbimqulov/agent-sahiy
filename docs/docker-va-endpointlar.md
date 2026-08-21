# Lokal endpointlar: boshidan oxirigacha + Docker

Bu hujjat **hozirgi kodni** yozib qo'yadi (`main.go` va `support/` paketi).
Har bir bandda manba fayl ko'rsatilgan — shubha bo'lsa kodning o'zidan
tekshirsa bo'ladi. Pastdagi `curl` javoblari haqiqiy: Docker'da ishlab
turgan konteynerdan olingan (ism/telefon kabi shaxsiy ma'lumot yashirilgan).

> Eski `docs/get-endpointlar.md` va `docs/tashqi-sorovlar.md` boshqa
> (`internal/...`) tuzilma haqida yozilgan — bu repoga to'g'ri kelmaydi.

## 1. Umumiy chizma

Bitta Go jarayoni 4 ta lokal endpoint ochadi (`main.go:209-212`) va ularni
**uchta boshqa-boshqa tashqi serverga** ulaydi. Har server o'z autentifikatsiyasi
bilan ishlaydi:

```
                    ┌──────────────────────────────────────────────────┐
  curl / brauzer    │  sahiy-api  (Docker, :8080)                      │
        │           │                                                  │
        ├─ POST /api/chats ──── chatsHandler ─────┐                     │
        ├─ GET  /api/messages ─ messagesHandler ──┤ token.json          │
        │           │                             ▼                     │
        │           │                    S: api.market.sahiy.uz        │
        │           │                       (admin login)              │
        │           │                                                  │
        ├─ GET  /api/orders ─── ordersHandler ────► A: api.sahiy.uz     │
        │           │                    .env dagi ADMINKA_TOKEN_BEARER│
        │           │                                                  │
        └─ GET  /api/dashboard ─ dashboardHandler ► D: api.sahiy.uz     │
                    │                    service-token.json            │
                    └──────────────────────────────────────────────────┘
```

| Belgi | Server | Base URL (env) | Standart | Auth |
|---|---|---|---|---|
| **S** | Support (yordam xizmati) | `BASE_URL` | `https://api.market.sahiy.uz` | admin login → `token.json` |
| **A** | Adminka / daigou (Xitoy tomoni) | `USER_BASE_URL` | `https://api.sahiy.uz` | qo'lda qo'yilgan bearer |
| **D** | Delivery (O'zbekistondagi yetkazma) | `SERVICE_BASE_URL` | `https://api.sahiy.uz` | service login → `service-token.json` |

Qisqa jadval:

| Lokal endpoint | Metod | Handler | Support funksiyasi | Tashqi so'rov | Manba |
|---|---|---|---|---|---|
| `/api/chats` | POST | `chatsHandler` (`main.go:43`) | `support.ChatsJSON` | `POST /api/v1/support.chat.conversation/filter` | S |
| `/api/messages` | GET | `messagesHandler` (`main.go:82`) | `support.MessagesJSON` | `GET /api/v1/support.chat.message/conversation/{id}` | S |
| `/api/orders` | GET | `ordersHandler` (`main.go:123`) | `support.OrdersJSON` | `GET /api/admin/daigou-orders` | A |
| `/api/dashboard` | GET | `dashboardHandler` (`main.go:158`) | `support.DeliveryJSON` | `GET /api/v2/admin/delivery/orders/filter` (×2) | D |

Bundan tashqari uchta hujjat yo'li bor (ular tashqariga chiqmaydi, faqat
o'zimizning spec'ni ko'rsatadi): `/docs`, `/redoc`, `/openapi.json` — 10-bandga
qarang.

Har to'rttasi ham **faqat o'qiydi**: hech narsa yozilmaydi, "o'qildi"
qo'yilmaydi, xabar yuborilmaydi.

Umumiy qoida (har handlerda bir xil):

1. metod tekshiriladi → mos kelmasa **405**;
2. parametrlar o'qiladi → yetmasa **400**;
3. token olinadi (cache yoki login) → bo'lmasa **502**;
4. tashqi API chaqiriladi;
5. `ErrUnauthorized` (401) kelsa — token yangilanib **bir marta** qayta
   uriniladi;
6. xato bo'lsa **502**, aks holda tayyor JSON qaytadi.

---

## 2. `POST /api/chats` — suhbatlar ro'yxati

**Nega POST?** Support serveri bu yo'lni GET bilan bermaydi (405 qaytaradi),
filtr esa body'da ketadi (`support/chat.go:14-16`). Ma'lumot o'zgarmaydi —
bu shunchaki o'sha serverning shakli.

### So'rov

```
POST /api/chats
Content-Type: application/json

{"client_id": 8396106, "page": 1, "limit": 10}
```

| Maydon | Majburiymi | Standart | Ma'nosi |
|---|---|---|---|
| `client_id` | yo'q | 0 | 0 bo'lsa — hamma suhbatlar; >0 bo'lsa faqat o'sha mijozniki |
| `page` | yo'q | 1 | sahifa |
| `limit` | yo'q | 10 | nechta suhbat |

Body umuman bo'sh bo'lsa ham ishlaydi (`main.go:50` — `ContentLength == 0`
tekshiriladi), u holda hammasi standart qiymatda ketadi.

### Zanjir

1. `chatsHandler` (`main.go:43`) body'ni `support.ChatFilter` ga o'qiydi.
2. `support.CredentialsFromEnv()` (`support/login.go:34`) — `.env` dan
   `LOGIN`, `PASSWORD`, `LOGIN_FIELD`, `BASE_URL`.
3. `support.Token(creds, support.TokenFile)` (`support/login.go:100`) —
   avval `token.json`, bo'lmasa login.
4. `support.ChatsJSON` → `FetchChats` (`support/chat.go:39`) tashqariga:
   ```
   POST {BASE_URL}/api/v1/support.chat.conversation/filter?page=1&limit=10
   Authorization: Bearer <token>
   {"type":"client","state":[1,2,3], "client_id": <agar berilgan bo'lsa>}
   ```
5. Javobdan `data.chats` olinadi, har suhbatdan **4 ta maydon** qoladi
   (`support/chat.go:22-27`): `id`, `client_id`, `created_at`,
   `ms_created_at`. Qolgani tashlanadi.

`id` — keyingi endpointga (`/api/messages?conversation_id=`) beriladigan
qiymat; `client_id` esa `/api/orders?user_id=` va
`/api/dashboard?user_id=` ga beriladi. Ya'ni **chats — zanjirning boshi**.

### Haqiqiy javob

```sh
curl -sS -X POST localhost:8080/api/chats -d '{"page":1,"limit":3}'
```
```json
{
  "count": 3,
  "chats": [
    { "id": 57295, "client_id": 8396106,
      "created_at": "2026-08-12T23:28:57.289438Z",
      "ms_created_at": "2026-08-18T18:00:41.812867Z" },
    { "id": 58064, "client_id": 7903808,
      "created_at": "2026-08-18T19:31:20.841266Z",
      "ms_created_at": "2026-08-19T11:25:56.756768Z" }
  ]
}
```

### Xatolar

| Holat | Status | Javob |
|---|---|---|
| `GET /api/chats` | 405 | `{"error":"faqat POST"}` |
| body JSON emas | 400 | `{"error":"body JSON emas: ..."}` |
| login bo'lmadi | 502 | `{"error":"login: ..."}` |
| tashqi API xatosi | 502 | `{"error":"suhbatlar ro'yxati (status N): ..."}` |

---

## 3. `GET /api/messages` — suhbat ichidagi xabarlar

### So'rov

```
GET /api/messages?conversation_id=58064&limit=10
```

| Parametr | Majburiymi | Standart |
|---|---|---|
| `conversation_id` | **ha** (musbat son) | — |
| `limit` | yo'q | 10 (`support.DefaultMessageLimit`) |

### Zanjir

1. `messagesHandler` (`main.go:82`) `conversation_id` ni parse qiladi,
   noto'g'ri yoki ≤0 bo'lsa 400.
2. Token — `/api/chats` bilan **bir xil** (`token.json`, o'sha admin login).
3. `support.MessagesJSON` → `FetchMessages` (`support/messages.go:29`):
   ```
   GET {BASE_URL}/api/v1/support.chat.message/conversation/58064?page=1&limit=10
   Authorization: Bearer <token>
   ```
4. Server yangi xabarni birinchi qilib beradi — kod `id` bo'yicha qayta
   saralaydi va **oxirgi `limit` tasini** eskisidan yangisiga qoldiradi
   (`support/messages.go:77-81`). Ya'ni javobdagi tartib — suhbatning
   tabiiy tartibi.
5. Har xabardan **3 ta maydon**: `message`, `sender_type` (`client` yoki
   agent), `created_at`.

### Haqiqiy javob

```sh
curl -sS 'localhost:8080/api/messages?conversation_id=58064&limit=4'
```
```json
{
  "conversation_id": 58064,
  "count": 3,
  "messages": [
    { "message": "как забать товар и где узнать код для получения",
      "sender_type": "client", "created_at": "2026-08-18T19:32:01.736189Z" },
    { "message": "???",
      "sender_type": "client", "created_at": "2026-08-19T11:25:56.756768Z" }
  ]
}
```

### Xatolar

| Holat | Status | Javob |
|---|---|---|
| `POST` | 405 | `{"error":"faqat GET"}` |
| `conversation_id` yo'q / noto'g'ri | 400 | `{"error":"conversation_id berilmagan"}` |
| login / tashqi API | 502 | `{"error":"..."}` |

---

## 4. `GET /api/orders` — adminka (daigou) buyurtmalari

Bu **Xitoy tomonidagi** ma'lumot: nima sotib olingan, qachon yo'lga chiqqan,
trek raqami qaysi. "Buyurtmam qachon keladi?" savoliga shu manba ishlatiladi.

### So'rov

```
GET /api/orders?user_id=7903808&page=1&size=10
GET /api/orders?order_sn=DG60597226
GET /api/orders?express_num=79021428785596
```

| Parametr | Ma'nosi |
|---|---|
| `user_id` | Ilova Profil ID = support'dagi `client_id` |
| `order_sn` | buyurtma raqami (`DG...`) |
| `express_num` | Xitoydagi trek/posilka raqami |
| `status`, `keyword` | ixtiyoriy qo'shimcha filtr |
| `page` / `size` | standart 1 / 10 |

`user_id`, `order_sn`, `express_num`, `keyword` — **hech biri berilmasa 400**
(`main.go:142`). Qolgan bo'sh parametrlar baribir yuboriladi, serverda
e'tiborga olinmaydi (`support/adminka.go:92-104`).

### Zanjir

1. `ordersHandler` (`main.go:123`) query'dan `support.OrderFilter` yig'adi.
2. `support.AdminkaFromEnv()` (`support/adminka.go:29`) — `USER_BASE_URL` va
   `ADMINKA_TOKEN_BEARER`. **Login yo'q**: token qo'lda `.env` ga qo'yiladi
   (boshidagi `Bearer ` prefiksi kesib tashlanadi).
3. `support.OrdersJSON` → `FetchOrders` (`support/adminka.go:76`):
   ```
   GET {USER_BASE_URL}/api/admin/daigou-orders?page=..&size=..&user_id=..&order_sn=..&express_num=..
   Authorization: Bearer <ADMINKA_TOKEN_BEARER>
   ```
4. **Javob shakli barqaror emas.** Shuning uchun xom `map` sifatida o'qiladi:
   - buyurtmalar massivi `findList` bilan qidiriladi: `data`, `data.data`,
     `data.list`, `data.rows`, `data.items` (`support/adminka.go:273`);
   - har maydon `get("a.b.0.c")` yo'li bilan olinadi, bir nechta ehtimoliy
     joydan birinchi bo'sh bo'lmagani `first(...)` bilan tanlanadi
     (`support/adminka.go:210-241`). Masalan `express_num` to'rt xil yo'lda
     uchraydi.
5. Bir buyurtmaning xom javobi ~10 KB — undan **17 ta maydon** qoladi
   (`support/adminka.go:54-72`): `order_sn`, `user_id`, `status`, `amount`,
   `receiver_name`, `province`, `area`, `sub_area`, `street`, `express_line`,
   `express_num`, `package_name`, `quantity`, `created_at`, `shipped_at`,
   `packed_at`, `in_storage_at`.

`quantity` — SKU'lar bo'yicha qo'shib chiqiladi (`support/adminka.go:189`).

### Haqiqiy javob

```sh
curl -sS 'localhost:8080/api/orders?user_id=7903808&size=2'
```
```json
{
  "count": 2,
  "orders": [
    { "order_sn": "DG60597226", "user_id": 7903808, "status": 6,
      "amount": "174.05", "receiver_name": "К***",
      "province": "Toshkent shahri", "area": "Toshkent shahri",
      "sub_area": "Yunusobod tumani", "street": "***",
      "express_line": "Auto cargo-Pickup", "express_num": "79021428785596",
      "package_name": "F08-5606010 ... Chery Jet", "quantity": 1,
      "created_at": "2026-07-25 21:24:31", "shipped_at": "2026-07-30 12:21:29",
      "packed_at": "2026-07-28 15:32:53", "in_storage_at": "2026-07-28 15:32:53" },
    { "order_sn": "DG60555680", "user_id": 7903808, "status": 10,
      "amount": "104.38", "express_num": "", "shipped_at": "" }
  ]
}
```

Ikkinchi buyurtmada `express_num`, `shipped_at` bo'sh — buyurtma hali yo'lga
chiqmagan. Ya'ni **bo'sh maydon xato emas, holatning o'zi**. `status` — raqam
(6 — yakunlangan).

### Xatolar

| Holat | Status |
|---|---|
| `POST` | 405 `{"error":"faqat GET"}` |
| hech qanday qidiruv maydoni yo'q | 400 |
| `ADMINKA_TOKEN_BEARER` bo'sh | 502 `{"error":"ADMINKA_TOKEN_BEARER berilmagan"}` |
| token eskirgan (401) | 502 — **avtomatik yangilanmaydi**, `.env` ni qo'lda yangilash kerak |

---

## 5. `GET /api/dashboard` — yetkazma (delivery) buyurtmalari

Bu **O'zbekiston tomonidagi** ma'lumot: posilka qaysi filialda, qaysi
postamatda, kimga tegishli.

### So'rov

```
GET /api/dashboard?user_id=7903808
GET /api/dashboard?track=79021428785596     # track_number ham qabul qilinadi
```

| Parametr | Ma'nosi |
|---|---|
| `user_id` | = support'dagi `client_id` |
| `track` yoki `track_number` | trek raqami (`main.go:166-169` — ikkalasi ham qabul qilinadi) |
| `page` / `size` | standart 1 / 20 |

Ikkalasidan hech biri bo'lmasa — 400.

### Zanjir

1. `dashboardHandler` (`main.go:158`) `support.DeliveryFilter` yig'adi.
2. `support.ServiceFromEnv()` (`support/dashboard.go:40`) — `.env` dagi
   `SERVICE_*`. **Device maydonlari majburiy**: ularsiz login 500 qaytaradi.
3. `support.ServiceToken(svc, support.ServiceTokenFile)`
   (`support/dashboard.go:129`) — `service-token.json`, bo'lmasa service login.
   Bu **alohida server, alohida login, alohida cache fayl** — support tokeni
   bu yerda ishlamaydi.
4. `support.DeliveryJSON` → `FetchDelivery` (`support/dashboard.go:177`):
   ```
   GET {SERVICE_BASE_URL}/api/v2/admin/delivery/orders/filter
       ?page=1&size=20&delivered=false&{track_number|user_id}=..
   GET ... &delivered=true&...
   Authorization: Bearer <service token>
   ```
   ⚠️ **Bitta qidiruv = ikkita so'rov.** `delivered` majburiy filtr: usiz
   server doim "buyurtma yo'q" qaytaradi. Shuning uchun `false` va `true`
   alohida so'raladi, natijalar birlashtiriladi.
5. Buyurtma topilmasa `data` obyekt emas, **bo'sh massiv `[]`** bo'lib keladi —
   kod buni xato deb hisoblamaydi, shunchaki bo'sh ro'yxat qaytaradi
   (`support/dashboard.go:243-249`).
6. Har buyurtmadan **8 ta maydon** (`support/dashboard.go:163-172`):
   `full_name`, `phone`, `address`, `location_number`, `express_num`,
   `branch_name`, `created_at`, `user_id`. `branch_name` ba'zan yuqorida,
   ba'zan `delivery_address` ichida — `first(...)` bilan olinadi.

### Haqiqiy javob

```sh
curl -sS 'localhost:8080/api/dashboard?track=79021428785596'
```
```json
{
  "count": 1,
  "orders": [
    { "full_name": "К***", "phone": "998*******28",
      "address": "Yunusobod tumani", "location_number": "Pastamat shota",
      "express_num": "79021428785596", "branch_name": "SHOTA",
      "created_at": "2026-08-14T09:26:51.000000Z", "user_id": 7903808 }
  ]
}
```

`?user_id=7903808` ham aynan shu buyurtmani beradi — ikki manba `express_num`
orqali bog'lanadi (adminkadagi `express_num` = delivery'dagi `express_num`).

### Xatolar

| Holat | Status |
|---|---|
| `POST` | 405 |
| `user_id` ham, `track` ham yo'q | 400 |
| `SERVICE_PHONE`/`SERVICE_PASSWORD` yo'q | 502 `{"error":"service login: ..."}` |
| 401 yoki 403 | token yangilanadi, bir marta qayta uriniladi |

---

## 6. Uchta endpoint bir-biriga qanday ulanadi

Bitta mijozning muammosini oxirigacha ko'rish tartibi:

```
POST /api/chats                          → chats[].id, chats[].client_id
      │                    │
      │ id                 │ client_id
      ▼                    ▼
GET /api/messages    GET /api/orders?user_id=...   → order.express_num
  ?conversation_id=..        │
                             │ express_num
                             ▼
                     GET /api/dashboard?track=...  → qaysi filial/postamat
```

- `chats.id` → `messages.conversation_id` (mijoz nima deb yozgan);
- `chats.client_id` → `orders.user_id` **va** `dashboard.user_id` (bir xil
  raqam, ikki serverda ikki xil nomda);
- `orders.express_num` → `dashboard.track` (Xitoydagi posilka ↔ O'zbekistondagi
  yetkazma).

---

## 7. Autentifikatsiya va token cache

Uchta manba — uchta boshqacha usul:

### S — Support admin login (`token.json`)

- `POST {BASE_URL}/api/v1/admins/login` (`support/login.go:51`)
  body: `{"<LOGIN_FIELD>": LOGIN, "password": PASSWORD}`; `LOGIN_FIELD`
  standarti `login` (`username`/`phone` ham bo'lishi mumkin).
- Javobdan token moslashuvchan izlanadi: `data.token`, `token`,
  `access_token`, `accessToken`, `jwt` (`support/login.go:133`).
- Muddat JWT ichidagi `exp` dan olinadi, 60 soniya zaxira bilan; `exp`
  topilmasa `FallbackTTL = 30 daqiqa` (`support/login.go:151`).
- `token.json` ga `0600` ruxsat bilan yoziladi (`support/cache.go:36`).

### D — Service login (`service-token.json`)

- `POST {SERVICE_BASE_URL}/api/v2/service/user/login`
  (`support/dashboard.go:59`) body'da telefon, parol **va device maydonlari**.
- Muddat `expires_in` dan (ba'zan son, ba'zan matn bo'lib keladi — `json.Number`
  bilan o'qiladi), 5 daqiqa zaxira ayriladi; kelmasa 24 soat.

### A — Adminka (cache yo'q)

- Login endpointi ishlatilmaydi: token qo'lda `.env` dagi
  `ADMINKA_TOKEN_BEARER` ga qo'yiladi. Muddati tugasa faqat `.env` ni
  yangilash yordam beradi.

### Cache mantig'i (`support/cache.go`)

| Qadam | Nima bo'ladi |
|---|---|
| `Token()` / `ServiceToken()` | fayl bor va muddati o'tmagan bo'lsa — o'sha token |
| fayl yo'q / buzuq / muddati o'tgan | login qilinadi va fayl qayta yoziladi |
| tashqi API 401 qaytardi | `DropToken` — fayl o'chadi, qayta login, so'rov **bir marta** qaytariladi |

Qayta urinish faqat bir marta bo'ladi: ikkinchi 401 to'g'ridan-to'g'ri 502
bo'lib qaytadi (`main.go:65-72`, `106-113`, `186-193`).

---

## 8. Docker'da ishlatish

Fayllar: `Dockerfile` (ikki bosqichli: `golang:1.26-alpine` → `alpine`),
`docker-compose.yml`, `.dockerignore`.

```sh
docker compose up -d --build     # yig'ish va ishga tushirish
docker compose logs -f api       # loglar
docker compose ps                # holat
docker compose down              # to'xtatish
```

Ishga tushgan log:

```
tinglanmoqda: :8080 — POST /api/chats, GET /api/messages?conversation_id=..,
GET /api/orders?user_id=.., GET /api/dashboard?user_id=..
```

### Muhim tafsilotlar

- **`.env`** konteynerga `./.env:/app/.env:ro` qilib ulanadi. Kod uni ish
  katalogidan o'qiydi (`loadEnv(".env")`, `main.go:202`) va **mavjud muhit
  o'zgaruvchisi ustidan yozmaydi** — ya'ni compose'dagi `environment:`
  qiymatlari ustunroq.
- **Tokenlar** `sahiy-tokens` nomli volume'da (`/app`) saqlanadi. Shu sababli
  konteyner qayta ishga tushganda qaytadan login qilinmaydi:
  ```sh
  docker compose exec api ls -l /app
  # -rw-------  service-token.json
  # -rw-------  token.json
  ```
- **Port**: konteyner ichida doim `:8080`. Tashqi portni o'zgartirish uchun
  `docker-compose.yml` dagi `ports: "8090:8080"` ni tahrirlang.
- **CA sertifikatlari** runtime image'ga qo'shilgan (`ca-certificates`) —
  usiz uchala tashqi API ham HTTPS'da yiqilardi.
- Kod bog'liqliksiz (`go.mod` da faqat modul nomi), shuning uchun build tez.

### Docker'siz

```sh
go run .        # o'sha .env, o'sha token fayllari, o'sha :8080
```

---

## 9. Nozik joylar

- **Basic Auth YO'Q.** Bu endpointlar hech qanday parol so'ramaydi va
  mijozlarning shaxsiy ma'lumotini (ism, telefon, manzil) qaytaradi.
  Shuning uchun `docker-compose.yml` da port **faqat `127.0.0.1` ga**
  bog'langan. Tashqariga chiqarish kerak bo'lsa oldiga
  reverse-proxy + auth qo'ying — `"8080:8080"` deb yozilsa port mashinaning
  hamma interfeysida ochiladi.
- `.env`, `token.json`, `service-token.json` `.gitignore` da va
  `.dockerignore` da — image ichiga **kirmaydi**, faqat mount orqali keladi.
- `/api/chats` faqat POST — bu bizning tanlovimiz emas, support serveri GET'ga
  405 beradi.
- `/api/dashboard` har chaqiruvda tashqariga **ikkita** so'rov yuboradi
  (`delivered=false` va `true`).
- Adminka javobining shakli versiyaga qarab o'zgaradi — shuning uchun
  `findList` / `first(...)` yo'llari bor. Yangi maydon kerak bo'lsa
  `pickOrder` (`support/adminka.go:161`) ga yo'l qo'shiladi.
- Barcha tashqi so'rovlarda timeout bor: login 20s, ma'lumot so'rovlari 30s.
- Sahifalash `count` bilan cheklangan — tashqi API'ning `total` maydoni
  qaytarilmaydi. Ko'proq kerak bo'lsa `page`/`size`/`limit` ni oshiring.

---

## 10. Avtomatik hujjat: `/docs` (FastAPI'dagi kabi)

Go'da FastAPI'dek "o'zi paydo bo'ladigan" hujjat yo'q — spec qo'lda yoziladi,
lekin ko'rinishi va ishlashi bir xil: brauzerda ochib, **Try it out** bilan
so'rov yuborsa bo'ladi.

| Yo'l | Nima |
|---|---|
| `GET /docs` | Swagger UI — sinab ko'rish mumkin bo'lgan interaktiv sahifa |
| `GET /redoc` | O'sha spec, o'qishga qulayroq ko'rinishda |
| `GET /openapi.json` | OpenAPI 3.0.3 spec — Postman/Insomnia'ga import qilinadi |
| `GET /` | `/docs` sahifasini beradi |
| boshqa yo'l | 404 `{"error":"bunday yo'l yo'q — /docs ga qarang"}` |

```sh
open http://localhost:8080/docs
curl -sS localhost:8080/openapi.json > sahiy-openapi.json   # Postman uchun
```

### Qanday ishlaydi

- Spec **qo'lda yozilgan** `openapi.json` faylida (repo ildizida). To'rtala
  endpoint, ularning parametrlari, javob sxemalari va haqiqiy misollar shu
  yerda.
- `docs_api.go` uni `//go:embed` bilan binar ichiga qo'shadi — konteynerga
  alohida fayl ko'chirilmaydi, `openapi.json` build paytida binar ichiga kiradi.
- `/docs` va `/redoc` — kichik HTML sahifalar; Swagger UI / ReDoc fayllari
  CDN'dan (unpkg, cdn.redoc.ly) olinadi. Ya'ni **brauzerda internet kerak**;
  `/openapi.json` esa internetsiz ham ishlaydi.
- Yo'llar `main.go:215-218` da ro'yxatdan o'tadi.

### Endpoint qo'shsangiz

Spec o'zi yangilanmaydi (bu Go — tiplardan avtomatik chiqmaydi). Tartib:

1. `main.go` ga handler va `http.HandleFunc` qo'shiladi;
2. `openapi.json` ga o'sha yo'l `paths` ga, javob shakli `components.schemas`
   ga yoziladi;
3. `docker compose up -d --build` — embed qayta yig'iladi.

Spec buzuq JSON bo'lsa build yiqilmaydi, lekin `/docs` bo'sh chiqadi.
Tekshirish:

```sh
python3 -c "import json;json.load(open('openapi.json'));print('spec joyida')"
```
