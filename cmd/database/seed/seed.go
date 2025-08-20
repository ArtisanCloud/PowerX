package seed

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func seedCtx() context.Context { return context.Background() }

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func SeedCoreX(ctx context.Context, db *gorm.DB, cfg *config.Config) error {
	db, err := database.Connect(cfg.Database)
	if err != nil {
		//log.Fatal(err)
		return err
	}

	if err := SeedRoot(db); err != nil {
		//log.Fatal(err)
		return err
	}
	log.Println("seed ok")

	return nil
}
