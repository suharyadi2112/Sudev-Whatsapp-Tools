package config

import (
	"os"
)

type Config struct {
	Port               string
	DBConnectionString string
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "2121"),
		DBConnectionString: getEnv("DATABASE_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
