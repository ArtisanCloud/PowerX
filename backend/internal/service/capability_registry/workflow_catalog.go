package capability_registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultWorkflowCatalogKey = "capability_registry:workflow_catalog"
	defaultWorkflowCatalogTTL = 3 * time.Minute
)

// WorkflowCatalogOptions 配置 Workflow Catalog 服务。
type WorkflowCatalogOptions struct {
	TemplateRepo *repo.WorkflowTemplateRepository
	RecordRepo   *repo.CapabilityRecordRepository
	Redis        redis.UniversalClient
	CacheKey     string
	CacheTTL     time.Duration
	Clock        func() time.Time
	Telemetry    WorkflowCatalogTelemetry
}

// WorkflowCatalog 负责汇总 Registry 中的 Workflow 模板并缓存给 Builder/Engine 使用。
type WorkflowCatalog struct {
	templates *repo.WorkflowTemplateRepository
	records   *repo.CapabilityRecordRepository
	redis     redis.UniversalClient
	cacheKey  string
	cacheTTL  time.Duration
	now       func() time.Time
	telemetry WorkflowCatalogTelemetry
}

// WorkflowCatalogTelemetry 允许订阅 Workflow Catalog 快照。
type WorkflowCatalogTelemetry interface {
	ObserveWorkflowCatalogSnapshot(ctx context.Context, snapshot WorkflowCatalogSnapshot)
}

// WorkflowCatalogSnapshot 描述一次全量模板快照。
type WorkflowCatalogSnapshot struct {
	Version     string                    `json:"version"`
	GeneratedAt time.Time                 `json:"generated_at"`
	Templates   []WorkflowCatalogTemplate `json:"templates"`
}

// WorkflowCatalogTemplate 为 Workflow Builder 暴露单个模板的元数据。
type WorkflowCatalogTemplate struct {
	CapabilityID          string          `json:"capability_id"`
	CapabilityTitle       string          `json:"capability_title,omitempty"`
	PluginID              string          `json:"plugin_id,omitempty"`
	TemplateID            string          `json:"template_id"`
	Name                  string          `json:"name"`
	Description           string          `json:"description,omitempty"`
	Steps                 json.RawMessage `json:"steps,omitempty"`
	ParamsSchema          json.RawMessage `json:"params_schema,omitempty"`
	ProtocolRequirements  json.RawMessage `json:"protocol_requirements,omitempty"`
	CapabilitiesHash      string          `json:"capabilities_hash"`
	TemplateHash          string          `json:"template_hash"`
	RequiresManualUpgrade bool            `json:"requires_manual_upgrade"`
	LastSyncedAt          *time.Time      `json:"last_synced_at,omitempty"`
}

// NewWorkflowCatalog 构造 Workflow Catalog 服务。
func NewWorkflowCatalog(opts WorkflowCatalogOptions) *WorkflowCatalog {
	if opts.TemplateRepo == nil {
		panic("workflow catalog requires template repository")
	}
	cacheKey := strings.TrimSpace(opts.CacheKey)
	if cacheKey == "" {
		cacheKey = defaultWorkflowCatalogKey
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultWorkflowCatalogTTL
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &WorkflowCatalog{
		templates: opts.TemplateRepo,
		records:   opts.RecordRepo,
		redis:     opts.Redis,
		cacheKey:  cacheKey,
		cacheTTL:  cacheTTL,
		now:       clock,
		telemetry: opts.Telemetry,
	}
}

// Refresh 构建新的快照并写入缓存。
func (c *WorkflowCatalog) Refresh(ctx context.Context) (WorkflowCatalogSnapshot, error) {
	if c == nil {
		return WorkflowCatalogSnapshot{}, errors.New("workflow catalog is nil")
	}
	templates, err := c.templates.ListAll(ctx)
	if err != nil {
		return WorkflowCatalogSnapshot{}, fmt.Errorf("list workflow templates: %w", err)
	}
	items, err := c.buildTemplates(ctx, templates)
	if err != nil {
		return WorkflowCatalogSnapshot{}, err
	}
	snapshot := WorkflowCatalogSnapshot{
		Version:     catalogVersion(items),
		GeneratedAt: c.now().UTC(),
		Templates:   items,
	}
	if err := c.cacheSnapshot(ctx, snapshot); err != nil {
		return WorkflowCatalogSnapshot{}, err
	}
	if c.telemetry != nil {
		c.telemetry.ObserveWorkflowCatalogSnapshot(ctx, snapshot)
	}
	return snapshot, nil
}

// Snapshot 尝试从缓存读取快照，若不存在则刷新。
func (c *WorkflowCatalog) Snapshot(ctx context.Context) (WorkflowCatalogSnapshot, error) {
	if c == nil {
		return WorkflowCatalogSnapshot{}, errors.New("workflow catalog is nil")
	}
	if !isNilUniversalClient(c.redis) {
		raw, err := c.redis.Get(ctx, c.cacheKey).Bytes()
		if err == nil && len(raw) > 0 {
			var snapshot WorkflowCatalogSnapshot
			if unmarshalErr := json.Unmarshal(raw, &snapshot); unmarshalErr == nil {
				return snapshot, nil
			}
		} else if err != nil && !errors.Is(err, redis.Nil) {
			return WorkflowCatalogSnapshot{}, err
		}
	}
	return c.Refresh(ctx)
}

// cacheSnapshot 将快照写入 Redis。
func (c *WorkflowCatalog) cacheSnapshot(ctx context.Context, snapshot WorkflowCatalogSnapshot) error {
	if c == nil || isNilUniversalClient(c.redis) {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return c.redis.Set(ctx, c.cacheKey, payload, c.cacheTTL).Err()
}

func (c *WorkflowCatalog) buildTemplates(ctx context.Context, templates []models.WorkflowTemplateRef) ([]WorkflowCatalogTemplate, error) {
	items := make([]WorkflowCatalogTemplate, 0, len(templates))
	cache := make(map[string]*models.CapabilityRecord)
	for _, tpl := range templates {
		entry := WorkflowCatalogTemplate{
			CapabilityID:          tpl.CapabilityID,
			TemplateID:            tpl.TemplateID,
			Name:                  tpl.Name,
			Description:           tpl.Description,
			Steps:                 cloneJSON(tpl.Steps),
			ParamsSchema:          cloneJSON(tpl.ParamsSchema),
			ProtocolRequirements:  cloneJSON(tpl.ProtocolRequirements),
			CapabilitiesHash:      tpl.CapabilitiesHash,
			TemplateHash:          tpl.TemplateHash,
			RequiresManualUpgrade: tpl.RequiresManualUpgrade,
			LastSyncedAt:          tpl.LastSyncedAt,
		}
		if c.records != nil && tpl.CapabilityID != "" {
			record, err := c.lookupRecord(ctx, tpl.CapabilityID, cache)
			if err != nil {
				return nil, err
			}
			if record != nil {
				entry.PluginID = record.PluginID
				entry.CapabilityTitle = record.Title
			}
		}
		items = append(items, entry)
	}
	return items, nil
}

func (c *WorkflowCatalog) lookupRecord(ctx context.Context, capabilityID string, cache map[string]*models.CapabilityRecord) (*models.CapabilityRecord, error) {
	if capabilityID == "" {
		return nil, nil
	}
	if record, ok := cache[capabilityID]; ok {
		return record, nil
	}
	if c.records == nil {
		cache[capabilityID] = nil
		return nil, nil
	}
	record, err := c.records.GetByCapabilityID(ctx, capabilityID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cache[capabilityID] = nil
			return nil, nil
		}
		return nil, err
	}
	cache[capabilityID] = record
	return record, nil
}

func catalogVersion(templates []WorkflowCatalogTemplate) string {
	hasher := sha256.New()
	for _, tpl := range templates {
		_, _ = hasher.Write([]byte(strings.TrimSpace(tpl.CapabilityID)))
		_, _ = hasher.Write([]byte(strings.TrimSpace(tpl.TemplateID)))
		_, _ = hasher.Write([]byte(strings.TrimSpace(tpl.TemplateHash)))
		_, _ = hasher.Write([]byte(strings.TrimSpace(tpl.CapabilitiesHash)))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func cloneJSON(src datatypes.JSON) json.RawMessage {
	if len(src) == 0 {
		return nil
	}
	cp := make([]byte, len(src))
	copy(cp, src)
	return json.RawMessage(cp)
}
