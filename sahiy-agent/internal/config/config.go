package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config dasturga kerakli sozlamalar.
type Config struct {
	Login    string
	Password string
	BaseURL  string
}

// Load .env faylni o'qiydi va Config qaytaradi.
// .env bo'lmasa, tizim muhit o'zgaruvchilaridan foydalanadi.
func Load(envPath string) (*Config, error) {
	_ = loadDotEnv(envPath) // .env ixtiyoriy

	cfg := &Config{
		Login:    os.Getenv("LOGIN"),
		Password: os.Getenv("PASSWORD"),
		BaseURL:  os.Getenv("BASE_URL"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.market.sahiy.uz"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.Login == "" || cfg.Password == "" {
		return nil, fmt.Errorf("LOGIN va PASSWORD .env faylda yoki muhit o'zgaruvchilarida bo'lishi kerak")
	}
	return cfg, nil
}

// loadDotEnv oddiy KEY=VALUE parser (tashqi kutubxonasiz).
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// qo'shtirnoqlarni olib tashlash
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}
