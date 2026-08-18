# Modellar va cache

Bu hujjat hozirgi holatni **yozib qo'yadi** — kodda hech narsa o'zgartirilgani
yo'q. Har bir bandda manba fayl:qator ko'rsatilgan, ya'ni shubha bo'lsa
kodning o'zidan tekshirish mumkin.

## 1. Qaysi model qayerda ishlaydi

Butun tizimda **bitta** `ai.Client` bor (`main.go`), vazifaga qarab model
almashmaydi. Farq faqat qatlamda: asosiy — lokal, zaxira — bulut.

| Rol | Paket | Model | Manba |
|---|---|---|---|
| Asosiy (lokal, bepul) | `ollama` | `llama3.1:8b` | `internal/ollama/ollama.go:20-21` |
| Zaxira (bulut, pullik) | `groq` | `openai/gpt-oss-120b` | `internal/groq/groq.go:23-29` |

**Groq modelini o'zgartirishdan oldin o'qing:** `llama-3.3-70b`,
`llama-3.1-8b` va `groq-compound` `response_format: json_schema` ni
qabul qilmaydi, butun qaror yo'li esa aynan shunga tayanadi
(`internal/groq/groq.go:24-29`). Shuning uchun `gpt-oss-120b` tanlangan.

### Zaxiraga qachon o'tiladi

`internal/ai/fallback.go:45-58`: zaxira **faqat asosiy model bo'sh javob
qaytarganda** ishga tushadi. Javob bo'sh bo'lmasa, lekin xato ham bo'lsa
(masalan `ollama.ErrContextFull`, `ollama.go:231-233`) — bu qisman muvaffaqiyat
sanaladi, javob ishlatiladi, xato esa Meter ogohlantirishi sifatida qoladi.

`GROQ_API_KEY` bo'sh bo'lsa zaxira umuman tuzilmaydi va faqat Ollama qoladi
(`fallback.go:24-29`). Model nomi `"ollama llama3.1:8b → groq openai/gpt-oss-120b"`
ko'rinishida yoziladi (`fallback.go:34-36`) va narx hisobida `modelName()`
undan asosiy modelni ajratib oladi (`main.go`).

### Vazifalar orasidagi farq — promptda, modelda emas

Uchala chaqiruv ham bir xil backendga boradi; farqi prompt va JSON sxemada:

| Chaqiruv | Prompt kaliti | Sozlama | Manba |
|---|---|---|---|
| `Decide` (kategoriya + raqamlar) | `base` | `MaxTokens: 300`, `TempZero`, `JSON`, `DecisionSchema` | `internal/ai/ai.go:172-183` |
| `OrderAnswer` (buyurtma javobi) | `base` + bloklar | `MaxTokens: 600`, `TempZero`, `JSON`, `OrderReplySchema` | `internal/ai/ai.go:239-256` |
| `Answer` (erkin matn) | `cat:<slug>` | provayder sozlamalari | `internal/ai/ai.go:228-235` |

Kategoriya kalitlari `main.go` da: `yetkazib-berish`, `order`,
`xato-mahsulot-kelganda`. Promptlarning o'zi Postgres'da, dashboarddan
tahrirlanadi (`/prompts`).

Diqqat: Groq'da `reasoning_effort` qo'yilgan bo'lsa (gpt-oss uchun avtomatik
`low`) `max_tokens` ichkarida 1024 gacha ko'tariladi — reasoning tokenlari
ham shu byudjetdan yeyiladi (`internal/groq/groq.go:184-186`).

### Muhit o'zgaruvchilari

- Ollama: `OLLAMA_URL`, `OLLAMA_MODEL`, `OLLAMA_KEEP_ALIVE`, `OLLAMA_NUM_CTX`,
  `OLLAMA_MAX_TOKENS`, `OLLAMA_TEMPERATURE`, `OLLAMA_TIMEOUT_SEC`
  (`internal/config/config.go`). Standart: keep_alive `2m`, num_ctx `4096`,
  max_tokens `600`, temp `0.2`, timeout `180s`.
- Groq: `GROQ_API_KEY`, `GROQ_MODEL`, `GROQ_BASE_URL`, `GROQ_MAX_TOKENS`,
  `GROQ_TEMPERATURE`, `GROQ_TIMEOUT_SEC`, `GROQ_REASONING_EFFORT`,
  `GROQ_PRICE_IN`, `GROQ_PRICE_OUT`. Standart: max_tokens 600, temp 0.2,
  timeout 60s.
- Narx va byudjet: `AI_PRICE_IN`, `AI_PRICE_CACHED_IN`, `AI_PRICE_OUT`,
  `AI_BUDGET_USD`.

## 2. Cache — uch xil, uchalasi ham joyida qoladi

### 2.1. API tokeni (disk)

`internal/cache/cache.go` — bu AI keshi emas, **tashqi API tokeni**.
`TokenCache{Token, ExpiresAt}` JSON sifatida `0600` huquqi bilan saqlanadi
(`cache.go:33-43`). Fayl yo'q, buzilgan yoki muddati o'tgan bo'lsa `Load`
nil qaytaradi (`cache.go:26-29`).

- TTL: JWT ichidagi `exp` dan 60 soniya ayirib olinadi; `exp` bo'lmasa
  `auth.FallbackTTL` (`internal/auth/auth.go:97-118`).
- 401 kelsa fayl o'chiriladi va qaytadan login qilinadi
  (`auth.go:121-151`).
- Fayllar: `token.json` (support API), `service-token.json` (delivery API).
- Service client'da alohida xotira muddati bor: `ExpiresIn` (standart 24 soat)
  minus 5 daqiqa xavfsizlik zaxirasi (`internal/service/service.go:114-120`).

### 2.2. Promptlar (xotira)

`internal/prompts/cache.go` — `atomic.Pointer[map[string]string]`, butun xarita
birdan almashtiriladi (`cache.go:14-27`). Qulf yo'q, o'qish arzon.

- **TTL yo'q**, eviction yo'q: bu bazadagi yoqilgan promptlarning to'liq nusxasi.
- Yangilanish: dashboarddan yozilganda darhol, ustiga har **60 soniyada**
  fingerprint (soni + oxirgi tahrir vaqti) tekshiriladi va faqat o'zgargan
  bo'lsa qayta o'qiladi (`service.go:18`, `service.go:153-180`).
- Baza yiqilsa yoki nol qator qaytsa — **eski kesh saqlanib qoladi**
  (`service.go:128-135`). Bu ataylab: prompt yo'qolgani agentni to'xtatmasin.
- Bitta prompt uchun chegara 1 MB (`service.go:21`).

### 2.3. Provayder tomonidagi prompt-kesh (KV-cache)

`buildSystem` system-promptni **qat'iy tartibda** yig'adi, chunki provayderning
prefiks keshi shunga tayanadi (`internal/ai/ai.go:158-167`). Tartib
o'zgarsa kesh bekor bo'ladi va har so'rov to'liq narxda ketadi.

Nozik joy: `render()` har promptga bugungi sanani qo'shadi
(`internal/ai/ai.go:295-297`, `ai.go:320-325`). Sana kuniga bir marta
o'zgaradi, ya'ni prefiks bir kun ichida barqaror — lekin har yarim tunda kesh
bir marta bekor bo'ladi. Bu ma'lum va qabul qilingan xarajat.

### 2.4. `cached_tokens` — hozircha doim 0

Butun yo'l qurilgan: `models.Interaction.CachedTokens`
(`internal/models/models.go:70`), `AI_PRICE_CACHED_IN`, `pricing.Price.CachedIn`,
dashboarddagi `SUM(cached_tokens)` (`internal/store/store.go:173`, `:204`, `:241`).

Lekin **hech kim uni to'ldirmaydi**:

- Groq javobidan faqat `prompt_tokens` va `completion_tokens` o'qiladi
  (`internal/groq/groq.go:154-157`, `:241-247`).
- Ollama faqat `prompt_eval_count` / `eval_count` beradi
  (`internal/ollama/ollama.go:222-228`).

Anthropic uslubidagi aniq prompt caching (`cache_control`, `ephemeral`)
kodda umuman ishlatilmagan. Ya'ni dashboarddagi "cached" ustuni bugun doim
nol — bu buzilish emas, shunchaki provayder bu raqamni qaytarmaydi.
