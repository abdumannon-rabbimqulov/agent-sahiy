package main

import (
	"fmt"
	"os"
	"strings"

	"sahiy/support"
)

// loadEnv .env faylini o'qib muhit o'zgaruvchilariga qo'yadi.
// Allaqachon o'rnatilgan qiymat ustidan yozilmaydi.
func loadEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		os.Setenv(k, strings.Trim(strings.TrimSpace(v), `"'`))
	}
}

func main() {
	loadEnv(".env")

	token, err := support.Token(support.CredentialsFromEnv(), support.TokenFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "support login:", err)
		os.Exit(1)
	}
	fmt.Println("support token olindi, uzunligi:", len(token))
}
