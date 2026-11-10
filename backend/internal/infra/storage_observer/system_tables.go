package storageobserver

import (
	"context"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
)

func (c PostgresCollector) CollectSystemTables(ctx context.Context, id string) ([]repo.SystemTableObservation, error) {
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	queryTimeout := 100 * time.Millisecond
	if id != "" {
		queryTimeout = 2 * time.Second
	}
	return repo.NewSystemTablesRepository(c.DB).Observe(run, coremodel.PowerXSchema, id, queryTimeout)
}

// CollectSystemTableMetrics 用于既有后台采样，只取 pg_catalog 容量，绝不查询最早／最新行。
func (c PostgresCollector) CollectSystemTableMetrics(ctx context.Context) ([]repo.SystemTableObservation, error) {
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return repo.NewSystemTablesRepository(c.DB).Observe(run, coremodel.PowerXSchema, "", 0)
}
