package storagegov

import (
	"context"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"time"
)

// SystemTableRetentionPolicy 授权一类固定规则的保留期及批次，默认停用。
type SystemTableRetentionPolicy struct {
	Enabled       bool      `json:"enabled"`
	RetentionDays int       `json:"retention_days"`
	BatchLimit    int       `json:"batch_limit"`
	Actor         uint64    `json:"actor"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SystemTableRetentionRun 保留请求、截止时间、策略快照及可核对的事务结果。
type SystemTableRetentionRun struct {
	PreviewID string                     `json:"preview_id,omitempty"`
	ExpiresAt *time.Time                 `json:"expires_at,omitempty"`
	Trigger   string                     `json:"trigger,omitempty"`
	ID        string                     `json:"id"`
	TableID   string                     `json:"table_id"`
	Revision  int64                      `json:"revision"`
	State     string                     `json:"state"`
	At        time.Time                  `json:"at"`
	Actor     uint64                     `json:"actor"`
	Cutoff    time.Time                  `json:"cutoff"`
	Rows      int64                      `json:"rows"`
	Problem   string                     `json:"problem,omitempty"`
	Policy    SystemTableRetentionPolicy `json:"policy_snapshot"`
}
type systemTableRetentionDocument struct {
	Revision      int64                                 `json:"revision"`
	OwnerInstance string                                `json:"owner_instance"`
	UpdatedAt     time.Time                             `json:"updated_at"`
	Actor         uint64                                `json:"actor"`
	Policies      map[string]SystemTableRetentionPolicy `json:"policies"`
	Runs          []SystemTableRetentionRun             `json:"runs"`
	Previews      map[string]SystemTableRetentionRun    `json:"previews"`
}

// SystemTableRetentionStatus 区分部署门禁、策略与真实运行记录。
type SystemTableRetentionStatus struct {
	Configuration      *SystemTableRetentionConfiguration `json:"configuration"`
	UnconfirmedRuns    int                                `json:"unconfirmed_runs"`
	AdapterProblem     string                             `json:"adapter_problem,omitempty"`
	ID                 string                             `json:"id"`
	Revision           int64                              `json:"revision"`
	Policy             SystemTableRetentionPolicy         `json:"policy"`
	StateConfigured    bool                               `json:"state_configured"`
	EnforcementAllowed bool                               `json:"enforcement_allowed"`
	WorkerEnabled      bool                               `json:"worker_enabled"`
	Ready              bool                               `json:"ready"`
	Problem            string                             `json:"problem,omitempty"`
	Runs               []SystemTableRetentionRun          `json:"runs"`
}

func validSystemTablePolicy(p SystemTableRetentionPolicy) bool {
	return p.RetentionDays >= 7 && p.RetentionDays <= 3650 && p.BatchLimit >= 1 && p.BatchLimit <= 500
}
func (s *Service) systemTableDocument(raw []byte) (systemTableRetentionDocument, error) {
	d := systemTableRetentionDocument{OwnerInstance: s.opts.Identity.InstanceID, Policies: map[string]SystemTableRetentionPolicy{}, Previews: map[string]SystemTableRetentionRun{}, Runs: []SystemTableRetentionRun{}}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		d.Policies[id] = SystemTableRetentionPolicy{RetentionDays: 30, BatchLimit: 200}
	}
	if len(raw) > 0 {
		if json.Unmarshal(raw, &d) != nil || d.OwnerInstance != s.opts.Identity.InstanceID || d.Revision < 0 || len(d.Policies) != 2 || d.Previews == nil || len(d.Previews) > 2 || len(d.Runs) > 200 {
			return d, ErrStateUnavailable
		}
		for id, p := range d.Policies {
			if !repo.SystemRetentionSupported(id) || !validSystemTablePolicy(p) {
				return d, ErrStateUnavailable
			}
		}
		for id, p := range d.Previews {
			if !repo.SystemRetentionSupported(id) || p.TableID != id || p.State != "preview" || p.Rows < 0 || !validSystemTablePolicy(p.Policy) {
				return d, ErrStateUnavailable
			}
		}
		for _, r := range d.Runs {
			if !repo.SystemRetentionSupported(r.TableID) || r.State != "completed" && r.State != "failed" && r.State != "unconfirmed" || r.Rows < 0 || !validSystemTablePolicy(r.Policy) {
				return d, ErrStateUnavailable
			}
		}
	}
	return d, nil
}

// SystemTableRetention 返回本表的独立配置版本和执行记录，旧文件不构成清理许可。
func (s *Service) SystemTableRetention(ctx context.Context, id string) (SystemTableRetentionStatus, error) {
	out := SystemTableRetentionStatus{ID: id, StateConfigured: s.opts.Budget != nil, EnforcementAllowed: s.opts.DatabaseRetentionAllowed, WorkerEnabled: s.opts.LifecycleWorkerEnabled, Runs: []SystemTableRetentionRun{}}
	if !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return out, ErrSystemTableNotFound
	}
	d, _ := s.systemTableDocument(nil)
	if s.opts.Budget != nil {
		var e error
		d, _, e = s.legacySystemRetention(ctx, id)
		if e != nil {
			return out, e
		}
	}
	out.Policy = d.Policies[id]
	out.Policy.Enabled = false
	p, c, e := s.loadSystemRetentionControl(ctx, id)
	out.Configuration = &c
	if e == nil {
		out.Policy = p
		out.Revision = c.DesiredRevision
	} else {
		out.Problem = c.Problem
	}
	for _, r := range d.Runs {
		if r.TableID == id {
			out.Runs = append(out.Runs, r)
			if r.State == "unconfirmed" {
				out.UnconfirmedRuns++
			}
		}
	}
	if s.opts.Control != nil && s.opts.DB != nil {
		rows, e := s.systemRetentionLedger(id).List(ctx)
		if e != nil {
			out.Problem = "SYSTEM_RETENTION_LEDGER_UNAVAILABLE"
		} else {
			current := []SystemTableRetentionRun{}
			for _, row := range rows {
				r, e := decodeSystemRetentionRun(&row, id)
				if e != nil {
					out.Problem = "SYSTEM_RETENTION_LEDGER_UNAVAILABLE"
					break
				}
				current = append(current, r)
			}
			out.Runs = append(current, out.Runs...)
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if e = s.systemRetention.Ready(bounded, id); e != nil {
		out.AdapterProblem = e.Error()
		if out.Problem == "" {
			out.Problem = out.AdapterProblem
		}
	}
	if out.Problem == "" {
		switch {
		case c.Authority != "database":
			out.Problem = "SYSTEM_RETENTION_ADOPTION_REQUIRED"
		case !c.ExecutionReady:
			out.Problem = "SYSTEM_RETENTION_PUBLICATION_PENDING"
		case !out.EnforcementAllowed:
			out.Problem = "system_retention_enforcement_not_allowed"
		case !s.systemRetentionLedger(id).Ready(bounded):
			out.Problem = "SYSTEM_RETENTION_LEDGER_UNAVAILABLE"
		default:
			out.Ready = true
		}
	}
	return out, nil
}

// SaveSystemTableRetention 拒绝旧 JSON 写入；新策略必须明确接管和独立启用。
func (s *Service) SaveSystemTableRetention(ctx context.Context, id string, revision int64, p SystemTableRetentionPolicy) (SystemTableRetentionStatus, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return SystemTableRetentionStatus{}, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return SystemTableRetentionStatus{}, ErrSystemTableNotFound
	}
	return SystemTableRetentionStatus{}, controlError(428, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
}
func (s *Service) runSystemTableRetention(ctx context.Context, onError func(error)) {
	if s.opts.Budget == nil || s.systemRetention == nil || !s.opts.DatabaseRetentionAllowed || !s.opts.LifecycleWorkerEnabled {
		return
	}
	run, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		for i := 0; i < 5; i++ {
			out, e := s.systemTableBatch(run, id, 0, uuid.NewString(), "", true, true)
			if e != nil || out.State == "failed" {
				if onError != nil {
					onError(ErrStateUnavailable)
				}
				break
			}
			if out.Rows < int64(out.Policy.BatchLimit) || out.Policy.BatchLimit == 0 {
				break
			}
		}
	}
}
