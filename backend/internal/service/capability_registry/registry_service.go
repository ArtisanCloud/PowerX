package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RegistryServiceOptions configures RegistryService.
type RegistryServiceOptions struct {
	DB           *gorm.DB
	Redis        redis.UniversalClient
	RecordRepo   *repo.CapabilityRecordRepository
	TemplateRepo *repo.WorkflowTemplateRepository
	JobRepo      *repo.CapabilitySyncJobRepository
	Clock        func() time.Time
	DefaultLimit int
	MaxLimit     int
}

// RegistryService provides read APIs for capability catalog consumers.
type RegistryService struct {
	records      *repo.CapabilityRecordRepository
	templates    *repo.WorkflowTemplateRepository
	jobs         *repo.CapabilitySyncJobRepository
	now          func() time.Time
	defaultLimit int
	maxLimit     int
}

// NewRegistryService builds a RegistryService with sane defaults.
func NewRegistryService(opts RegistryServiceOptions) *RegistryService {
	if opts.RecordRepo == nil {
		if opts.DB == nil {
			panic("registry service requires DB or record repository")
		}
		opts.RecordRepo = repo.NewCapabilityRecordRepository(opts.DB, opts.Redis)
	}
	if opts.TemplateRepo == nil {
		if opts.DB == nil {
			panic("registry service requires DB or template repository")
		}
		opts.TemplateRepo = repo.NewWorkflowTemplateRepository(opts.DB)
	}
	if opts.JobRepo == nil {
		if opts.DB == nil {
			panic("registry service requires DB or job repository")
		}
		opts.JobRepo = repo.NewCapabilitySyncJobRepository(opts.DB)
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	defaultLimit := opts.DefaultLimit
	if defaultLimit <= 0 {
		defaultLimit = 50
	}
	maxLimit := opts.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 200
	}

	return &RegistryService{
		records:      opts.RecordRepo,
		templates:    opts.TemplateRepo,
		jobs:         opts.JobRepo,
		now:          clock,
		defaultLimit: defaultLimit,
		maxLimit:     maxLimit,
	}
}

// CapabilityListOptions describes filters supported by ListCapabilities.
type CapabilityListOptions struct {
	PluginID                 string
	Intent                   string
	Source                   string
	Protocol                 string
	Protocols                []string
	ToolScope                string
	TenantUUID               string
	ToolGrantIDs             []string
	Status                   []string
	Search                   string
	Limit                    int
	Offset                   int
	IncludeWorkflowTemplates bool
	IncludeTotal             bool
}

// CapabilityRecordView extends CapabilityRecord with optional workflow templates.
type CapabilityRecordView struct {
	Record            *models.CapabilityRecord
	WorkflowTemplates []models.WorkflowTemplateRef
}

// ListCapabilities returns filtered capability records and optional total.
func (s *RegistryService) ListCapabilities(ctx context.Context, opts CapabilityListOptions) ([]CapabilityRecordView, int64, error) {
	filter := repo.CapabilityRecordFilter{
		PluginID: opts.PluginID,
		Status:   opts.Status,
		Limit:    s.normalizeLimit(opts.Limit),
		Offset:   opts.Offset,
	}
	postFilter := requiresPostFilter(opts)
	if postFilter {
		filter.Limit = 0
		filter.Offset = 0
	}
	includeTotal := opts.IncludeTotal && !postFilter
	var total int64
	if includeTotal {
		count, err := s.records.Count(ctx, filter)
		if err != nil {
			return nil, 0, err
		}
		total = count
	}
	records, err := s.records.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	filtered := make([]CapabilityRecordView, 0, len(records))
	matched := 0
	limit := s.normalizeLimit(opts.Limit)
	offset := opts.Offset
	for i := range records {
		record := records[i]
		if !recordMatchesFilters(record, opts) {
			continue
		}
		if postFilter {
			if matched < offset {
				matched++
				continue
			}
			if len(filtered) >= limit {
				matched++
				continue
			}
		}
		matched++
		recordPtr := &records[i]
		view := CapabilityRecordView{Record: recordPtr}
		if opts.IncludeWorkflowTemplates {
			templates, err := s.loadWorkflowTemplates(ctx, recordPtr)
			if err != nil {
				return nil, 0, err
			}
			view.WorkflowTemplates = templates
		}
		filtered = append(filtered, view)
	}
	if !includeTotal {
		total = int64(matched)
	}
	return filtered, total, nil
}

// GetCapability returns the capability record and optional workflow templates.
func (s *RegistryService) GetCapability(ctx context.Context, capabilityID string, includeWorkflows bool) (CapabilityRecordView, error) {
	rec, err := s.records.GetByCapabilityID(ctx, capabilityID)
	if err != nil {
		return CapabilityRecordView{}, err
	}
	view := CapabilityRecordView{Record: rec}
	if includeWorkflows {
		templates, err := s.loadWorkflowTemplates(ctx, rec)
		if err != nil {
			return CapabilityRecordView{}, err
		}
		view.WorkflowTemplates = templates
	}
	return view, nil
}

// CapabilitySyncJobListOptions describes filters for ListSyncJobs.
type CapabilitySyncJobListOptions struct {
	PluginID     string
	CapabilityID string
	Status       []string
	Limit        int
	Offset       int
}

// ListSyncJobs returns recent sync job records.
func (s *RegistryService) ListSyncJobs(ctx context.Context, opts CapabilitySyncJobListOptions) ([]models.CapabilitySyncJob, error) {
	filter := repo.CapabilitySyncJobFilter{
		PluginID:     opts.PluginID,
		CapabilityID: opts.CapabilityID,
		Status:       opts.Status,
		Limit:        s.normalizeLimit(opts.Limit),
		Offset:       opts.Offset,
	}
	return s.jobs.List(ctx, filter)
}

func (s *RegistryService) normalizeLimit(value int) int {
	if value <= 0 {
		return s.defaultLimit
	}
	if value > s.maxLimit {
		return s.maxLimit
	}
	return value
}

func recordMatchesFilters(record models.CapabilityRecord, opts CapabilityListOptions) bool {
	if source := strings.TrimSpace(opts.Source); source != "" && !strings.EqualFold(source, CapabilitySource(&record)) {
		return false
	}
	if opts.Intent != "" && !jsonArrayContains(record.Intents, opts.Intent) {
		return false
	}
	if opts.ToolScope != "" && !jsonArrayContains(record.ToolScope, opts.ToolScope) {
		return false
	}
	if opts.Protocol != "" && !protocolBindingsContain(record.Protocols, opts.Protocol) {
		return false
	}
	if len(opts.Protocols) > 0 {
		found := false
		for _, channel := range opts.Protocols {
			if protocolBindingsContain(record.Protocols, channel) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if strings.TrimSpace(opts.Search) != "" {
		key := strings.ToLower(strings.TrimSpace(opts.Search))
		if !strings.Contains(strings.ToLower(record.CapabilityID), key) &&
			!strings.Contains(strings.ToLower(record.Title), key) &&
			!strings.Contains(strings.ToLower(record.PluginID), key) {
			return false
		}
	}

	needsPolicy := opts.TenantUUID != "" || len(opts.ToolGrantIDs) > 0
	if needsPolicy {
		policy := parseCapabilityPolicy(record.Policy)
		if opts.TenantUUID != "" && !policy.AllowsTenant(opts.TenantUUID) {
			return false
		}
		if len(opts.ToolGrantIDs) > 0 && !policy.AllowsToolGrants(opts.ToolGrantIDs) {
			return false
		}
	}
	return true
}

func requiresPostFilter(opts CapabilityListOptions) bool {
	if opts.Intent != "" || opts.ToolScope != "" || opts.Protocol != "" ||
		len(opts.Protocols) > 0 || strings.TrimSpace(opts.Search) != "" ||
		strings.TrimSpace(opts.Source) != "" ||
		opts.TenantUUID != "" || len(opts.ToolGrantIDs) > 0 {
		return true
	}
	return false
}

func (s *RegistryService) loadWorkflowTemplates(ctx context.Context, record *models.CapabilityRecord) ([]models.WorkflowTemplateRef, error) {
	if record == nil {
		return nil, errors.New("capability record is nil")
	}
	if cached, err := parseWorkflowTemplateSnapshots(record); err == nil && len(cached) > 0 {
		return cached, nil
	}
	return s.templates.ListByCapabilityID(ctx, record.CapabilityID)
}

func parseWorkflowTemplateSnapshots(record *models.CapabilityRecord) ([]models.WorkflowTemplateRef, error) {
	if record == nil {
		return nil, nil
	}
	raw := bytes.TrimSpace([]byte(record.WorkflowTemplateRefs))
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("[]")) {
		return nil, nil
	}
	var snapshots []workflowTemplateSnapshot
	if err := json.Unmarshal(raw, &snapshots); err != nil {
		return nil, err
	}
	templates := make([]models.WorkflowTemplateRef, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.TemplateID) == "" {
			continue
		}
		requiresUpgrade := true
		if snapshot.RequiresManualUpgrade != nil {
			requiresUpgrade = *snapshot.RequiresManualUpgrade
		}
		templates = append(templates, models.WorkflowTemplateRef{
			CapabilityID:          record.CapabilityID,
			TemplateID:            snapshot.TemplateID,
			Name:                  snapshot.Name,
			Description:           snapshot.Description,
			Steps:                 toDatatypesJSON(snapshot.Steps),
			ParamsSchema:          toDatatypesJSON(snapshot.ParamsSchema),
			ProtocolRequirements:  toDatatypesJSON(snapshot.ProtocolRequirements),
			RequiresManualUpgrade: requiresUpgrade,
		})
	}
	return templates, nil
}

type workflowTemplateSnapshot struct {
	TemplateID            string          `json:"template_id"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	Steps                 json.RawMessage `json:"steps"`
	ParamsSchema          json.RawMessage `json:"params_schema"`
	ProtocolRequirements  json.RawMessage `json:"protocol_requirements"`
	RequiresManualUpgrade *bool           `json:"requires_manual_upgrade"`
}

func toDatatypesJSON(raw json.RawMessage) datatypes.JSON {
	if len(raw) == 0 {
		return datatypes.JSON([]byte("null"))
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return datatypes.JSON(cp)
}

type capabilityPolicy struct {
	Prefer               string               `json:"prefer"`
	Fallback             []string             `json:"fallback"`
	RollbackCapabilityID string               `json:"rollback_capability_id"`
	Visibility           capabilityVisibility `json:"visibility"`
}

type capabilityVisibility struct {
	Tenants      capabilityVisibilityRule `json:"tenants"`
	ToolGrants   capabilityVisibilityRule `json:"tool_grants"`
	ToolGrantIDs capabilityVisibilityRule `json:"tool_grant_ids"`
	Channels     capabilityVisibilityRule `json:"channels"`
}

func (v capabilityVisibility) toolGrantRule() capabilityVisibilityRule {
	return v.ToolGrants.merge(v.ToolGrantIDs)
}

type capabilityVisibilityRule struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

func (r capabilityVisibilityRule) merge(other capabilityVisibilityRule) capabilityVisibilityRule {
	merged := capabilityVisibilityRule{
		Allow: append([]string{}, r.Allow...),
		Deny:  append([]string{}, r.Deny...),
	}
	merged.Allow = append(merged.Allow, other.Allow...)
	merged.Deny = append(merged.Deny, other.Deny...)
	merged.Allow = dedupStrings(merged.Allow)
	merged.Deny = dedupStrings(merged.Deny)
	return merged
}

func (p capabilityPolicy) AllowsTenant(tenant string) bool {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return true
	}
	return p.Visibility.Tenants.permitsValue(tenant)
}

func (p capabilityPolicy) AllowsToolGrants(grants []string) bool {
	return p.Visibility.toolGrantRule().permitsAny(grants)
}

func (r capabilityVisibilityRule) permitsValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return len(r.Allow) == 0
	}
	if containsFold(r.Deny, value) {
		return false
	}
	if len(r.Allow) == 0 {
		return true
	}
	return containsFold(r.Allow, value)
}

func (r capabilityVisibilityRule) permitsAny(values []string) bool {
	valid := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			valid = append(valid, value)
		}
	}
	if len(valid) == 0 {
		return len(r.Allow) == 0
	}
	for _, v := range valid {
		if containsFold(r.Deny, v) {
			return false
		}
	}
	if len(r.Allow) == 0 {
		return true
	}
	for _, v := range valid {
		if containsFold(r.Allow, v) {
			return true
		}
	}
	return false
}

func containsFold(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), needle) {
			return true
		}
	}
	return false
}

func dedupStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseCapabilityPolicy(raw []byte) capabilityPolicy {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return capabilityPolicy{}
	}
	var policy capabilityPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return capabilityPolicy{}
	}
	return policy
}

func jsonArrayContains(data []byte, needle string) bool {
	if len(data) == 0 || needle == "" {
		return false
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return false
	}
	needle = strings.TrimSpace(needle)
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func protocolBindingsContain(raw []byte, channel string) bool {
	if len(raw) == 0 || channel == "" {
		return false
	}
	var bindings []models.ProtocolBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return false
	}
	channel = strings.TrimSpace(channel)
	for _, binding := range bindings {
		if strings.EqualFold(binding.Channel, channel) {
			return true
		}
	}
	return false
}
