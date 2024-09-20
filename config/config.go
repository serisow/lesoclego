package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment   string
	APIEndpoint   string
	CheckInterval time.Duration
	Domains       []string
	CertCacheDir  string
	HTTPPort      string
	HTTPSPort     string
}

func Load() Config {
	return Config{
		Environment:   getEnv("ENVIRONMENT", "development"),
		APIEndpoint:   getEnv("API_ENDPOINT", "http://lesocle-dev.sa:9090/api"),
		CheckInterval: time.Duration(getEnvAsInt("CHECK_INTERVAL", 120)) * time.Second,
		Domains:       []string{getEnv("DOMAIN", "example.com")},
		CertCacheDir:  getEnv("CERT_CACHE_DIR", "/etc/letsencrypt/live/example.com"),
		HTTPPort:      getEnv("HTTP_PORT", "8086"),
		HTTPSPort:     getEnv("HTTPS_PORT", "443"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}