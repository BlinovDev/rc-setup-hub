package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	url := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(url) == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return Config{HTTPAddr: addr, DatabaseURL: url}, nil
}
