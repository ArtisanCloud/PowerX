package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	// ErrObjectKeyMustBeUUID 表示 object key 只能为 UUID。
	ErrObjectKeyMustBeUUID = errors.New("object key must be uuid")
)

// UploadMethod 定义媒体上传方式。
type UploadMethod string

const (
	UploadMethodDirect       UploadMethod = "direct_upload"
	UploadMethodExternalLink UploadMethod = "external_link"
	UploadMethodPresign      UploadMethod = "presign_upload"
)

const (
	externalProbeTimeout   = 5 * time.Second
	externalProbeRangeSize = 512
)

// MediaService 聚合媒体资产业务逻辑（状态流转、审计、预签名等）。
type assetRepository interface {
	List(ctx context.Context, filter mediarepo.AssetListFilter) ([]mediamodel.MediaAsset, int64, error)
	FindByUUID(ctx context.Context, tenantUUID string, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error)
	FindByStorageKey(ctx context.Context, tenantUUID string, driver, storageKey string) (*mediamodel.MediaAsset, error)
	ListByDriverAndStorageKey(ctx context.Context, driver, storageKey string) ([]mediamodel.MediaAsset, error)
	CreateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error)
	UpdateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error)
	SoftDeleteByUUID(ctx context.Context, tenantUUID string, uuid string, deletedBy *uint64) error
	ListVariants(ctx context.Context, tenantUUID, assetUUID string, includeDeleted bool) ([]mediamodel.MediaAssetVariant, error)
	HardDeleteByUUID(ctx context.Context, tenantUUID string, uuid string) error
	FindByUUIDGlobal(ctx context.Context, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error)
	FindVariant(ctx context.Context, tenantUUID, assetUUID, variant string) (*mediamodel.MediaAssetVariant, error)
	FindVariantByStorageKey(ctx context.Context, driver, storageKey string) (*mediamodel.MediaAssetVariant, error)
	CreateVariant(ctx context.Context, variant *mediamodel.MediaAssetVariant) (*mediamodel.MediaAssetVariant, error)
	UpdateVariant(ctx context.Context, variant *mediamodel.MediaAssetVariant) (*mediamodel.MediaAssetVariant, error)
}

type MediaService struct {
	*service.BaseService
	repo       assetRepository
	manager    *mediamgr.MediaManager
	audit      auditsvc.Service
	defaultTTL time.Duration
	// publicResourceTokenSecret 用于公开资源入口的临时 token（HMAC）。
	publicResourceTokenSecret []byte
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

// SetPublicResourceTokenSecret 设置公开资源入口的 token 签名密钥。
func (s *MediaService) SetPublicResourceTokenSecret(secret string) {
	if s == nil {
		return
	}
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		s.publicResourceTokenSecret = nil
		return
	}
	s.publicResourceTokenSecret = []byte(trimmed)
}

func (s *MediaService) generatePublicResourceToken(uuid string, expUnix int64) string {
	if s == nil || len(s.publicResourceTokenSecret) == 0 {
		return ""
	}
	id := strings.TrimSpace(uuid)
	if id == "" || expUnix <= 0 {
		return ""
	}
	mac := hmac.New(sha256.New, s.publicResourceTokenSecret)
	mac.Write([]byte(id))
	mac.Write([]byte("\n"))
	mac.Write([]byte(strconv.FormatInt(expUnix, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *MediaService) verifyPublicResourceToken(uuid, expStr, token string) bool {
	if s == nil || len(s.publicResourceTokenSecret) == 0 {
		return false
	}
	id := strings.TrimSpace(uuid)
	if id == "" {
		return false
	}
	expStr = strings.TrimSpace(expStr)
	token = strings.TrimSpace(token)
	if expStr == "" || token == "" {
		return false
	}
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || expUnix <= 0 {
		return false
	}
	// 允许轻微时钟漂移
	if time.Now().Unix() > expUnix+5 {
		return false
	}
	expected := s.generatePublicResourceToken(id, expUnix)
	if expected == "" {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(token))
}

// CanAccessPublicResource 判断公开资源入口是否允许访问。
// - published：允许匿名访问
// - 其它状态：仅当携带有效 token+exp 时允许（用于短期分发/预览）
func (s *MediaService) CanAccessPublicResource(asset *Asset, expStr, token string) bool {
	if asset == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(asset.BusinessStatus), coremodel.MediaAssetStatusPublished) {
		return true
	}
	return s.verifyPublicResourceToken(asset.UUID, expStr, token)
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
	TenantUUID  string
	UUID        string
	Variant     string
	OperatorID  *uint64
	Action      string
	Method      string
	TTL         time.Duration
	Headers     http.Header
	ContentType string
}

// CreateAssetVariantInput 定义创建媒体资产资源版本所需参数。
type CreateAssetVariantInput struct {
	TenantUUID string
	AssetUUID  string
	Variant    string
	OperatorID *uint64
	Name       string
	Driver     string
	Bucket     string
	BaseURL    string
	StorageKey string
	SizeBytes  int64
	MimeType   string
	Metadata   map[string]any
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

// AssetVariant 为媒体资产的资源版本视图。
type AssetVariant struct {
	UUID           string
	TenantUUID     string
	AssetUUID      string
	Variant        string
	Name           string
	Driver         string
	StorageKey     string
	Bucket         string
	BaseURL        string
	SizeBytes      int64
	MimeType       string
	DownloadURL    string
	DownloadExpiry *time.Time
	Metadata       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
		if err := s.populateExternalLinkMetadata(ctx, &in); err != nil {
			return nil, err
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
	var assetUUID uuid.UUID
	if storageKey == "" {
		assetUUID = uuid.New()
		storageKey = mediaAssetOriginStorageKey(assetUUID.String())
	} else {
		parsed, parseErr := uuid.Parse(storageKey)
		if parseErr != nil {
			return nil, ErrObjectKeyMustBeUUID
		}
		assetUUID = parsed
		storageKey = mediaAssetOriginStorageKey(assetUUID.String())
	}

	if existing, findErr := s.repo.FindByStorageKey(ctx, tenantUUID, driverName, storageKey); findErr == nil {
		return toAsset(existing), nil
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return nil, findErr
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
		PowerUUIDModel:          coremodel.PowerUUIDModel{UUID: assetUUID},
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

// OpenAssetResource 返回资产及其二进制内容（若存在本地/对象存储文件）。
// 对于 external_link 能力，objectResult 将为 nil，调用方可根据 Asset.ExternalURL 做跳转。
func (s *MediaService) OpenAssetResource(ctx context.Context, tenantUUID string, uuid string) (*Asset, *driver.GetObjectResult, error) {
	trimmedTenant := strings.TrimSpace(tenantUUID)
	var (
		entity *mediamodel.MediaAsset
		err    error
	)
	if trimmedTenant != "" {
		entity, err = s.repo.FindByUUID(ctx, trimmedTenant, uuid, false)
	} else {
		entity, err = s.repo.FindByUUIDGlobal(ctx, uuid, false)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAssetNotFound
		}
		return nil, nil, err
	}
	asset := toAsset(entity)
	if asset.ExternalURL != "" {
		return asset, nil, nil
	}
	if s.manager == nil {
		return asset, nil, fmt.Errorf("media manager not configured")
	}
	if strings.TrimSpace(entity.StorageKey) == "" {
		return asset, nil, driver.ErrNotFound
	}
	object, err := s.manager.Get(ctx, entity.Driver, driver.GetObjectInput{
		Bucket:    entity.Bucket,
		ObjectKey: entity.StorageKey,
	})
	if err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return nil, nil, ErrAssetNotFound
		}
		return nil, nil, err
	}
	return asset, object, nil
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

// PurgeAsset 永久删除媒体资产及其所有资源版本，并释放底层存储对象。
func (s *MediaService) PurgeAsset(ctx context.Context, in DeleteAssetInput) error {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	assetUUID := strings.TrimSpace(in.UUID)
	if tenantUUID == "" || assetUUID == "" {
		return fmt.Errorf("tenant uuid and asset uuid required")
	}
	entity, err := s.repo.FindByUUID(ctx, tenantUUID, assetUUID, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssetNotFound
		}
		return err
	}
	variants, err := s.repo.ListVariants(ctx, tenantUUID, assetUUID, true)
	if err != nil {
		return err
	}
	if s.manager == nil {
		return fmt.Errorf("media manager not configured")
	}
	deleteObject := func(driverName, bucket, objectKey string) error {
		driverName = strings.TrimSpace(driverName)
		objectKey = strings.TrimSpace(objectKey)
		if driverName == "" || objectKey == "" {
			return nil
		}
		if err := s.manager.Delete(ctx, driverName, driver.DeleteObjectInput{
			Bucket:    strings.TrimSpace(bucket),
			ObjectKey: objectKey,
			Force:     true,
		}); err != nil {
			return err
		}
		return nil
	}
	if err := deleteObject(entity.Driver, entity.Bucket, entity.StorageKey); err != nil {
		return fmt.Errorf("delete media origin object failed: %w", err)
	}
	for _, variant := range variants {
		if err := deleteObject(variant.Driver, variant.Bucket, variant.StorageKey); err != nil {
			return fmt.Errorf("delete media variant object failed: variant=%s: %w", variant.Variant, err)
		}
	}
	if err := s.repo.HardDeleteByUUID(ctx, tenantUUID, assetUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssetNotFound
		}
		return err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.purge", assetUUID, in.OperatorID, map[string]any{
		"variants": len(variants),
	})
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

// CreateAssetVariant 创建或返回媒体资产的资源版本。
func (s *MediaService) CreateAssetVariant(ctx context.Context, in CreateAssetVariantInput) (*AssetVariant, error) {
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	assetUUID := strings.TrimSpace(in.AssetUUID)
	variantName := normalizeVariantName(in.Variant)
	if tenantUUID == "" || assetUUID == "" {
		return nil, fmt.Errorf("tenant uuid and asset uuid required")
	}
	if variantName == "" {
		return nil, fmt.Errorf("variant required")
	}
	if variantName == "origin" {
		asset, err := s.GetAsset(ctx, tenantUUID, assetUUID, false)
		if err != nil {
			return nil, err
		}
		return originAssetVariant(asset), nil
	}
	parent, err := s.repo.FindByUUID(ctx, tenantUUID, assetUUID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFound
		}
		return nil, err
	}
	if existing, findErr := s.repo.FindVariant(ctx, tenantUUID, assetUUID, variantName); findErr == nil {
		return toAssetVariant(existing), nil
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return nil, findErr
	}

	driverName := strings.TrimSpace(in.Driver)
	if driverName == "" {
		driverName = parent.Driver
	}
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
		storageKey = mediaAssetVariantStorageKey(assetUUID, variantName)
	}

	meta := make(map[string]any, len(in.Metadata))
	for k, v := range in.Metadata {
		if strings.TrimSpace(k) == "" || v == nil {
			continue
		}
		meta[k] = v
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encode variant meta failed: %w", err)
	}
	ttlSeconds := int32(s.defaultTTL / time.Second)
	if ttlSeconds <= 0 {
		ttlSeconds = int32((12 * time.Hour) / time.Second)
	}
	entity := &mediamodel.MediaAssetVariant{
		PowerUUIDModel:          coremodel.PowerUUIDModel{UUID: uuid.New()},
		TenantUUID:              tenantUUID,
		AssetUUID:               assetUUID,
		Variant:                 variantName,
		Name:                    strings.TrimSpace(in.Name),
		Driver:                  driverName,
		StorageKey:              storageKey,
		Bucket:                  firstNonEmptyString(strings.TrimSpace(in.Bucket), parent.Bucket),
		BaseURL:                 firstNonEmptyString(strings.TrimSpace(in.BaseURL), parent.BaseURL),
		SizeBytes:               in.SizeBytes,
		MimeType:                strings.TrimSpace(in.MimeType),
		Meta:                    datatypes.JSON(metaJSON),
		LastPresignedTTLSeconds: ttlSeconds,
	}
	if in.OperatorID != nil {
		entity.CreatedBy = in.OperatorID
		entity.UpdatedBy = in.OperatorID
	}
	created, err := s.repo.CreateVariant(ctx, entity)
	if err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantUUID, "media.asset.variant.create", assetUUID, in.OperatorID, map[string]any{"variant": variantName})
	return toAssetVariant(created), nil
}

// OpenAssetVariantResource 返回指定版本的二进制内容。origin 读取主资源。
func (s *MediaService) OpenAssetVariantResource(ctx context.Context, tenantUUID, assetUUID, variant string) (*AssetVariant, *driver.GetObjectResult, error) {
	variantName := normalizeVariantName(variant)
	if variantName == "" {
		return nil, nil, fmt.Errorf("invalid media asset variant")
	}
	if variantName == "origin" {
		asset, object, err := s.OpenAssetResource(ctx, tenantUUID, assetUUID)
		if err != nil {
			return nil, nil, err
		}
		return originAssetVariant(asset), object, nil
	}
	entity, err := s.repo.FindVariant(ctx, strings.TrimSpace(tenantUUID), strings.TrimSpace(assetUUID), variantName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAssetNotFound
		}
		return nil, nil, err
	}
	out := toAssetVariant(entity)
	if s.manager == nil {
		return out, nil, fmt.Errorf("media manager not configured")
	}
	if strings.TrimSpace(entity.StorageKey) == "" {
		return out, nil, driver.ErrNotFound
	}
	object, err := s.manager.Get(ctx, entity.Driver, driver.GetObjectInput{
		Bucket:    entity.Bucket,
		ObjectKey: entity.StorageKey,
	})
	if err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return nil, nil, ErrAssetNotFound
		}
		return nil, nil, err
	}
	return out, object, nil
}

// ListAssetVariants 返回指定媒体资产已生成的资源版本。
func (s *MediaService) ListAssetVariants(ctx context.Context, tenantUUID, assetUUID string, includeDeleted bool) ([]AssetVariant, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("media repository not configured")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	assetUUID = strings.TrimSpace(assetUUID)
	if tenantUUID == "" || assetUUID == "" {
		return nil, fmt.Errorf("tenant uuid and asset uuid required")
	}
	items, err := s.repo.ListVariants(ctx, tenantUUID, assetUUID, includeDeleted)
	if err != nil {
		return nil, err
	}
	out := make([]AssetVariant, 0, len(items))
	for i := range items {
		view := toAssetVariant(&items[i])
		if view == nil {
			continue
		}
		out = append(out, *view)
	}
	return out, nil
}

// PresignAssetVariant 生成指定资源版本的上传/下载链接。
func (s *MediaService) PresignAssetVariant(ctx context.Context, in PresignAssetInput) (*driver.GenerateURLOutput, error) {
	variantName := normalizeVariantName(in.Variant)
	if variantName == "" {
		return nil, fmt.Errorf("invalid media asset variant")
	}
	if variantName == "origin" {
		return s.PresignAsset(ctx, in)
	}
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	entity, err := s.repo.FindVariant(ctx, tenantUUID, strings.TrimSpace(in.UUID), variantName)
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
		if action == "upload" {
			method = http.MethodPut
		} else {
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
	if strings.EqualFold(entity.Driver, "local") {
		switch action {
		case "upload":
			urlOut.URL = fmt.Sprintf("/api/v1/media/assets/%s/variants/%s", entity.AssetUUID, entity.Variant)
		case "download":
			urlOut.URL = fmt.Sprintf("/api/v1/media/assets/%s/variants/%s/resource", entity.AssetUUID, entity.Variant)
		}
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
	updated, updateErr := s.repo.UpdateVariant(ctx, entity)
	if updateErr != nil {
		return nil, updateErr
	}
	entity = updated
	s.emitAudit(ctx, tenantUUID, "media.asset.variant.presign", in.UUID, in.OperatorID, map[string]any{
		"method":  method,
		"ttl":     ttl.Seconds(),
		"action":  action,
		"variant": variantName,
	})
	urlOut.Method = method
	return urlOut, nil
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

	var urlOut *driver.GenerateURLOutput
	if strings.EqualFold(action, "download") && strings.TrimSpace(entity.BaseURL) == "" && strings.EqualFold(entity.Driver, "local") {
		expireAt := time.Now().Add(ttl)
		urlOut = &driver.GenerateURLOutput{
			Bucket:    entity.Bucket,
			ObjectKey: entity.StorageKey,
			Method:    method,
			URL:       fmt.Sprintf("/media/%s/resource", entity.UUID.String()),
			ExpireAt:  expireAt,
			Headers:   in.Headers,
		}
	} else {
		urlOut, err = s.manager.GenerateURL(ctx, entity.Driver, driver.GenerateURLInput{
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
	}
	if strings.EqualFold(entity.Driver, "local") && entity.UUID != uuid.Nil {
		switch strings.ToLower(action) {
		case "upload":
			urlOut.URL = fmt.Sprintf("/api/v1/media/assets/%s", entity.UUID.String())
		case "download":
			if strings.EqualFold(entity.BusinessStatus, coremodel.MediaAssetStatusPublished) {
				urlOut.URL = fmt.Sprintf("/media/%s/resource", entity.UUID.String())
				break
			}
			if len(s.publicResourceTokenSecret) == 0 {
				return nil, fmt.Errorf("storage.local.public_token_secret 未配置，无法为非 published 资源生成可直接访问的下载链接")
			}
			expUnix := urlOut.ExpireAt.Unix()
			token := s.generatePublicResourceToken(entity.UUID.String(), expUnix)
			if token == "" {
				return nil, fmt.Errorf("生成公开下载 token 失败")
			}
			urlOut.URL = fmt.Sprintf("/media/%s/resource?exp=%d&token=%s", entity.UUID.String(), expUnix, token)
		}
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
		if resolved := resolveDownloadURL(entity.BaseURL, entity.StorageKey); resolved != "" {
			downloadURL = resolved
		} else if entity.UUID != uuid.Nil {
			downloadURL = fmt.Sprintf("/media/%s/resource", entity.UUID.String())
		}
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

func toAssetVariant(entity *mediamodel.MediaAssetVariant) *AssetVariant {
	if entity == nil {
		return nil
	}
	meta := extractMeta(entity.Meta)
	downloadURL := stringFromMeta(meta, "last_download_url")
	if downloadURL == "" {
		downloadURL = resolveDownloadURL(entity.BaseURL, entity.StorageKey)
	}
	var downloadExpiry *time.Time
	if entity.LastPresignedAt != nil {
		expiry := *entity.LastPresignedAt
		downloadExpiry = &expiry
	}
	return &AssetVariant{
		UUID:           entity.UUID.String(),
		TenantUUID:     entity.TenantUUID,
		AssetUUID:      entity.AssetUUID,
		Variant:        entity.Variant,
		Name:           entity.Name,
		Driver:         entity.Driver,
		StorageKey:     entity.StorageKey,
		Bucket:         entity.Bucket,
		BaseURL:        entity.BaseURL,
		SizeBytes:      entity.SizeBytes,
		MimeType:       entity.MimeType,
		DownloadURL:    downloadURL,
		DownloadExpiry: downloadExpiry,
		Metadata:       cloneMetadata(meta),
		CreatedAt:      entity.CreatedAt,
		UpdatedAt:      entity.UpdatedAt,
	}
}

func originAssetVariant(asset *Asset) *AssetVariant {
	if asset == nil {
		return nil
	}
	return &AssetVariant{
		UUID:           asset.UUID,
		TenantUUID:     asset.TenantUUID,
		AssetUUID:      asset.UUID,
		Variant:        "origin",
		Name:           asset.Name,
		Driver:         asset.Driver,
		StorageKey:     asset.StorageKey,
		Bucket:         asset.Bucket,
		BaseURL:        asset.BaseURL,
		SizeBytes:      asset.SizeBytes,
		MimeType:       asset.MimeType,
		DownloadURL:    asset.DownloadURL,
		DownloadExpiry: asset.DownloadExpiry,
		Metadata:       cloneMetadata(asset.Metadata),
		CreatedAt:      asset.CreatedAt,
		UpdatedAt:      asset.UpdatedAt,
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

func normalizeVariantName(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "origin"
	}
	switch trimmed {
	case "origin", "preview", "thumbnail":
		return trimmed
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

func mediaAssetOriginStorageKey(assetUUID string) string {
	return mediaAssetVariantStorageKey(assetUUID, "origin")
}

func mediaAssetVariantStorageKey(assetUUID string, variant string) string {
	assetUUID = strings.TrimSpace(assetUUID)
	variant = normalizeVariantName(variant)
	if assetUUID == "" || variant == "" {
		return ""
	}
	return assetUUID + "/" + variant
}

// SyncUploadedFileMetadata 在本地上传完成后回填实际文件大小与 MIME。
func (s *MediaService) SyncUploadedFileMetadata(ctx context.Context, driver, storageKey string, size int64, mimeType string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	driver = strings.TrimSpace(driver)
	storageKey = strings.TrimSpace(storageKey)
	if driver == "" || storageKey == "" {
		return nil
	}
	if variant, err := s.repo.FindVariantByStorageKey(ctx, driver, storageKey); err == nil && variant != nil {
		mimeTrimmed := strings.TrimSpace(mimeType)
		changed := false
		if size > 0 && variant.SizeBytes != size {
			variant.SizeBytes = size
			changed = true
		}
		if mimeTrimmed != "" {
			if strings.TrimSpace(variant.MimeType) == "" || !strings.EqualFold(variant.MimeType, mimeTrimmed) {
				variant.MimeType = mimeTrimmed
				changed = true
			}
		}
		if changed {
			if _, err := s.repo.UpdateVariant(ctx, variant); err != nil {
				return err
			}
		}
		return nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	assets, err := s.repo.ListByDriverAndStorageKey(ctx, driver, storageKey)
	if err != nil {
		return err
	}
	if len(assets) == 0 {
		return fmt.Errorf("uploaded media metadata target not found: driver=%s storage_key=%s", driver, storageKey)
	}
	mimeTrimmed := strings.TrimSpace(mimeType)
	for idx := range assets {
		entity := assets[idx]
		changed := false
		if size > 0 && entity.SizeBytes != size {
			entity.SizeBytes = size
			changed = true
		}
		if mimeTrimmed != "" {
			if strings.TrimSpace(entity.MimeType) == "" || !strings.EqualFold(entity.MimeType, mimeTrimmed) {
				entity.MimeType = mimeTrimmed
				changed = true
			}
		}
		if !changed {
			continue
		}
		if _, err := s.repo.UpdateAsset(ctx, &entity); err != nil {
			return err
		}
	}
	return nil
}

func (s *MediaService) populateExternalLinkMetadata(ctx context.Context, in *CreateAssetInput) error {
	if in == nil {
		return ErrExternalURLRequired
	}
	rawURL := strings.TrimSpace(in.ExternalURL)
	if rawURL == "" {
		return ErrExternalURLRequired
	}
	size, mime, err := probeExternalObject(ctx, rawURL)
	if err != nil {
		return err
	}
	if in.SizeBytes == 0 && size > 0 {
		in.SizeBytes = size
	}
	if strings.TrimSpace(in.MimeType) == "" && mime != "" {
		in.MimeType = mime
	}
	return nil
}

func probeExternalObject(ctx context.Context, target string) (int64, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return 0, "", fmt.Errorf("invalid external url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return 0, "", fmt.Errorf("unsupported external url scheme: %s", parsed.Scheme)
	}
	client := &http.Client{Timeout: externalProbeTimeout}
	if size, mime, err := issueProbeRequest(ctx, client, http.MethodHead, parsed.String()); err == nil {
		return size, mime, nil
	}
	return issueProbeRequest(ctx, client, http.MethodGet, parsed.String())
}

func issueProbeRequest(ctx context.Context, client *http.Client, method, target string) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return 0, "", err
	}
	if method == http.MethodGet {
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", externalProbeRangeSize-1))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, "", fmt.Errorf("probe request failed: %s", resp.Status)
	}
	size := totalSizeFromResponse(resp)
	mime := cleanContentType(resp.Header.Get("Content-Type"))
	if method == http.MethodHead {
		return size, mime, nil
	}
	buf := make([]byte, externalProbeRangeSize)
	n, _ := io.ReadFull(resp.Body, buf)
	if mime == "" && n > 0 {
		mime = http.DetectContentType(buf[:n])
	}
	return size, mime, nil
}

func cleanContentType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, ";"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.ToLower(trimmed)
}

func totalSizeFromResponse(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if resp.StatusCode == http.StatusPartialContent {
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			parts := strings.Split(cr, "/")
			if len(parts) == 2 {
				if total, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
					return total
				}
			}
		}
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}
