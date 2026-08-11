# --- Build bosqichi ---
FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent .

# --- Ishga tushirish bosqichi ---
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/agent /app/agent

# Ma'lumot (sessiya/token) uchun katalog
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/agent"]
