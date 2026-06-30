package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm/clause"

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
	bindingRepo  *skillrepo.SkillCapabilityBindingRepository
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

func (s *ImportService) WithCapabilityBindingRepository(bindingRepo *skillrepo.SkillCapabilityBindingRepository) *ImportService {
	if s == nil {
		return nil
	}
	s.bindingRepo = bindingRepo
	return s
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

func (s *ImportService) PublishLatest(ctx context.Context, skillID, version, operator, approvalNote string) error {
	if s == nil || s.registryRepo == nil {
		return errors.New("skill import service is not configured")
	}
	return s.registryRepo.SetLatestPublished(ctx, skillID, version, operator, approvalNote)
}

func (s *ImportService) ImportPluginPublished(ctx context.Context, req ImportRequest, approvalNote string) (*skillmodel.SkillRegistryRecord, error) {
	if s == nil || s.registryRepo == nil {
		return nil, errors.New("skill import service is not configured")
	}
	req.SkillID = strings.ToLower(strings.TrimSpace(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	req.BundleURI = strings.TrimSpace(req.BundleURI)
	req.Checksum = strings.TrimSpace(req.Checksum)
	req.Signature = strings.TrimSpace(req.Signature)
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	req.SourceRef = strings.TrimSpace(req.SourceRef)
	req.ImportType = strings.ToLower(strings.TrimSpace(req.ImportType))
	if req.ImportType == "" {
		req.ImportType = ImportTypeUpload
	}
	if req.SkillID == "" || req.Version == "" {
		return nil, errors.New("skill_id and version are required")
	}
	if req.Source != skillmodel.SkillSourcePlugin {
		return nil, errors.New("plugin published import requires source=plugin")
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
		BundleURI:    req.BundleURI,
		Checksum:     req.Checksum,
		Signature:    req.Signature,
		ManifestJSON: req.Manifest,
		SourceURL:    req.SourceURL,
		SourceRef:    req.SourceRef,
		ImportType:   req.ImportType,
		UpdatedBy:    req.Operator,
	}
	saved, err := s.registryRepo.UpsertPublished(ctx, record, req.Operator, approvalNote)
	if err != nil {
		return nil, err
	}
	if err := s.syncPluginCapabilityBindings(ctx, saved, req.Operator); err != nil {
		return nil, err
	}
	if s.auditService != nil {
		_ = s.auditService.RecordLifecycleAudit(ctx, LifecycleAuditInput{
			Action:   "plugin_sync_publish",
			SkillID:  saved.SkillID,
			Version:  saved.Version,
			Operator: req.Operator,
			Source:   saved.Source,
			Result:   "success",
			Reason:   approvalNote,
		})
	}
	return saved, nil
}

func (s *ImportService) syncPluginCapabilityBindings(ctx context.Context, rec *skillmodel.SkillRegistryRecord, operator string) error {
	if s == nil || s.bindingRepo == nil || rec == nil || len(rec.ManifestJSON) == 0 {
		return nil
	}
	capabilities, err := pluginManifestCapabilityIDs(rec.ManifestJSON)
	if err != nil {
		return err
	}
	for _, capabilityID := range capabilities {
		binding := &skillmodel.SkillCapabilityBinding{
			SkillID:          rec.SkillID,
			Version:          rec.Version,
			CapabilityID:     capabilityID,
			ToolGrants:       datatypes.JSON([]byte("[]")),
			IntentHints:      rec.ManifestJSON,
			Tags:             datatypes.JSON([]byte("[]")),
			SemanticText:     pluginManifestSemanticText(rec.ManifestJSON),
			VisibilityScope:  "tenant",
			BindingStatus:    "active",
			SourceConstraint: skillmodel.SkillSourcePlugin,
			CreatedBy:        operator,
			UpdatedBy:        operator,
		}
		binding.Normalize()
		if _, err := s.bindingRepo.Upsert(ctx, binding, []clause.Column{
			{Name: "skill_id"},
			{Name: "version"},
			{Name: "capability_id"},
		}); err != nil {
			return err
		}
	}
	return nil
}

func pluginManifestCapabilityIDs(raw datatypes.JSON) ([]string, error) {
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("plugin skill manifest json is invalid: %w", err)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(value any) {
		capabilityID := strings.TrimSpace(fmt.Sprint(value))
		if capabilityID == "" || capabilityID == "<nil>" {
			return
		}
		if _, ok := seen[capabilityID]; ok {
			return
		}
		seen[capabilityID] = struct{}{}
		out = append(out, capabilityID)
	}
	if executor, ok := manifest["executor"].(map[string]any); ok {
		add(executor["prepare_capability"])
		if actions, ok := executor["action_map"].(map[string]any); ok {
			for _, capabilityID := range actions {
				add(capabilityID)
			}
		}
	}
	if actions, ok := manifest["action_capabilities"].(map[string]any); ok {
		for _, capabilityID := range actions {
			add(capabilityID)
		}
	}
	return out, nil
}

func pluginManifestSemanticText(raw datatypes.JSON) string {
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	parts := make([]string, 0, 8)
	for _, key := range []string{"title", "description", "capability"} {
		value := strings.TrimSpace(fmt.Sprint(manifest[key]))
		if value != "" && value != "<nil>" {
			parts = append(parts, value)
		}
	}
	if examples, ok := manifest["intent_examples"].([]any); ok {
		for _, example := range examples {
			value := strings.TrimSpace(fmt.Sprint(example))
			if value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, "\n")
}
