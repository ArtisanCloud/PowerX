package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	skillsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/skills/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

type adminServer struct {
	skillsv1.UnimplementedSkillAdminServiceServer
	registryRepo *skillrepo.SkillRegistryRepository
	bindingRepo  *skillrepo.SkillCapabilityBindingRepository
	importSvc    *skillservice.ImportService
	lifecycleSvc *skillservice.LifecycleService
}

// RegisterAdminService wires SkillAdminService.
func RegisterAdminService(registrar grpc.ServiceRegistrar, deps *shared.Deps) {
	if registrar == nil || deps == nil || deps.DB == nil {
		return
	}
	registryRepo := skillrepo.NewSkillRegistryRepository(deps.DB)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(deps.DB)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(deps.DB)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(deps.DB)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	skillsv1.RegisterSkillAdminServiceServer(registrar, &adminServer{
		registryRepo: registryRepo,
		bindingRepo:  bindingRepo,
		importSvc: skillservice.NewImportService(registryRepo, auditSvc).
			WithCapabilityBindingRepository(bindingRepo),
		lifecycleSvc: skillservice.NewLifecycleService(registryRepo, auditSvc),
	})
}

func (s *adminServer) ListSkills(ctx context.Context, req *skillsv1.ListSkillsRequest) (*skillsv1.ListSkillsResponse, error) {
	if req == nil {
		req = &skillsv1.ListSkillsRequest{}
	}
	filter := skillrepo.SkillRegistryFilter{
		SkillID:  strings.TrimSpace(req.GetSkillId()),
		Status:   splitCSV(req.GetStatus()),
		Source:   splitCSV(req.GetSource()),
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
	}
	rows, total, err := s.registryRepo.List(ctx, filter)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*skillsv1.SkillRecord, 0, len(rows))
	for i := range rows {
		items = append(items, toProtoSkillRecord(&rows[i]))
	}
	return &skillsv1.ListSkillsResponse{
		Page:     int32(maxInt(filter.Page, 1)),
		PageSize: int32(normalizedPageSize(filter.PageSize)),
		Total:    total,
		Items:    items,
	}, nil
}

func (s *adminServer) ImportSkill(ctx context.Context, req *skillsv1.ImportSkillRequest) (*skillsv1.ImportSkillResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	record, err := s.importSvc.ImportDraft(ctx, skillservice.ImportRequest{
		SkillID:    req.GetSkillId(),
		Version:    req.GetVersion(),
		Source:     req.GetSource(),
		BundleURI:  req.GetBundleUri(),
		Checksum:   req.GetChecksum(),
		Signature:  req.GetSignature(),
		SourceURL:  req.GetSourceUrl(),
		SourceRef:  req.GetSourceRef(),
		ImportType: skillservice.ImportTypeUpload,
		Operator:   "grpc.admin",
	})
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &skillsv1.ImportSkillResponse{Skill: toProtoSkillRecord(record)}, nil
}

func (s *adminServer) PublishSkill(ctx context.Context, req *skillsv1.PublishSkillRequest) (*skillsv1.PublishSkillResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if err := s.lifecycleSvc.Publish(ctx, req.GetSkillId(), req.GetVersion(), "grpc.admin", req.GetApprovalNote()); err != nil {
		return nil, mapGRPCError(err)
	}
	record, err := s.registryRepo.GetBySkillVersion(ctx, req.GetSkillId(), req.GetVersion())
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &skillsv1.PublishSkillResponse{Skill: toProtoSkillRecord(record)}, nil
}

func (s *adminServer) RollbackSkill(ctx context.Context, req *skillsv1.RollbackSkillRequest) (*skillsv1.RollbackSkillResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if err := s.lifecycleSvc.Rollback(ctx, req.GetSkillId(), req.GetTargetVersion(), "grpc.admin", req.GetReason()); err != nil {
		return nil, mapGRPCError(err)
	}
	record, err := s.registryRepo.GetBySkillVersion(ctx, req.GetSkillId(), req.GetTargetVersion())
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &skillsv1.RollbackSkillResponse{Skill: toProtoSkillRecord(record)}, nil
}

func (s *adminServer) BindCapability(ctx context.Context, req *skillsv1.BindCapabilityRequest) (*skillsv1.BindCapabilityResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.GetSkillId()) == "" || strings.TrimSpace(req.GetVersion()) == "" || strings.TrimSpace(req.GetCapabilityId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "skill_id/version/capability_id are required")
	}
	grants, err := json.Marshal(req.GetToolGrants())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tool_grants: %v", err)
	}
	binding := &skillmodel.SkillCapabilityBinding{
		SkillID:      req.GetSkillId(),
		Version:      req.GetVersion(),
		CapabilityID: req.GetCapabilityId(),
		ToolGrants:   datatypes.JSON(grants),
		CreatedBy:    "grpc.admin",
		UpdatedBy:    "grpc.admin",
	}
	binding.Normalize()
	saved, err := s.bindingRepo.Upsert(ctx, binding, []clause.Column{
		{Name: "skill_id"},
		{Name: "version"},
		{Name: "capability_id"},
	})
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &skillsv1.BindCapabilityResponse{
		BindingId: fmt.Sprintf("%d", saved.ID),
		Status:    saved.BindingStatus,
	}, nil
}

func toProtoSkillRecord(rec *skillmodel.SkillRegistryRecord) *skillsv1.SkillRecord {
	if rec == nil {
		return nil
	}
	return &skillsv1.SkillRecord{
		SkillId:           rec.SkillID,
		Version:           rec.Version,
		Source:            rec.Source,
		Status:            rec.Status,
		BundleUri:         rec.BundleURI,
		Checksum:          rec.Checksum,
		Signature:         rec.Signature,
		IsLatestPublished: rec.IsLatestPublished,
		UpdatedAt:         rec.UpdatedAt.Format(time.RFC3339),
	}
}

func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, skillrepo.ErrSkillNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") || strings.Contains(msg, "must") {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.TrimSpace(strings.ToLower(p))
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizedPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	if pageSize > 200 {
		return 200
	}
	return pageSize
}

func maxInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}
