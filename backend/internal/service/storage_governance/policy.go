package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrStateUnavailable = errors.New("storage state unavailable")
	ErrPolicyInvalid    = errors.New("storage policy invalid")
	ErrPolicyConflict   = errors.New("storage policy revision conflict")
	ErrRootRequired     = errors.New("storage root user required")
)

// HostBudget 是草稿容量目标；尚未成为跨实例原子准入规则。
type HostBudget struct {
	MaxManagedBytes   int64 `json:"max_managed_bytes"`
	MinFreeBytes      int64 `json:"min_free_bytes"`
	CriticalFreeBytes int64 `json:"critical_free_bytes"`
}

// ResourcePolicy 不接受路径、凭据、来源身份或自动启用。保护项只能显式保存，不能改变旧清理器。
type ResourcePolicy struct {
	RetentionMode          string `json:"retention_mode,omitempty"`
	ResourceID             string `json:"resource_id"`
	TargetDays             int    `json:"target_days"`
	MaxBytes               int64  `json:"max_bytes"`
	RetentionCount         int    `json:"retention_count"`
	MinVerifiedPoints      int    `json:"min_verified_points"`
	ProtectLatest          bool   `json:"protect_latest"`
	RequireVerifiedArchive bool   `json:"require_verified_archive"`
}

type PolicyInput struct {
	SchemaVersion    int              `json:"schema_version"`
	ExpectedRevision int64            `json:"expected_revision"`
	State            string           `json:"state"`
	Budget           HostBudget       `json:"budget"`
	Resources        []ResourcePolicy `json:"resources"`
}

type PolicyDocument struct {
	SchemaVersion         int                    `json:"schema_version"`
	Revision              int64                  `json:"revision"`
	State                 string                 `json:"state"`
	Identity              Identity               `json:"identity"`
	Budget                HostBudget             `json:"budget"`
	Resources             []ResourcePolicy       `json:"resources"`
	UpdatedAt             *time.Time             `json:"updated_at"`
	UpdatedBy             uint64                 `json:"updated_by"`
	CleanupEnabled        bool                   `json:"cleanup_enabled"`
	LegacyPoliciesChanged bool                   `json:"legacy_policies_changed"`
	Catalog               []ResourcePolicyOption `json:"catalog,omitempty"`
	CatalogVersion        int                    `json:"catalog_version,omitempty"`
}

type stateStore interface {
	GetPolicy(context.Context, string) (*m.Policy, error)
	SavePolicy(context.Context, *m.Policy, int64) error
	SaveSample(context.Context, *m.Sample) error
	Samples(context.Context, string, time.Time) ([]m.Sample, error)
}

func (s *Service) stateReady() bool {
	knownEnvironment := s.opts.Identity.Environment == "dev" || s.opts.Identity.Environment == "test" || s.opts.Identity.Environment == "staging" || s.opts.Identity.Environment == "prod"
	return s.store != nil && idPattern.MatchString(s.opts.Identity.InstanceID) && knownEnvironment
}

func (s *Service) Policy(ctx context.Context) (PolicyDocument, error) {
	out := PolicyDocument{SchemaVersion: 1, State: "draft", Identity: s.opts.Identity, Resources: []ResourcePolicy{}}
	if !s.stateReady() {
		return out, ErrStateUnavailable
	}
	row, err := s.store.GetPolicy(ctx, s.opts.Identity.InstanceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		out.Catalog = s.policyOptions()
		out.CatalogVersion = 1
		return out, nil
	}
	if err != nil {
		return out, ErrStateUnavailable
	}
	if json.Unmarshal(row.Document, &out) != nil || row.SchemaVersion != 1 || out.SchemaVersion != 1 || out.Revision != row.Revision || out.State != "draft" || out.CleanupEnabled || out.LegacyPoliciesChanged || out.Identity != s.opts.Identity {
		return PolicyDocument{}, ErrStateUnavailable
	}
	out.Catalog = s.policyOptions()
	out.CatalogVersion = 1
	return out, nil
}

func (s *Service) SavePolicy(ctx context.Context, actorID uint64, in PolicyInput) (PolicyDocument, error) {
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	var out PolicyDocument
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !reqctx.IsRoot(ctx) || actorID == 0 || claims.UserID != actorID || claims.Issuer == "powerx-sts" {
		return out, ErrRootRequired
	}
	for _, p := range claims.Platforms {
		if p == "api_key" || p == "service_actor" {
			return out, ErrRootRequired
		}
	}
	if !s.stateReady() {
		return out, ErrStateUnavailable
	}
	if in.SchemaVersion != 1 || in.State != "draft" || in.ExpectedRevision < 0 || in.ExpectedRevision >= maxSafeBytes || len(in.Resources) > 64 ||
		in.Budget.MaxManagedBytes < 1 || in.Budget.MaxManagedBytes > maxSafeBytes || in.Budget.MinFreeBytes < 1 || in.Budget.MinFreeBytes > maxSafeBytes ||
		in.Budget.CriticalFreeBytes < 1 || in.Budget.CriticalFreeBytes >= in.Budget.MinFreeBytes {
		return out, ErrPolicyInvalid
	}
	registered := map[string]Resource{}
	for _, r := range s.opts.Resources {
		registered[r.ID] = r
	}
	seen := map[string]bool{}
	for _, r := range in.Resources {
		source, exists := registered[r.ResourceID]
		if !exists || seen[r.ResourceID] || r.MaxBytes < 1 || r.MaxBytes > in.Budget.MaxManagedBytes || !validResourcePolicy(r, source) {
			return out, ErrPolicyInvalid
		}
		seen[r.ResourceID] = true
	}
	// 主机／文件系统身份缺失仍可观测，但不能保存看似可执行的主机策略。
	if s.opts.Identity.HostID == "" || s.opts.Identity.FilesystemID == "" {
		return out, ErrPolicyInvalid
	}
	current, err := s.Policy(ctx)
	if err != nil {
		return out, err
	}
	if current.Revision != in.ExpectedRevision {
		return out, ErrPolicyConflict
	}
	now := s.opts.Now().UTC()
	resources := append([]ResourcePolicy{}, in.Resources...)
	out = PolicyDocument{SchemaVersion: 1, Revision: in.ExpectedRevision + 1, State: "draft", Identity: s.opts.Identity, Budget: in.Budget, Resources: resources, UpdatedAt: &now, UpdatedBy: actorID}
	raw, err := json.Marshal(out)
	if err != nil {
		return PolicyDocument{}, ErrPolicyInvalid
	}
	row := &m.Policy{TenantID: 0, InstanceID: s.opts.Identity.InstanceID, Revision: out.Revision, SchemaVersion: 1, Document: datatypes.JSON(raw), UpdatedBy: actorID}
	if err = s.store.SavePolicy(ctx, row, in.ExpectedRevision); errors.Is(err, repo.ErrRevisionConflict) {
		return PolicyDocument{}, ErrPolicyConflict
	}
	if err != nil {
		return PolicyDocument{}, ErrStateUnavailable
	}
	out.Catalog = s.policyOptions() // 运行目录只在响应附加，不写入草稿／审计文档。
	out.CatalogVersion = 1
	return out, nil
}
