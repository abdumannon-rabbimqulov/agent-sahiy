# O'zimizning GET endpointlar

Agent uchta tashqi API'dan ma'lumot oladi va ularning xom javobidan faqat
kerakli maydonlarni saralaydi. O'sha saralangan ko'rinish endi HTTP orqali
ham ochiq — agent siklini kutish yoki CLI ishga tushirish shart emas.

Kod: `internal/sources/sources.go` (mantiq), `internal/web/sources.go` (HTTP).
Barchasi **faqat o'qiydi**: "o'qildi" qo'yilmaydi, xabar yuborilmaydi.

## Endpointlar

Hammasi dashboard bilan bir portda (`WEB_ADDR`, standart `:8080`) va o'sha
Basic Auth ostida (`ADMIN_USER` / `ADMIN_PASS`).

| Yo'l | Manba | Parametrlar |
|---|---|---|
| `GET /api/source/delivery` | `GET /api/v2/admin/delivery/orders/filter` | `track` yoki `client_id` |
| `GET /api/source/daigou` | `GET /api/admin/daigou-orders` | `order_sn`, `express_num` (yoki `track`), `client_id` |
| `GET /api/source/support` | `GET /api/v1/support.chat.message/conversation` | `conversation_id` yoki `client_id`, `limit` |
| `GET /api/source/all` | uchalasi birga | yuqoridagilarning istalgan aralashmasi |

`track` va `express_num` — bir xil qiymat, ikki manbada nomi boshqacha.
`track` ga `DG...` yozilsa, daigou tomonida u avtomatik `order_sn` deb
qabul qilinadi.

## Xulq

- Hech qanday parametr berilmasa → **400**.
- Bitta manba yiqilsa → **200**, xato o'sha blokning `error` maydonida.
  Qolgan ma'lumot yo'qolmaydi (agentning xulqi ham shunday).
- Manba sozlanmagan bo'lsa (`SERVICE_PHONE`, `ADMINKA_TOKEN_BEARER` yo'q) →
  **200** va `"disabled": true`.
- `/api/source/all` da `client_id` berilmasa, u avval `conversation_id`
  bo'yicha suhbatdan aniqlanadi — keyin buyurtmalar so'raladi.

## Javob

Har blokda ikki qavat:

- `items` — saralangan tipli ro'yxat (daigou'da `internal/daigou` dagi
  `rows` jadvalidagi maydonlargina; xom obyekt bir buyurtma uchun ~10 KB).
- `summary` — **modelga aynan shu matn ketadi**. Ya'ni endpoint javobidagi
  `summary` bilan AI ko'radigan matn bir xil manbadan keladi.

```
GET /api/source/all?client_id=8162004&track=YT7635822034113

{
  "support":  { "conversation_id": 56825, "count": 12, "messages": [...], "transcript": "..." },
  "delivery": { "query": "track_number=YT7635822034113", "count": 1, "items": [...], "summary": "..." },
  "daigou":   { "query": "express_num=YT7635822034113", "count": 1, "items": [...], "summary": "..." }
}
```

## Tekshirish

```sh
curl -u "$ADMIN_USER:$ADMIN_PASS" 'localhost:8080/api/source/delivery?track=YT7635822034113'
curl -u "$ADMIN_USER:$ADMIN_PASS" 'localhost:8080/api/source/all?client_id=8162004'
```

`summary` CLI chiqishi bilan solishtiriladi:

```sh
go run ./sahiy/dashboard -track YT7635822034113   # /api/source/delivery bilan bir yo'l
go run ./sahiy/chat -conv 56825
```
