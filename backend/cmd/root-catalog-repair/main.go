package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "etc/config.yaml", "CORE_CONFIG_PATH")
	apply := flag.Bool("apply", false, "EXPLICIT_APPLY_DEFAULT_FALSE")
	fingerprint := flag.String("expect-fingerprint", "", "REVIEWED_PREVIEW_FINGERPRINT")
	operator := flag.String("operator", "", "MAINTENANCE_OPERATOR")
	flag.Parse()
	if *apply && (*operator == "" || len(*fingerprint) != 64) {
		return errors.New("IAM_ROOT_CATALOG_REPAIR_CONFIRMATION_REQUIRED")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return errors.New("IAM_ROOT_CATALOG_REPAIR_CONFIG_INVALID")
	}
	config.GlobalConfig = cfg
	db, err := database.Connect(cfg.Database)
	if err != nil {
		return errors.New("IAM_ROOT_CATALOG_REPAIR_CONNECTION_FAILED")
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := baseline.RepairRootCatalog(ctx, db, baseline.RootCatalogRepairInput{Apply: *apply, ExpectedFingerprint: *fingerprint, Operator: *operator})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
