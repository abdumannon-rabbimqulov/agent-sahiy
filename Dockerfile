# --- 1-bosqich: build ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
# openapi.json binar ichiga embed qilinadi (docs_api.go).
COPY openapi.json ./
COPY support ./support
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sahiy .

# --- 2-bosqich: runtime ---
FROM alpine:3.20
# Tashqi API'lar HTTPS — ildiz sertifikatlarsiz TLS ishlamaydi.
# tesseract — rasmdagi buyurtma/trek raqamini MODELSIZ o'qish uchun
# (support/ocr.go). Ko'ruvchi modelga faqat OCR hech narsa topmaganda
# boriladi. `-data-eng` bo'lmasa tesseract ishga tushmaydi.
RUN apk add --no-cache ca-certificates tzdata tesseract-ocr tesseract-ocr-data-eng
# Kod .env ni va token cache fayllarini ISH KATALOGIDAN o'qiydi/yozadi
# (loadEnv(".env"), support.TokenFile, support.ServiceTokenFile).
WORKDIR /app
COPY --from=build /out/sahiy /usr/local/bin/sahiy
EXPOSE 8080
CMD ["sahiy"]
