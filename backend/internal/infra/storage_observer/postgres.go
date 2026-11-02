package storageobserver

import (
	"context"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"gorm.io/gorm"
	"time"
)

type RelationSource struct{ ID, Schema, Name string }

// PostgresCollector 复用当前部署的连接；不接收 API 输入的连接串或 SQL。
type PostgresCollector struct {
	DB        *gorm.DB
	Identity  storagegov.Identity
	ClusterID string
	Relations []RelationSource
}

func (c PostgresCollector) CollectDatabase(ctx context.Context) storagegov.DatabaseUsage {
	out := storagegov.DatabaseUsage{State: "unavailable", InstanceID: c.Identity.InstanceID, Environment: c.Identity.Environment, ClusterID: c.ClusterID, WALState: "unavailable", Relations: []storagegov.RelationUsage{}}
	if c.DB == nil || c.DB.Dialector.Name() != "postgres" || len(c.Relations) > 32 {
		out.Problem = "postgres_metrics_unavailable"
		return out
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	r := repo.NewPostgresMetricsRepository(c.DB)
	if size, err := r.DatabaseSize(run); err == nil && size >= 0 && size <= 1<<53-1 {
		out.DatabaseBytes = pointer(size)
		out.State = "observed"
	} else {
		out.Problem = "database_size_unreadable"
	}
	for _, source := range c.Relations {
		u := storagegov.RelationUsage{ID: source.ID, State: "unavailable"}
		m, err := r.Relation(run, source.Schema, source.Name)
		if err == nil && m.TotalBytes <= 1<<53-1 {
			u.State = "observed"
			u.HeapBytes = pointer(m.HeapBytes)
			u.IndexesBytes = pointer(m.IndexesBytes)
			u.ToastBytes = pointer(m.ToastBytes)
			u.TotalBytes = pointer(m.TotalBytes)
		} else {
			u.Problem = "relation_metrics_unreadable"
			out.State = "partial"
		}
		out.Relations = append(out.Relations, u)
	}
	if size, err := r.WALSize(run); err == nil && size >= 0 && size <= 1<<53-1 {
		out.WALState = "observed"
		out.WALBytes = pointer(size)
	}
	if out.DatabaseBytes == nil {
		out.State = "unavailable"
	} else if out.WALBytes == nil {
		out.State = "partial"
	}
	return out
}
