// config.go
package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	BotToken        string `json:"bot_token"`
	UpdateChannelID string `json:"update_channel_id"`
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
