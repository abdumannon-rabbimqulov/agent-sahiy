# O'zimizning GET endpointlar

Agent uchta tashqi API'dan ma'lumot oladi va ularning xom javobidan faqat
kerakli maydonlarni saralaydi. O'sha saralangan ko'rinish endi HTTP orqali
ham ochiq — agent siklini kutish yoki CLI ishga tushirish shart emas.

Har manbaning mantig'i ham, HTTP qatlami ham ALOHIDA faylda:
`internal/sources/{dashboard,adminka,support}.go` va
`internal/web/{dashboard,adminka,support}_api.go`.
Barchasi **faqat o'qiydi**: "o'qildi" qo'yilmaydi, xabar yuborilmaydi.

## Endpointlar

Hammasi dashboard bilan bir portda (`WEB_ADDR`, standart `:8080`) va o'sha
Basic Auth ostida (`ADMIN_USER` / `ADMIN_PASS`).

| Yo'l | Manba | Fayl | Parametrlar |
|---|---|---|---|
| `GET /api/dashboard` | `GET /api/v2/admin/delivery/orders/filter` | `internal/web/dashboard_api.go` | `track` yoki `client_id` |
| `GET /api/adminka` | `GET /api/admin/daigou-orders` | `internal/web/adminka_api.go` | `order_sn`, `express_num` (yoki `track`), `client_id` |
| `GET /api/support` | `GET /api/v1/support.chat.message/conversation` | `internal/web/support_api.go` | `conversation_id` yoki `client_id`, `limit` |

`track` va `express_num` — bir xil qiymat, ikki manbada nomi boshqacha.
`track` ga `DG...` yozilsa, daigou tomonida u avtomatik `order_sn` deb
qabul qilinadi.

## Xulq

- Hech qanday parametr berilmasa → **400**.
- Bitta manba yiqilsa → **200**, xato o'sha blokning `error` maydonida.
  Qolgan ma'lumot yo'qolmaydi (agentning xulqi ham shunday).
- Manba sozlanmagan bo'lsa (`SERVICE_PHONE`, `ADMINKA_TOKEN_BEARER` yo'q) →
  **200** va `"disabled": true`.

## Javob

Har blokda ikki qavat:

- `items` — saralangan tipli ro'yxat (daigou'da `internal/daigou` dagi
  `rows` jadvalidagi maydonlargina; xom obyekt bir buyurtma uchun ~10 KB).
- `summary` — **modelga aynan shu matn ketadi**. Ya'ni endpoint javobidagi
  `summary` bilan AI ko'radigan matn bir xil manbadan keladi.

```
GET /api/dashboard?track=YT7635822034113

{ "query": "track_number=YT7635822034113", "count": 1, "items": [...], "summary": "..." }
```

Zanjir dvigateli ham AYNAN shu funksiyalarni chaqiradi: prompt
`{"dashboard": true}` qaytarsa, kod `/api/dashboard` beradigan `summary`
matnini keyingi promptga qo'shadi (`docs/prompt-zanjiri.md`).

## Tekshirish

```sh
curl -u "$ADMIN_USER:$ADMIN_PASS" 'localhost:8080/api/dashboard?track=YT7635822034113'
curl -u "$ADMIN_USER:$ADMIN_PASS" 'localhost:8080/api/adminka?client_id=8162004'
curl -u "$ADMIN_USER:$ADMIN_PASS" 'localhost:8080/api/support?conversation_id=56825'
```

`summary` CLI chiqishi bilan solishtiriladi:

```sh
go run ./sahiy/dashboard -track YT7635822034113   # /api/dashboard bilan bir yo'l
go run ./sahiy/chat -conv 56825
```
