// config.go
package main

import (
	"log"
	"os"
)

type Config struct {
	BotToken        string
	UpdateChannelID string
	APIBaseURL      string
	ReaderBaseURL   string
	DatabaseURL     string
}

func LoadConfig() *Config {
	cfg := &Config{
		BotToken:        os.Getenv("BOT_TOKEN"),
		UpdateChannelID: os.Getenv("UPDATE_CHANNEL_ID"),
		APIBaseURL:      os.Getenv("API_BASE_URL"),
		ReaderBaseURL:   os.Getenv("READER_BASE_URL"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
	}

	if cfg.BotToken == "" || cfg.UpdateChannelID == "" || cfg.APIBaseURL == "" || cfg.ReaderBaseURL == "" || cfg.DatabaseURL == "" {
		log.Fatalf("FATAL: One or more required environment variables are not set. Please check BOT_TOKEN, UPDATE_CHANNEL_ID, API_BASE_URL, READER_BASE_URL, DATABASE_URL.")
	}

	return cfg
}
