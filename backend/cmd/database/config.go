package main

import (
	"fmt"

	"github.com/ArtisanCloud/PowerX/config"
)

func loadDatabaseCommandConfig(command, path string) (*config.Config, error) {
	if command == "migrate" || command == "status" {
		return config.LoadForMigration(path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		return nil, fmt.Errorf("读取 server.secret_key 失败: %w", err)
	}
	return cfg, nil
}
