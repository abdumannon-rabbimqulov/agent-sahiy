# Promt 8 — Yig'ish (mijozga javob yozilmaydi)

Yig'ish rejimining yagona promti. 1–7 promtlarga tegilmaydi: `collect_mode`
o'chirilsa eski agent avvalgidek ishlaydi.

Baza yozuvini kod ishga tushganda o'zi yaratadi (`ensureCollectPromt`,
`support/collect_model.go`), nomi bo'yicha topiladi. Keyin matnni paneldagi
"Promtlar" sahifasidan tahrirlash mumkin, kod uni qayta yozmaydi.

Model **kod hisoblagan turni o'zgartirmaydi**: uning xulosasi alohida
`ai_type` va `ai_note` maydonlariga yoziladi. Noto'g'ri JSON yoki ro'yxatda
yo'q tur kelsa xulosa bo'sh qoladi.

Boshlang'ich matn: `collectPromtText` (`support/collect_model.go`).
