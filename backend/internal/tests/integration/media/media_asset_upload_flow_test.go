package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	mediarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type MediaUploadRequest struct {
	TenantID         string
	OperatorID       string
	Name             string
	Driver           string
	UploadMethod     string
	ExternalURL      string
	Tags             []string
	PresignTokenID   string
	PresignExpiresIn int64
}

type mediaIntegrationTestEnv struct {
	t            *testing.T
	manager      *mediamgr.MediaManager
	service      *mediasvc.MediaService
	audit        *auditRecorder
	tenantUUIDs  map[string]string
	assetTenants map[string]string
	fixtures     map[string]string
}

func newMediaIntegrationTestEnv(t *testing.T) *mediaIntegrationTestEnv {
	t.Helper()
	repo := newMemoryAssetRepo()
	tmpDir := t.TempDir()
	localDrv, err := local.New(local.Options{
		Name:          "local",
		BasePath:      filepath.Join(tmpDir, "media"),
		PublicBaseURL: "http://localhost/media",
		EnableUpload:  true,
	})
	require.NoError(t, err)

	manager := mediamgr.New(localDrv.Name())
	manager.RegisterDriver(localDrv)
	require.NoError(t, manager.SetDefaultDriver(localDrv.Name()))

	audit := &auditRecorder{}
	service := mediasvc.NewMediaService(nil, repo, manager, audit, 12*time.Hour)

	return &mediaIntegrationTestEnv{
		t:            t,
		manager:      manager,
		service:      service,
		audit:        audit,
		tenantUUIDs:  map[string]string{},
		assetTenants: map[string]string{},
		fixtures:     map[string]string{},
	}
}

func (env *mediaIntegrationTestEnv) UploadDraftAsset(ctx context.Context, req MediaUploadRequest) (string, error) {
	tenantUUID := env.ensureTenant(req.TenantID)
	operator := parseUint(req.OperatorID)
	asset, err := env.service.CreateAsset(ctx, mediasvc.CreateAssetInput{
		TenantUUID:   tenantUUID,
		OperatorID:   operator,
		Name:         req.Name,
		Driver:       req.Driver,
		Tags:         req.Tags,
		UploadMethod: mediasvc.UploadMethod(req.UploadMethod),
		Metadata:     map[string]any{"content_sha256": mediaFixtureContentSHA256(req.TenantID, req.Name, req.UploadMethod)},
	})
	if err != nil {
		return "", err
	}
	env.assetTenants[asset.UUID] = tenantUUID
	return asset.UUID, nil
}

func mediaFixtureContentSHA256(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

func (env *mediaIntegrationTestEnv) TriggerProcessingFailure(ctx context.Context, assetID string) error {
	tenantUUID := env.assetTenants[assetID]
	if tenantUUID == "" {
		return ErrMediaAssetNotFound
	}
	return env.service.RollbackAsset(ctx, mediasvc.DeleteAssetInput{
		TenantUUID: tenantUUID,
		UUID:       assetID,
	})
}

func (env *mediaIntegrationTestEnv) LookupAsset(ctx context.Context, assetID string) (any, error) {
	tenantUUID := env.assetTenants[assetID]
	asset, err := env.service.GetAsset(ctx, tenantUUID, assetID, false)
	if err != nil {
		return nil, ErrMediaAssetNotFound
	}
	return asset, nil
}

func (env *mediaIntegrationTestEnv) CollectAuditEvents() []string {
	return env.audit.Events()
}

func (env *mediaIntegrationTestEnv) ensureTenant(key string) string {
	if key == "" {
		key = "default"
	}
	if uuid, ok := env.tenantUUIDs[key]; ok {
		return uuid
	}
	id := uuid.NewString()
	env.tenantUUIDs[key] = id
	return id
}

func (env *mediaIntegrationTestEnv) fixture(key string) string {
	return env.fixtures[key]
}

func (env *mediaIntegrationTestEnv) setFixture(key, value string) {
	if key == "" || value == "" {
		return
	}
	env.fixtures[key] = value
}

func parseUint(value string) *uint64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

type auditRecorder struct {
	events []string
}

func (a *auditRecorder) Emit(_ context.Context, evt *dbmaudit.AuditEvent) error {
	if evt != nil {
		a.events = append(a.events, evt.Operation)
	}
	return nil
}

func (a *auditRecorder) Close() {}

func (a *auditRecorder) Events() []string {
	return append([]string(nil), a.events...)
}

// memoryAssetRepo implements assetRepository for tests without external DB.
type memoryAssetRepo struct {
	mu       sync.RWMutex
	seq      uint64
	assets   map[string]*mediamodel.MediaAsset
	variants map[string]*mediamodel.MediaAssetVariant
}

func newMemoryAssetRepo() *memoryAssetRepo {
	return &memoryAssetRepo{assets: make(map[string]*mediamodel.MediaAsset), variants: make(map[string]*mediamodel.MediaAssetVariant)}
}

func (m *memoryAssetRepo) List(_ context.Context, filter mediarepo.AssetListFilter) ([]mediamodel.MediaAsset, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []mediamodel.MediaAsset
	for _, asset := range m.assets {
		if tenant := strings.TrimSpace(filter.TenantUUID); tenant != "" && asset.TenantUUID != tenant {
			continue
		}
		if filter.OnlyDeleted && !asset.DeletedAt.Valid {
			continue
		}
		if !filter.IncludeDeleted && asset.DeletedAt.Valid {
			continue
		}
		if len(filter.TagsAll) > 0 {
			tags := decodeTags(asset.Tags)
			if !containsAll(tags, filter.TagsAll) {
				continue
			}
		}
		filtered = append(filtered, *cloneAsset(asset))
	}
	total := int64(len(filtered))
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	size := filter.PageSize
	if size <= 0 {
		size = 20
	}
	start := (page - 1) * size
	if start >= len(filtered) {
		return []mediamodel.MediaAsset{}, total, nil
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *memoryAssetRepo) FindByUUID(_ context.Context, tenantUUID string, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	asset, ok := m.assets[uuid]
	if !ok || (tenantUUID != "" && asset.TenantUUID != tenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	if !includeDeleted && asset.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneAsset(asset), nil
}

func (m *memoryAssetRepo) FindByStorageKey(_ context.Context, tenantUUID string, driver, storageKey string) (*mediamodel.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, asset := range m.assets {
		if asset.Driver != driver || asset.StorageKey != storageKey {
			continue
		}
		if tenantUUID != "" && asset.TenantUUID != tenantUUID {
			continue
		}
		if asset.DeletedAt.Valid {
			continue
		}
		return cloneAsset(asset), nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *memoryAssetRepo) ListByDriverAndStorageKey(_ context.Context, driver, storageKey string) ([]mediamodel.MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matches []mediamodel.MediaAsset
	for _, asset := range m.assets {
		if asset.Driver == driver && asset.StorageKey == storageKey {
			matches = append(matches, *cloneAsset(asset))
		}
	}
	return matches, nil
}

func (m *memoryAssetRepo) FindVariant(_ context.Context, tenantUUID, assetUUID, variant string) (*mediamodel.MediaAssetVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.variants[assetUUID+"|"+variant]
	if !ok || (tenantUUID != "" && item.TenantUUID != tenantUUID) {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneVariant(item), nil
}

func (m *memoryAssetRepo) FindVariantByStorageKey(_ context.Context, driver, storageKey string) (*mediamodel.MediaAssetVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, item := range m.variants {
		if item.Driver == driver && item.StorageKey == storageKey {
			return cloneVariant(item), nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *memoryAssetRepo) CreateVariant(_ context.Context, variant *mediamodel.MediaAssetVariant) (*mediamodel.MediaAssetVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := cloneVariant(variant)
	if clone.UUID == uuid.Nil {
		clone.UUID = uuid.New()
	}
	m.variants[clone.AssetUUID+"|"+clone.Variant] = clone
	return cloneVariant(clone), nil
}

func (m *memoryAssetRepo) UpdateVariant(_ context.Context, variant *mediamodel.MediaAssetVariant) (*mediamodel.MediaAssetVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := cloneVariant(variant)
	m.variants[clone.AssetUUID+"|"+clone.Variant] = clone
	return cloneVariant(clone), nil
}

func (m *memoryAssetRepo) CreateAsset(_ context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := cloneAsset(asset)
	if clone.UUID == uuid.Nil {
		clone.UUID = uuid.New()
	}
	m.seq++
	clone.ID = m.seq
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	m.assets[clone.UUID.String()] = clone
	return cloneAsset(clone), nil
}

func (m *memoryAssetRepo) UpdateAsset(_ context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.assets[asset.UUID.String()]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	updated := cloneAsset(asset)
	updated.CreatedAt = existing.CreatedAt
	updated.ID = existing.ID
	updated.UpdatedAt = time.Now()
	m.assets[asset.UUID.String()] = updated
	return cloneAsset(updated), nil
}

func (m *memoryAssetRepo) SoftDeleteByUUID(_ context.Context, tenantUUID string, uuid string, deletedBy *uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	asset, ok := m.assets[uuid]
	if !ok || (tenantUUID != "" && asset.TenantUUID != tenantUUID) {
		return gorm.ErrRecordNotFound
	}
	now := time.Now()
	asset.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	if deletedBy != nil {
		asset.DeletedBy = deletedBy
	}
	return nil
}

func (m *memoryAssetRepo) ListVariants(_ context.Context, tenantUUID, assetUUID string, includeDeleted bool) ([]mediamodel.MediaAssetVariant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var items []mediamodel.MediaAssetVariant
	for _, item := range m.variants {
		if item.AssetUUID != assetUUID {
			continue
		}
		if tenantUUID != "" && item.TenantUUID != tenantUUID {
			continue
		}
		if !includeDeleted && item.DeletedAt.Valid {
			continue
		}
		items = append(items, *cloneVariant(item))
	}
	return items, nil
}

func (m *memoryAssetRepo) HardDeleteByUUID(_ context.Context, tenantUUID string, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	asset, ok := m.assets[uuid]
	if !ok || (tenantUUID != "" && asset.TenantUUID != tenantUUID) {
		return gorm.ErrRecordNotFound
	}
	delete(m.assets, uuid)
	for key, item := range m.variants {
		if item.AssetUUID == uuid && (tenantUUID == "" || item.TenantUUID == tenantUUID) {
			delete(m.variants, key)
		}
	}
	return nil
}

func (m *memoryAssetRepo) FindByUUIDGlobal(_ context.Context, id string, includeDeleted bool) (*mediamodel.MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	asset, ok := m.assets[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if !includeDeleted && asset.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneAsset(asset), nil
}

func cloneAsset(asset *mediamodel.MediaAsset) *mediamodel.MediaAsset {
	if asset == nil {
		return nil
	}
	cloned := *asset
	if asset.Tags != nil {
		cloned.Tags = append(datatypes.JSON(nil), asset.Tags...)
	}
	if asset.Meta != nil {
		cloned.Meta = append(datatypes.JSON(nil), asset.Meta...)
	}
	return &cloned
}

func cloneVariant(variant *mediamodel.MediaAssetVariant) *mediamodel.MediaAssetVariant {
	if variant == nil {
		return nil
	}
	cloned := *variant
	if variant.Meta != nil {
		cloned.Meta = append(datatypes.JSON(nil), variant.Meta...)
	}
	return &cloned
}

func decodeTags(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}
	var tags []string
	_ = json.Unmarshal(data, &tags)
	return tags
}

func containsAll(have []string, need []string) bool {
	if len(need) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(have))
	for _, v := range have {
		set[v] = struct{}{}
	}
	for _, v := range need {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}

func TestMediaAssetUploadFlowRollbackOnFailure(t *testing.T) {
	t.Parallel()

	env := newMediaIntegrationTestEnv(t)
	ctx := context.Background()

	request := MediaUploadRequest{
		TenantID:       "tenant_a",
		OperatorID:     "op_admin",
		Name:           "homepage-banner",
		Driver:         "local",
		UploadMethod:   "direct_upload",
		Tags:           []string{"banner", "homepage"},
		PresignTokenID: "token_123",
	}

	assetID, err := env.UploadDraftAsset(ctx, request)
	require.NoError(t, err)
	require.NotEmpty(t, assetID)

	err = env.TriggerProcessingFailure(ctx, assetID)
	require.NoError(t, err)

	_, lookupErr := env.LookupAsset(ctx, assetID)
	require.Error(t, lookupErr)
	require.ErrorIs(t, lookupErr, ErrMediaAssetNotFound)

	events := env.CollectAuditEvents()
	require.Len(t, events, 2)
	require.Equal(t, "media.asset.rollback", events[1])
}

var ErrMediaAssetNotFound = errors.New("media asset not found")
