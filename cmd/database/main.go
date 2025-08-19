// cmd/database/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"log"
	"os"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s [migrate|seed]", os.Args[0])
	}
	cmd := os.Args[1]
	configPath := flag.String("config", "etc/config.yaml", "配置文件路径")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	ctx := context.Background()
	// 连接数据库
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	switch cmd {
	case "migrate":
		if err := MigrateDatabase(ctx, db); err != nil {
			log.Fatal("migrate failed:", err)
		}
		fmt.Println("migrate ok")

	case "seed":
		if err := SeedCoreX(ctx, db, cfg); err != nil { // 这里调用你写的 seed 方法
			log.Fatal("seed failed:", err)
		}
		fmt.Println("seed ok")

	default:
		log.Fatalf("Unknown command: %s", cmd)
	}
}
