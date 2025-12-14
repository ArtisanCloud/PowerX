package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	"github.com/ArtisanCloud/PowerX/internal/service"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	mediarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
)

var (
	// ErrAssetNotFound 表示资产不存在或已被删除。
	ErrAssetNotFound = errors.New("media asset not found")
	// ErrInvalidStatusTransition 表示业务状态流转不合法。
	ErrInvalidStatusTransition = errors.New("invalid media asset status transition")
	// ErrInvalidUploadMethod 表示上传方式不受支持。
	ErrInvalidUploadMethod = errors.New("invalid upload method")
	// ErrExternalURLRequired 表示上传方式为外链时需要提供 URL。
	ErrExternalURLRequired = errors.New("external url required for upload method")
)

// UploadMethod 定义媒体上传方式。
type UploadMethod string

const (
	UploadMethodDirect       UploadMethod = "direct_upload"
	UploadMethodExternalLink UploadMethod = "external_link"
	UploadMethodPresign      UploadMethod = "presign_upload"
)

// MediaService 聚合媒体资产业务逻辑（状态流转、审计、预签名等）。
type assetRepository interface {
	List(ctx context.Context, filter mediarepo.AssetListFilter) ([]mediamodel.MediaAsset, int64, error)
	FindByUUID(ctx context.Context, tenantUUID string, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error)
	CreateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error)
	UpdateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error)
	SoftDeleteByUUID(ctx context.Context, tenantUUID string, uuid string, deletedBy *uint64) error
}

type MediaService struct {
	*service.BaseService
	repo       assetRepository
	manager    *mediamgr.MediaManager
	audit      auditsvc.Service
	defaultTTL time.Duration
}

// NewMediaService 构建媒体服务实例。
func NewMediaService(db *gorm.DB, repo assetRepository, manager *mediamgr.MediaManager, audit auditsvc.Service, defaultTTL time.Duration) *MediaService {
	if repo == nil {
		repo = mediarepo.NewAssetRepository(db)
	}
	if defaultTTL <= 0 {
		defaultTTL = 12 * time.Hour
	}
	return &MediaService{
		BaseService: &service.BaseService{DB: db},
		repo:        repo,
		manager:     manager,
		audit:       audit,
		defaultTTL:  defaultTTL,
	}
}

// CreateAssetInput 定义创建媒体资产所需参数。
type CreateAssetInput struct {
	TenantUUID   string
	OperatorID   *uint64
	Name         string
	Description  string
	Driver       string
	Bucket       string
	BaseURL      string
	Folder       string
	StorageKey   string
	SizeBytes    int64
	MimeType     string
	OwnerType    string
	OwnerID      string
	Tags         []string
	UploadMethod UploadMethod
	ExternalURL  string
	Metadata     map[string]any
}

// UpdateAssetInput 定义更新媒体资产所需参数。
type UpdateAssetInput struct {
	TenantUUID     string
	UUID           string
	OperatorID     *uint64
	Name           *string
	Description    *string
	BusinessStatus *string
	Tags           []string
	Metadata       map[string]any
}

// DeleteAssetInput 定义删除媒体资产所需参数。
type DeleteAssetInput struct {
	TenantUUID string
	UUID       string
	OperatorID *uint64
}

// PresignAssetInput 定义生成预签名链接所需参数。
type PresignAssetInput struct {
	TenantUUID string
	UUID        string
	OperatorID  *uint64
	Action      string
	Method      string
	TTL         time.Duration
	Headers     http.Header
	ContentType string
}

// ListAssetsInput 定义分页查询参数。
type ListAssetsInput struct {
	TenantUUID     string
	UUIDs          []string
	Drivers        []string
	OwnerType      string
	OwnerID        string
	BusinessStatus []string
	Keyword        string
	TagsAll        []string
	IncludeDeleted bool
	OnlyDeleted    bool
	Page           int
	PageSize       int
	OrderBy        string
}

// Asset 为对外视图。
type Asset struct {
	UUID           string
	TenantUUID     string
	Name           string
	Description    string
	Driver         string
	Folder         string
	StorageKey     string
	Bucket         string
	BaseURL        string
	SizeBytes      int64
	MimeType       string
	OwnerType      string
	OwnerID        string
	Tags           []string
	BusinessStatus string
	ExternalURL    string
	DownloadURL    string
	DownloadExpiry *time.Time
	Metadata       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CreatedBy      *uint64
	UpdatedBy      *uint64
	Deleted        bool
}

// CreateAsset 创建媒体资产，默认进入 draft 状态。
func (s *MediaService) CreateAsset(ctx context.Context, in CreateAssetInput) (*Asset, error) {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	if tenantUUID == "" {
		return nil, fmt.Errorf("tenant uuid required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}

	method := in.UploadMethod
	if method == "" {
		method = UploadMethodDirect
	}
	switch method {
	case UploadMethodDirect, UploadMethodPresign:
	// nothing
	case UploadMethodExternalLink:
		if strings.TrimSpace(in.ExternalURL) == "" {
			return nil, ErrExternalURLRequired
		}
	default:
		return nil, ErrInvalidUploadMethod
	}

	driverName := strings.TrimSpace(in.Driver)
	if driverName == "" {
		if s.manager == nil {
			return nil, fmt.Errorf("driver required")
		}
		resolved, err := s.manager.DefaultDriver()
		if err != nil {
			return nil, err
		}
		driverName = resolved
	}
	if s.manager != nil {
		if err := s.manager.EnsureDriver(driverName); err != nil {
			return nil, err
		}
	}

	storageKey := strings.TrimSpace(in.StorageKey)
	if storageKey == "" {
		storageKey = uuid.NewString()
	}

	tags := normalizeTags(in.Tags)
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("encode tags failed: %w", err)
	}

	meta := make(map[string]any, len(in.Metadata)+4)
	for k, v := range in.Metadata {
		if strings.TrimSpace(k) == "" || v == nil {
			continue
		}
		meta[k] = v
	}
	if desc := strings.TrimSpace(in.Description); desc != "" {
		meta["description"] = desc
	}
	if folder := strings.TrimSpace(in.Folder); folder != "" {
		meta["folder"] = folder
	}
	if ext := strings.TrimSpace(in.ExternalURL); ext != "" {
		meta["external_url"] = ext
	}
	meta["upload_method"] = string(method)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encode meta failed: %w", err)
	}

	ttlSeconds := int32(s.defaultTTL / time.Second)
	if ttlSeconds <= 0 {
		ttlSeconds = int32((12 * time.Hour) / time.Second)
	}

	asset := &mediamodel.MediaAsset{
		TenantUUID:              tenantUUID,
		Name:                    name,
		Driver:                  driverName,
		StorageKey:              storageKey,
		Bucket:                  strings.TrimSpace(in.Bucket),
		BaseURL:                 strings.TrimSpace(in.BaseURL),
		SizeBytes:               in.SizeBytes,
		MimeType:                strings.TrimSpace(in.MimeType),
		OwnerType:               strings.TrimSpace(in.OwnerType),
		OwnerID:                 strings.TrimSpace(in.OwnerID),
		BusinessStatus:          coremodel.MediaAssetStatusDraft,
		Tags:                    datatypes.JSON(tagsJSON),
		Meta:                    datatypes.JSON(metaJSON),
		LastPresignedTTLSeconds: ttlSeconds,
	}
	if in.OperatorID != nil {
		asset.CreatedBy = in.OperatorID
		asset.UpdatedBy = in.OperatorID
	}

	created, err := s.repo.CreateAsset(ctx, asset)
	if err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.create", created.UUID.String(), in.OperatorID, map[string]any{"name": created.Name})
	return toAsset(created), nil
}

// ListAssets 按条件分页检索媒体资产。
func (s *MediaService) ListAssets(ctx context.Context, in ListAssetsInput) ([]Asset, int64, error) {
	filter := mediarepo.AssetListFilter{
		TenantUUID:     strings.TrimSpace(in.TenantUUID),
		UUIDs:          normalizeStrings(in.UUIDs),
		Drivers:        normalizeStrings(in.Drivers),
		OwnerType:      strings.TrimSpace(in.OwnerType),
		OwnerID:        strings.TrimSpace(in.OwnerID),
		BusinessStatus: normalizeStrings(in.BusinessStatus),
		Keyword:        strings.TrimSpace(in.Keyword),
		TagsAll:        normalizeTags(in.TagsAll),
		IncludeDeleted: in.IncludeDeleted,
		OnlyDeleted:    in.OnlyDeleted,
		Page:           in.Page,
		PageSize:       in.PageSize,
		OrderBy:        strings.TrimSpace(in.OrderBy),
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	result := make([]Asset, 0, len(items))
	for i := range items {
		result = append(result, *toAsset(&items[i]))
	}
	return result, total, nil
}

// GetAsset 读取单个媒体资产。
func (s *MediaService) GetAsset(ctx context.Context, tenantUUID string, uuid string, includeDeleted bool) (*Asset, error) {
	entity, err := s.repo.FindByUUID(ctx, strings.TrimSpace(tenantUUID), uuid, includeDeleted)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	return toAsset(entity), nil
}

// UpdateAsset 更新媒体资产信息并校验状态机。
func (s *MediaService) UpdateAsset(ctx context.Context, in UpdateAssetInput) (*Asset, error) {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	entity, err := s.repo.FindByUUID(ctx, tenantUUID, in.UUID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}

	if in.Name != nil {
		if trimmed := strings.TrimSpace(*in.Name); trimmed != "" {
			entity.Name = trimmed
		}
	}
	if in.Description != nil {
		meta := extractMeta(entity.Meta)
		if strings.TrimSpace(*in.Description) == "" {
			delete(meta, "description")
		} else {
			meta["description"] = strings.TrimSpace(*in.Description)
		}
		encoded, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("encode meta failed: %w", err)
		}
		entity.Meta = datatypes.JSON(encoded)
	}

	if in.BusinessStatus != nil {
		target := strings.TrimSpace(*in.BusinessStatus)
		if target != "" {
			if err := coremodel.ValidateMediaAssetTransition(entity.BusinessStatus, target); err != nil {
				return nil, ErrInvalidStatusTransition
			}
			entity.BusinessStatus = target
		}
	}
	if in.Tags != nil {
		tags := normalizeTags(in.Tags)
		encoded, err := json.Marshal(tags)
		if err != nil {
			return nil, fmt.Errorf("encode tags failed: %w", err)
		}
		entity.Tags = datatypes.JSON(encoded)
	}
	if in.Metadata != nil {
		meta := extractMeta(entity.Meta)
		for k, v := range in.Metadata {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			if v == nil {
				delete(meta, key)
			} else {
				meta[key] = v
			}
		}
		encoded, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("encode meta failed: %w", err)
		}
		entity.Meta = datatypes.JSON(encoded)
	}
	if in.OperatorID != nil {
		entity.UpdatedBy = in.OperatorID
	}

	updated, err := s.repo.UpdateAsset(ctx, entity)
	if err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.update", updated.UUID.String(), in.OperatorID, map[string]any{"status": updated.BusinessStatus})
	return toAsset(updated), nil
}

// DeleteAsset 执行软删除。
func (s *MediaService) DeleteAsset(ctx context.Context, in DeleteAssetInput) error {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	err := s.repo.SoftDeleteByUUID(ctx, tenantUUID, in.UUID, in.OperatorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssetNotFound
		}
		return err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.delete", in.UUID, in.OperatorID, nil)
	return nil
}

// RollbackAsset 用于异常回滚（测试夹具使用）。
func (s *MediaService) RollbackAsset(ctx context.Context, in DeleteAssetInput) error {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	err := s.repo.SoftDeleteByUUID(ctx, tenantUUID, in.UUID, in.OperatorID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssetNotFound
		}
		return err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.rollback", in.UUID, in.OperatorID, nil)
	return nil
}

// PresignAsset 生成预签名链接并记录审计事件。
func (s *MediaService) PresignAsset(ctx context.Context, in PresignAssetInput) (*driver.GenerateURLOutput, error) {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	entity, err := s.repo.FindByUUID(ctx, tenantUUID, in.UUID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}

	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		action = "download"
	}

	ttl := in.TTL
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	if s.manager == nil {
		return nil, fmt.Errorf("media manager not configured")
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		switch action {
		case "upload":
			method = http.MethodPut
		case "download":
			method = http.MethodGet
		default:
			method = http.MethodGet
		}
	}

	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" && in.Headers != nil {
		contentType = strings.TrimSpace(in.Headers.Get("Content-Type"))
	}
	if contentType != "" {
		if in.Headers == nil {
			in.Headers = http.Header{}
		}
		in.Headers.Set("Content-Type", contentType)
	}

	urlOut, err := s.manager.GenerateURL(ctx, entity.Driver, driver.GenerateURLInput{
		Bucket:      entity.Bucket,
		ObjectKey:   entity.StorageKey,
		Method:      method,
		TTL:         ttl,
		Headers:     in.Headers,
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}

	expireAt := urlOut.ExpireAt
	entity.LastPresignedAt = &expireAt
	ttlSeconds := int32(ttl / time.Second)
	if ttlSeconds <= 0 {
		ttlSeconds = 1
	}
	entity.LastPresignedTTLSeconds = ttlSeconds
	if in.OperatorID != nil {
		entity.UpdatedBy = in.OperatorID
	}

	meta := extractMeta(entity.Meta)
	if strings.EqualFold(action, "download") {
		meta["last_download_url"] = urlOut.URL
		meta["last_download_method"] = method
		if headers := headerToMap(in.Headers); len(headers) > 0 {
			meta["last_download_headers"] = headers
		}
	}
	if encoded, encodeErr := json.Marshal(meta); encodeErr == nil {
		entity.Meta = datatypes.JSON(encoded)
	}

	updated, updateErr := s.repo.UpdateAsset(ctx, entity)
	if updateErr != nil {
		return nil, updateErr
	}
	entity = updated

	s.emitAudit(ctx, tenantUUID, "media.asset.presign", in.UUID, in.OperatorID, map[string]any{
		"method": method,
		"ttl":    ttl.Seconds(),
		"action": action,
	})
	urlOut.Method = method
	return urlOut, nil
}

func (s *MediaService) emitAudit(ctx context.Context, tenantUUID string, operation, resourceID string, operatorID *uint64, meta map[string]any) {
	if s.audit == nil {
		return
	}
	payload, _ := json.Marshal(meta)
	var actorID *int64
	if operatorID != nil {
		v := int64(*operatorID)
		actorID = &v
	}
	_ = s.audit.Emit(ctx, &dbmaudit.AuditEvent{
		OccurredAt:   time.Now(),
		TenantUUID:   tenantUUID,
		Source:       "media.service",
		Operation:    operation,
		ResourceType: "media.asset",
		ResourceID:   resourceID,
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		ActorUserID:  actorID,
		Meta:         datatypes.JSON(payload),
	})
}

func toAsset(entity *mediamodel.MediaAsset) *Asset {
	if entity == nil {
		return nil
	}
	meta := extractMeta(entity.Meta)
	tags := extractTags(entity.Tags)
	description := stringFromMeta(meta, "description")
	externalURL := stringFromMeta(meta, "external_url")
	folder := stringFromMeta(meta, "folder")
	downloadURL := stringFromMeta(meta, "last_download_url")
	if downloadURL == "" {
		downloadURL = stringFromMeta(meta, "download_url")
	}
	if downloadURL == "" {
		downloadURL = resolveDownloadURL(entity.BaseURL, entity.StorageKey)
	}
	var downloadExpiry *time.Time
	if entity.LastPresignedAt != nil {
		expiry := *entity.LastPresignedAt
		downloadExpiry = &expiry
	}

	return &Asset{
		UUID:           entity.UUID.String(),
		TenantUUID:     entity.TenantUUID,
		Name:           entity.Name,
		Description:    description,
		Driver:         entity.Driver,
		Folder:         folder,
		StorageKey:     entity.StorageKey,
		Bucket:         entity.Bucket,
		BaseURL:        entity.BaseURL,
		SizeBytes:      entity.SizeBytes,
		MimeType:       entity.MimeType,
		OwnerType:      entity.OwnerType,
		OwnerID:        entity.OwnerID,
		Tags:           tags,
		BusinessStatus: entity.BusinessStatus,
		ExternalURL:    externalURL,
		DownloadURL:    downloadURL,
		DownloadExpiry: downloadExpiry,
		Metadata:       cloneMetadata(meta),
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
		CreatedBy:      entity.CreatedBy,
		UpdatedBy:      entity.UpdatedBy,
		Deleted:        entity.DeletedAt.Valid,
	}
}

func extractTags(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil
	}
	return normalizeTags(tags)
}

func extractMeta(data datatypes.JSON) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		return map[string]any{}
	}
	return meta
}

func headerToMap(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		result[key] = values[0]
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneMetadata(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(meta))
	for k, v := range meta {
		cloned[k] = v
	}
	return cloned
}

func normalizeTags(tags []string) []string {
	return normalizeStrings(tags)
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func stringFromMeta(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func resolveDownloadURL(baseURL, objectKey string) string {
	base := strings.TrimSpace(baseURL)
	key := strings.TrimSpace(objectKey)
	if base == "" || key == "" {
		return ""
	}
	base = strings.TrimSuffix(base, "/")
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return base
	}
	return base + "/" + key
}
