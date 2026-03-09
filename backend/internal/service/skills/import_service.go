package skills

import (
	"context"
	"errors"
	"strings"

	"gorm.io/datatypes"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

const ImportTypeUpload = "upload"

// ImportRequest describes one skill import operation.
type ImportRequest struct {
	SkillID    string
	Version    string
	Source     string
	BundleURI  string
	Checksum   string
	Signature  string
	SourceURL  string
	SourceRef  string
	Manifest   datatypes.JSON
	Operator   string
	ImportType string
}

// ImportService validates and creates draft skill records.
type ImportService struct {
	registryRepo *skillrepo.SkillRegistryRepository
	auditService *AuditTraceService
}

func NewImportService(
	registryRepo *skillrepo.SkillRegistryRepository,
	auditService *AuditTraceService,
) *ImportService {
	if registryRepo == nil {
		panic("import service requires registry repository")
	}
	return &ImportService{registryRepo: registryRepo, auditService: auditService}
}

func (s *ImportService) ImportDraft(ctx context.Context, req ImportRequest) (*skillmodel.SkillRegistryRecord, error) {
	req.SkillID = strings.ToLower(strings.TrimSpace(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	req.BundleURI = strings.TrimSpace(req.BundleURI)
	req.Checksum = strings.TrimSpace(req.Checksum)
	req.ImportType = strings.ToLower(strings.TrimSpace(req.ImportType))

	if req.ImportType == "" {
		req.ImportType = ImportTypeUpload
	}
	if req.ImportType != ImportTypeUpload {
		return nil, errors.New("only upload import type is allowed in phase 1")
	}
	if req.SkillID == "" || req.Version == "" {
		return nil, errors.New("skill_id and version are required")
	}
	if req.BundleURI == "" {
		return nil, errors.New("bundle_uri is required")
	}
	if req.Checksum == "" {
		return nil, errors.New("checksum is required")
	}
	if req.Source == skillmodel.SkillSourceBuiltin {
		return nil, errors.New("builtin source should be maintained by official catalog flow")
	}

	record := &skillmodel.SkillRegistryRecord{
		SkillID:      req.SkillID,
		Version:      req.Version,
		Source:       req.Source,
		Status:       skillmodel.SkillStatusDraft,
		BundleURI:    req.BundleURI,
		Checksum:     req.Checksum,
		Signature:    req.Signature,
		ManifestJSON: req.Manifest,
		SourceURL:    req.SourceURL,
		SourceRef:    req.SourceRef,
		ImportType:   req.ImportType,
		UpdatedBy:    req.Operator,
	}

	saved, err := s.registryRepo.UpsertDraft(ctx, record)
	if err != nil {
		return nil, err
	}
	if s.auditService != nil {
		_ = s.auditService.RecordLifecycleAudit(ctx, LifecycleAuditInput{
			Action:   "import",
			SkillID:  saved.SkillID,
			Version:  saved.Version,
			Operator: req.Operator,
			Source:   saved.Source,
			Result:   "success",
			Reason:   "draft imported",
		})
	}
	return saved, nil
}
