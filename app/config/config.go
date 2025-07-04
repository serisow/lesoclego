package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Environment                string
	APIEndpoint                string
	APIHost                    string
	ServiceBaseURL             string
	CheckInterval              time.Duration
	Domains                    []string
	CertCacheDir               string
	HTTPPort                   string
	HTTPSPort                  string
	DrupalUsername             string
	DrupalPassword             string
	GoogleCustomSearchAPIKey   string
	GoogleCustomSearchEngineID string
	NewsAPIKey                 string
	CronURL                    string
	CronInterval               time.Duration
}

var isTest bool

func init() {
	isTest = os.Getenv("GO_ENVIRONMENT") == "test"
	if !isTest {
		err := godotenv.Load()
		if err != nil {
			log.Println("Warning: Error loading .env file:", err)
		}
	}
}

func Load() Config {

	return Config{
		Environment:                getEnv("ENVIRONMENT", "development"),
		APIEndpoint:                getEnv("API_ENDPOINT", "http://lesocle-dev.sa/api"),
		APIHost:                    getEnv("API_HOST", "lesocle-dev.sa"),
		ServiceBaseURL:             getEnv("SERVICE_BASE_URL", "http://localhost:8086"), // Default to localhost
		CheckInterval:              time.Duration(getEnvAsInt("CHECK_INTERVAL", 1200)) * time.Second,
		Domains:                    []string{getEnv("DOMAIN", "example.com")},
		CertCacheDir:               getEnv("CERT_CACHE_DIR", "/etc/letsencrypt/live/example.com"),
		HTTPPort:                   getEnv("HTTP_PORT", "8086"),
		HTTPSPort:                  getEnv("HTTPS_PORT", "443"),
		DrupalUsername:             getEnv("DRUPAL_USERNAME", ""),
		DrupalPassword:             getEnv("DRUPAL_PASSWORD", ""),
		GoogleCustomSearchAPIKey:   getEnv("GoogleCustomSearchAPIKey", ""),
		GoogleCustomSearchEngineID: getEnv("GoogleCustomSearchEngineID", ""),
		NewsAPIKey:                 getEnv("NEWS_API_KEY", ""),
		CronURL:                    getEnv("DRUPAL_CRON_URL", ""),
		CronInterval:               time.Duration(getEnvAsInt("CRON_INTERVAL", 300)) * time.Second, // Default 5 minutes
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	strValue := getEnv(key, "")
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return fallback
}
