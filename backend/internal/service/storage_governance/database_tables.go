package storagegov

import (
	"context"
	"errors"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
)

type DatabaseTableInventory struct {
	ObservedAt       time.Time                     `json:"observed_at"`
	ExpiresAt        time.Time                     `json:"expires_at"`
	Identity         Identity                      `json:"identity"`
	Scope            string                        `json:"scope"`
	Items            []repo.SystemTableObservation `json:"items"`
	InvocationPolicy *DatabaseRetentionPolicy      `json:"invocation_policy"`
	PolicyProblem    string                        `json:"policy_problem,omitempty"`
	Monitoring       TableMonitoringStatus         `json:"monitoring"`
}

// SystemTableCollector 扩展只读数据库观测，不创建新连接或迁移表。
type SystemTableCollector interface {
	CollectSystemTables(context.Context, string) ([]repo.SystemTableObservation, error)
}

var ErrSystemTableNotFound = errors.New("system_table_not_registered")

// DatabaseTables 仅供平台 Root；缓存和限时查询避免刷新页面反复扫描大表。
func (s *Service) DatabaseTables(ctx context.Context, id string) (DatabaseTableInventory, error) {
	out := DatabaseTableInventory{Identity: s.opts.Identity, Scope: "current_database_registered_tables_including_soft_deleted", Items: []repo.SystemTableObservation{}}
	if rootActor(ctx) == 0 {
		return out, ErrRootRequired
	}
	if id != "" {
		found := false
		for _, d := range m.SystemTableCatalog() {
			if d.Name == id {
				found = true
				break
			}
		}
		if !found {
			return out, ErrSystemTableNotFound
		}
	}
	s.tablesMu.Lock()
	defer s.tablesMu.Unlock()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	now := s.opts.Now().UTC()
	cached := id == "" && s.tablesCached != nil && now.Before(s.tablesCached.ExpiresAt)
	if cached {
		out = *s.tablesCached
		out.InvocationPolicy, out.PolicyProblem = nil, ""
	} else {
		out.ObservedAt, out.ExpiresAt = now, now.Add(2*time.Minute)
		if collector, ok := s.opts.DatabaseCollector.(SystemTableCollector); ok {
			items, err := collector.CollectSystemTables(ctx, id)
			if err != nil {
				return out, err
			}
			out.Items = items
		} else {
			for _, d := range m.SystemTableCatalog() {
				if id == "" || id == d.Name {
					out.Items = append(out.Items, repo.SystemTableObservation{SystemTableDefinition: d, State: "unavailable", TimeState: "unavailable", Problem: "postgres_metrics_unavailable"})
				}
			}
		}
	}
	if s.opts.Control != nil {
		p, c, e := s.loadRetentionControl(ctx)
		if e == nil && c.Authority == "database" {
			p.Enabled = p.Enabled && c.ExecutionReady
			out.InvocationPolicy = &p
		} else {
			out.PolicyProblem = "database_policy_unreadable"
		}
	} else {
		out.PolicyProblem = "database_retention_runtime_not_configured"
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	out.Monitoring = s.tableMonitoring(ctx)
	if id == "" {
		s.tablesCached = &out
	}
	return out, nil
}
