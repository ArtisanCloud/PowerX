// cmd/database/main.go
package main

import (
	"context"
	"flag"
	"github.com/ArtisanCloud/PowerX/cmd/database/seed"
	"github.com/ArtisanCloud/PowerX/config"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("Usage: %s [migrate|seed|refresh]", os.Args[0])
	}
	cmd := os.Args[1]
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	fs := flag.NewFlagSet("database", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "配置文件路径")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fatalf("解析参数失败: %v", err)
	}

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("加载配置失败: %v", err)
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		fatalf("读取 server.secret_key 失败: %v", err)
	}

	ctx := context.Background()
	// 连接数据库
	db, err := database.Connect(cfg.Database)
	if err != nil {
		fatalf("连接数据库失败: %v", err)
	}

	switch cmd {
	case "migrate":
		if err := MigrateDatabase(ctx, db, cfg); err != nil {
			fatalf("migrate failed: %v", err)
		}
		logger.InfoF(context.Background(), "migrate ok")

	case "seed":
		if err := seed.SeedCoreX(ctx, db, cfg); err != nil {
			fatalf("seed failed: %v", err)
		}
		logger.InfoF(context.Background(), "seed ok")

	case "refresh":
		// 先 drop database（或 drop all tables）
		if err := ResetDatabase(ctx, db); err != nil {
			fatalf("reset failed: %v", err)
		}
		logger.InfoF(context.Background(), "reset ok")

		// 再 migrate
		if err := MigrateDatabase(ctx, db, cfg); err != nil {
			fatalf("migrate failed: %v", err)
		}
		logger.InfoF(context.Background(), "migrate ok")

		// 最后 seed
		if err := seed.SeedCoreX(ctx, db, cfg); err != nil {
			fatalf("seed failed: %v", err)
		}
		logger.InfoF(context.Background(), "seed ok")

	default:
		fatalf("Unknown command: %s", cmd)
	}
}

func fatalf(format string, args ...any) {
	logger.ErrorF(context.Background(), format, args...)
	os.Exit(1)
}
