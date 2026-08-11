package main

import (
	"fmt"
	"os"

	"sahiy-agent/internal/auth"
	"sahiy-agent/internal/client"
	"sahiy-agent/internal/config"
)

const (
	envPath   = ".env"
	cachePath = "token.json"
)

func main() {
	cfg, err := config.Load(envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config xatosi:", err)
		os.Exit(1)
	}

	token, err := auth.GetToken(cfg, cachePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "token olish xatosi:", err)
		os.Exit(1)
	}

	fmt.Println("✓ Token olindi va cache'landi (token.json)")
	fmt.Println("Token:", mask(token))

	// Kelajakdagi endpointlar shu client orqali chaqiriladi:
	c := client.New(cfg.BaseURL, token)
	_ = c
	// Masalan:
	// body, status, err := c.Get("/api/v1/admins/me")
	// fmt.Println(status, string(body), err)
}

// mask tokenni to'liq chop etmaslik uchun qisqartiradi.
func mask(t string) string {
	if len(t) <= 12 {
		return t
	}
	return t[:6] + "..." + t[len(t)-4:]
}
