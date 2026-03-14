package skills

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/datatypes"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

const ImportTypeUpload = "upload"
const ImportTypeMarketplace = "marketplace"

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
	SourcePath string
	Manifest   datatypes.JSON
	Operator   string
	ImportType string
}

// ImportService validates and creates draft skill records.
type ImportService struct {
	registryRepo *skillrepo.SkillRegistryRepository
	auditService *AuditTraceService
	integrity    *IntegrityPolicy
	resolver     SkillSourceResolver
}

func NewImportService(
	registryRepo *skillrepo.SkillRegistryRepository,
	auditService *AuditTraceService,
) *ImportService {
	if registryRepo == nil {
		panic("import service requires registry repository")
	}
	return &ImportService{
		registryRepo: registryRepo,
		auditService: auditService,
		integrity:    NewIntegrityPolicyFromEnv(),
		resolver:     NewGitHubSkillSourceResolver(),
	}
}

func (s *ImportService) WithSourceResolver(resolver SkillSourceResolver) *ImportService {
	if s == nil {
		return nil
	}
	s.resolver = resolver
	return s
}

func (s *ImportService) PreviewMarketplace(ctx context.Context, req ImportRequest) (*MarketplacePreview, error) {
	if s == nil || s.resolver == nil {
		return nil, errors.New("marketplace source resolver is not configured")
	}
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.SourcePath = strings.TrimSpace(req.SourcePath)
	req.SkillID = strings.TrimSpace(strings.ToLower(req.SkillID))
	if req.SourceURL == "" {
		return nil, errors.New("source_url is required")
	}
	previewer, ok := s.resolver.(SkillSourcePreviewer)
	if !ok {
		return nil, errors.New("source resolver does not support preview")
	}
	return previewer.Preview(ctx, req)
}

func (s *ImportService) ImportDraft(ctx context.Context, req ImportRequest) (*skillmodel.SkillRegistryRecord, error) {
	req.SkillID = strings.ToLower(strings.TrimSpace(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	req.BundleURI = strings.TrimSpace(req.BundleURI)
	req.Checksum = strings.TrimSpace(req.Checksum)
	req.Signature = strings.TrimSpace(req.Signature)
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.SourcePath = strings.TrimSpace(req.SourcePath)
	req.ImportType = strings.ToLower(strings.TrimSpace(req.ImportType))

	if req.ImportType == "" {
		req.ImportType = ImportTypeUpload
	}
	if req.ImportType != ImportTypeUpload && req.ImportType != ImportTypeMarketplace {
		return nil, errors.New("import_type must be upload or marketplace")
	}
	if req.SkillID == "" || req.Version == "" {
		return nil, errors.New("skill_id and version are required")
	}
	if req.Source == skillmodel.SkillSourceBuiltin {
		return nil, errors.New("builtin source should be maintained by official catalog flow")
	}
	if req.ImportType == ImportTypeUpload && (strings.HasPrefix(strings.ToLower(req.BundleURI), "http://") || strings.HasPrefix(strings.ToLower(req.BundleURI), "https://")) {
		return nil, errors.New("remote repository online pull is disabled; only uploaded bundle_uri is allowed")
	}
	if req.ImportType == ImportTypeMarketplace {
		if strings.TrimSpace(req.SourceURL) == "" {
			return nil, errors.New("source_url is required for marketplace import")
		}
		if len(req.Manifest) == 0 {
			resolver := s.resolver
			if resolver == nil {
				return nil, errors.New("marketplace source resolver is not configured")
			}
			resolved, err := resolver.Resolve(ctx, req)
			if err != nil {
				return nil, err
			}
			manifest, err := ParseSkillMarkdownToManifest(resolved.SkillMarkdown, req.Version)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(manifest)
			if err != nil {
				return nil, err
			}
			req.Manifest = datatypes.JSON(raw)
			if strings.TrimSpace(req.BundleURI) == "" {
				req.BundleURI = strings.TrimSpace(resolved.BundleURI)
			}
			if strings.TrimSpace(req.SourceRef) == "" {
				req.SourceRef = strings.TrimSpace(resolved.SourceRef)
			}
		}
	}
	if req.BundleURI == "" {
		return nil, errors.New("bundle_uri is required")
	}

	integrity := s.integrity
	if integrity == nil {
		integrity = NewIntegrityPolicyFromEnv()
	}
	if err := integrity.ValidateImport(req); err != nil {
		return nil, err
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
