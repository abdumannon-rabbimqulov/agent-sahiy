# Sahiy admin paneli (frontend)

Backenddan butunlay alohida React (Vite) loyihasi.

## Ishga tushirish (dev)

```sh
npm install
npm run dev          # http://localhost:5173
```

Backend `http://localhost:8080` da ishlab turishi kerak — `/api` so'rovlari
vite proksisi orqali o'tadi (`vite.config.js`). Boshqa manzil kerak bo'lsa
`.env` ga `VITE_API_URL=http://boshqa-host:8080` yozing.

Backendda `CORS_ORIGIN` shu manzilni ruxsat etishi kerak (default:
`http://localhost:5173`).

## Build

```sh
npm run build        # dist/
```

## Kirish

Login/parol backend seed qilgan admin: `991134543` / `991134543`.

## Sahifalar

| Yo'l | Nima qiladi |
|---|---|
| `#/` | Statistika: murojaatlar, AI hal qilgani, tokenlar, xarajat, kunlik grafik |
| `#/queue` | Tasdiqlash navbati — chat/help matnini tahrirlab tasdiqlash yoki rad etish |
| `#/interactions/:id` | Bitta murojaat: zanjir bosqichlari, har bosqich JSON'i, tokenlar |
| `#/promts` | Promt CRUD (zanjir promt id bo'yicha ishlaydi, boshi — #1) |
| `#/settings` | Avtomatik javob va fon sikli tugmalari |
