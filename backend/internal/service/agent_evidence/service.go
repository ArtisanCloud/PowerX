package agent_evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"slices"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ProjectHistory 不执行工具、不重建 ledger；缺失/清理/无权读取时移除报告及标签，明确不可用。
func (s *Service) ProjectHistory(ctx context.Context, meta datatypes.JSONMap) datatypes.JSONMap {
	envelope, ok := meta["response_envelope"].(map[string]any)
	if !ok {
		envelope, ok = meta["responseEnvelope"].(map[string]any)
	}
	if !ok || envelope["schema"] != evidence.ReportSchemaV5 {
		return meta
	}
	out := datatypes.JSONMap{}
	for k, v := range meta {
		out[k] = v
	}
	delete(out, "responseEnvelope")
	delete(out, "responseLabels")
	raw, _ := json.Marshal(meta["response_evidence"])
	var ref evidence.EvidenceReference
	unavailable := func() datatypes.JSONMap {
		delete(out, "response_envelope")
		delete(out, "response_labels")
		delete(out, "response_evidence")
		out["response_evidence_state"] = "unavailable"
		return out
	}
	if utils.DecodeStrictJSON(raw, &ref) != nil {
		return unavailable()
	}
	a, err := s.Read(ctx, ref)
	if err != nil || a.Report == nil {
		return unavailable()
	}
	canonical := func(value any) []byte {
		raw, _ := json.Marshal(value)
		var m any
		_ = json.Unmarshal(raw, &m)
		normalized, _ := json.Marshal(m)
		return normalized
	}
	if !bytes.Equal(canonical(a.Report), canonical(envelope)) {
		return unavailable()
	}
	out["response_labels"] = a.Labels
	out["response_envelope"] = envelope
	out["response_evidence_state"] = "available"
	return out
}

// Service 是用户态证据存储；当前只启用 run_input + builtin output，外部对象需正式授权端口。
type Service struct {
	repo        *repo.EvidenceSnapshotRepository
	permissions *agentauthz.Service
	catalog     *skillrepo.ToolCatalogRepository
}

func NewService(db *gorm.DB) *Service {
	return &Service{repo: repo.NewEvidenceSnapshotRepository(db), permissions: agentauthz.NewService(db), catalog: skillrepo.NewToolCatalogRepository(db)}
}
func (s *Service) Ready(ctx context.Context) error {
	if s == nil || s.repo.Ready(ctx) != nil {
		return fmt.Errorf("evidence.storage_unavailable")
	}
	return nil
}
func owner(ctx context.Context) (uuid.UUID, uuid.UUID, error) {
	tenant, e1 := uuid.Parse(reqctx.GetTenantUUID(ctx))
	user, e2 := uuid.Parse(reqctx.GetUserUUID(ctx))
	if e1 != nil || e2 != nil || tenant == uuid.Nil || user == uuid.Nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("evidence.source_access_denied")
	}
	return tenant, user, nil
}
func digest(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }
func (s *Service) Save(ctx context.Context, a evidence.RunEvidenceArchive) (evidence.EvidenceReference, error) {
	if err := a.VerifyOrigin(ctx); err != nil {
		return evidence.EvidenceReference{}, err
	}
	if err := s.Ready(ctx); err != nil {
		return evidence.EvidenceReference{}, err
	}
	tenant, user, err := owner(ctx)
	if err != nil || tenant.String() != a.TenantUUID {
		return evidence.EvidenceReference{}, fmt.Errorf("evidence.source_access_denied")
	}
	if err := evidence.ValidateArchive(a); err != nil {
		return evidence.EvidenceReference{}, err
	}
	// 不能靠历史所属用户绕过原始业务对象或外部工具权限。未接入相应读取授权前禁止保存为可回读。
	for _, source := range a.Sources {
		if source.Kind != "run_input" && source.Kind != "tool_output" {
			return evidence.EvidenceReference{}, fmt.Errorf("evidence.source_access_denied")
		}
	}
	if err := s.authorizeArchive(ctx, a); err != nil {
		return evidence.EvidenceReference{}, err
	}
	raw, err := json.Marshal(a)
	if err != nil || len(raw) > 96<<20 {
		return evidence.EvidenceReference{}, fmt.Errorf("evidence.archive_invalid")
	}
	row := &model.EvidenceSnapshot{TenantUUID: tenant, SubjectUUID: user, RevisionUUID: uuid.MustParse(a.RevisionUUID), RunID: a.RunID, TraceID: a.TraceID, SHA256: digest(raw), Payload: raw, Status: 1}
	if err := s.repo.SaveSnapshot(ctx, row); err != nil {
		return evidence.EvidenceReference{}, fmt.Errorf("evidence.storage_unavailable")
	}
	return evidence.EvidenceReference{Schema: evidence.ArchiveSchema, SnapshotUUID: row.UUID.String(), SHA256: row.SHA256}, nil
}
func (s *Service) Read(ctx context.Context, ref evidence.EvidenceReference) (evidence.RunEvidenceArchive, error) {
	deny := func() (evidence.RunEvidenceArchive, error) {
		return evidence.RunEvidenceArchive{}, fmt.Errorf("evidence.snapshot_unavailable")
	}
	if s == nil || s.repo.Ready(ctx) != nil {
		return deny()
	}
	tenant, user, err := owner(ctx)
	if err != nil {
		return deny()
	}
	id, err := uuid.Parse(ref.SnapshotUUID)
	if err != nil || id == uuid.Nil || ref.Schema != evidence.ArchiveSchema {
		return deny()
	}
	row, err := s.repo.ReadSnapshot(ctx, tenant, user, id)
	if err != nil || len(row.Payload) > 96<<20 || row.SHA256 != ref.SHA256 || digest(row.Payload) != ref.SHA256 {
		return deny()
	}
	var a evidence.RunEvidenceArchive
	if utils.DecodeStrictJSON(row.Payload, &a) != nil || evidence.ValidateArchive(a) != nil || a.TenantUUID != tenant.String() || a.RevisionUUID != row.RevisionUUID.String() || a.RunID != row.RunID || a.TraceID != row.TraceID {
		return deny()
	}
	if s.authorizeArchive(ctx, a) != nil {
		return deny()
	}
	return a, nil
}

// 历史只重新判断当前主体/租户/Agent/capability 权限，不要求旧进程运行，不重执行工具。
func (s *Service) authorizeArchive(ctx context.Context, a evidence.RunEvidenceArchive) error {
	deny := func() error { return fmt.Errorf("evidence.source_access_denied") }
	for _, receipt := range a.Receipts {
		if receipt.Tool.Binding.Kind == "builtin" {
			continue
		}
		if receipt.Tool.Binding.Kind != "capability" || receipt.Tool.Permission.Actor != "admin_user" || receipt.Authorization.SubjectUUID != reqctx.GetUserUUID(ctx) || receipt.Tool.Binding.Protocol != "http" {
			return deny()
		}
		agentUUID, err := uuid.Parse(a.AgentUUID)
		if err != nil || agentUUID == uuid.Nil {
			return deny()
		}
		snapshot, err := s.catalog.LoadToolCapability(ctx, a.TenantUUID, receipt.Tool.Binding.CapabilityUUID, receipt.Tool.Binding.ContractUUID)
		if err != nil || snapshot.Record.CapabilityID != receipt.Tool.ToolKey || !slices.Contains(agentauthz.CapabilityPermissionCodes(snapshot.Record), receipt.Tool.Permission.Code) {
			return deny()
		}
		allowed, err := s.permissions.AuthorizeCapability(ctx, agentauthz.AuthorizeInput{TenantUUID: a.TenantUUID, Env: reqctx.GetEnv(ctx), UserUUID: reqctx.GetUserUUID(ctx), MemberID: reqctx.GetMemberID(ctx), IsRoot: reqctx.IsRoot(ctx), AgentUUID: agentUUID, CapabilityID: receipt.Tool.ToolKey, PermissionCode: receipt.Tool.Permission.Code})
		if err != nil || !allowed.Allowed {
			return deny()
		}
	}
	return nil
}
