package config

import (
	"log"
	"os"
)

type Config struct {
	BaseURL string
	Token   string
}

func LoadConfig() *Config {
	baseURL := os.Getenv("PIPEDRIVE_BASE_URL")
	token := os.Getenv("PIPEDRIVE_API_TOKEN")

	if baseURL == "" || token == "" {
		log.Fatal("missing required environment variables for Pipedrive")
	}

	return &Config{
		BaseURL: baseURL,
		Token:   token,
	}
}
