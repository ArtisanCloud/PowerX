package bootstrap

import (
	"fmt"
	"os"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func datasetExternalSources(entries []config.StoragePluginPostgresSource) (map[string]storagegov.DatasetExternalSource, error) {
	if err := config.ValidateStoragePluginPostgresSources(entries); err != nil {
		return nil, err
	}
	sources := make(map[string]storagegov.DatasetExternalSource, len(entries))
	for _, entry := range entries {
		// Require an explicit deployment-provided connection. Do not let
		// empty configuration fall back to ambient PostgreSQL defaults.
		raw, ok := os.LookupEnv(entry.ObserverDSNEnv)
		if !ok || raw == "" {
			return nil, fmt.Errorf("storage_governance observer DSN missing for plugin %s", entry.PluginID)
		}
		cfg, err := pgx.ParseConfig(raw)
		if err != nil || cfg.Database == "" || cfg.User == "" || cfg.Host == "" {
			return nil, fmt.Errorf("storage_governance observer DSN invalid for plugin %s", entry.PluginID)
		}
		for _, fallback := range cfg.Fallbacks {
			if fallback.Host != cfg.Host || fallback.Port != cfg.Port {
				return nil, fmt.Errorf("storage_governance observer has multiple endpoints for plugin %s", entry.PluginID)
			}
		}
		cfg.ConnectTimeout = 2 * time.Second
		cfg.RuntimeParams["default_transaction_read_only"] = "on"
		sources[entry.PluginID] = storagegov.DatasetExternalSource{
			Binding: uuid.MustParse(entry.BindingUUID), ClusterID: entry.ClusterID, Schema: entry.Schema, Role: entry.WorkerRole,
			Open: func() (*gorm.DB, func(), error) {
				pool := stdlib.OpenDB(*cfg.Copy())
				pool.SetMaxOpenConns(1)
				pool.SetMaxIdleConns(0)
				closePool := func() { _ = pool.Close() }
				db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{
					DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
				})
				if err != nil {
					closePool()
					return nil, nil, err
				}
				return db, closePool, nil
			},
		}
	}
	return sources, nil
}
