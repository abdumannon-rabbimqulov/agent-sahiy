# --- 1-bosqich: build ---
FROM golang:1.26-alpine AS build
WORKDIR /src
# Tashqi bog'liqlik yo'q (go.mod'da faqat modul nomi) — shuning uchun
# `go mod download` qadami ham kerak emas.
COPY go.mod ./
COPY main.go docs_api.go ./
# openapi.json binar ichiga embed qilinadi (docs_api.go).
COPY openapi.json ./
COPY support ./support
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sahiy .

# --- 2-bosqich: runtime ---
FROM alpine:3.20
# Tashqi API'lar HTTPS — ildiz sertifikatlarsiz TLS ishlamaydi.
RUN apk add --no-cache ca-certificates tzdata
# Kod .env ni va token cache fayllarini ISH KATALOGIDAN o'qiydi/yozadi
# (loadEnv(".env"), support.TokenFile, support.ServiceTokenFile).
WORKDIR /app
COPY --from=build /out/sahiy /usr/local/bin/sahiy
EXPOSE 8080
CMD ["sahiy"]
