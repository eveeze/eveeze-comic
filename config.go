// config.go
package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	BotToken        string `json:"bot_token"`
	UpdateChannelID string `json:"update_channel_id"`
	APIBaseURL      string `json:"api_base_url"`
	ReaderBaseURL   string `json:"reader_base_url"`
}

func LoadConfig() (*Config, error) {
	file, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = json.Unmarshal(file, &cfg)
	return &cfg, err
}
