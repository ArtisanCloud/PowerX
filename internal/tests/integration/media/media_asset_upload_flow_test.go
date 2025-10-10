package media

import (
	"context"
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
	tenantIDs    map[string]uint64
	assetTenants map[string]uint64
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
		tenantIDs:    map[string]uint64{},
		assetTenants: map[string]uint64{},
		fixtures:     map[string]string{},
	}
}

func (env *mediaIntegrationTestEnv) UploadDraftAsset(ctx context.Context, req MediaUploadRequest) (string, error) {
	tenantID := env.ensureTenant(req.TenantID)
	operator := parseUint(req.OperatorID)
	asset, err := env.service.CreateAsset(ctx, mediasvc.CreateAssetInput{
		TenantID:     tenantID,
		OperatorID:   operator,
		Name:         req.Name,
		Driver:       req.Driver,
		Tags:         req.Tags,
		UploadMethod: mediasvc.UploadMethod(req.UploadMethod),
	})
	if err != nil {
		return "", err
	}
	env.assetTenants[asset.UUID] = tenantID
	return asset.UUID, nil
}

func (env *mediaIntegrationTestEnv) TriggerProcessingFailure(ctx context.Context, assetID string) error {
	tenantID := env.assetTenants[assetID]
	if tenantID == 0 {
		return ErrMediaAssetNotFound
	}
	return env.service.RollbackAsset(ctx, mediasvc.DeleteAssetInput{
		TenantID: tenantID,
		UUID:     assetID,
	})
}

func (env *mediaIntegrationTestEnv) LookupAsset(ctx context.Context, assetID string) (any, error) {
	tenantID := env.assetTenants[assetID]
	asset, err := env.service.GetAsset(ctx, tenantID, assetID, false)
	if err != nil {
		return nil, ErrMediaAssetNotFound
	}
	return asset, nil
}

func (env *mediaIntegrationTestEnv) CollectAuditEvents() []string {
	return env.audit.Events()
}

func (env *mediaIntegrationTestEnv) ensureTenant(key string) uint64 {
	if key == "" {
		key = "default"
	}
	if id, ok := env.tenantIDs[key]; ok {
		return id
	}
	id := uint64(len(env.tenantIDs) + 1)
	env.tenantIDs[key] = id
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
	mu     sync.RWMutex
	seq    uint64
	assets map[string]*mediamodel.MediaAsset
}

func newMemoryAssetRepo() *memoryAssetRepo {
	return &memoryAssetRepo{assets: make(map[string]*mediamodel.MediaAsset)}
}

func (m *memoryAssetRepo) List(_ context.Context, filter mediarepo.AssetListFilter) ([]mediamodel.MediaAsset, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []mediamodel.MediaAsset
	for _, asset := range m.assets {
		if filter.TenantID > 0 && asset.TenantID != filter.TenantID {
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

func (m *memoryAssetRepo) FindByUUID(_ context.Context, tenantID uint64, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	asset, ok := m.assets[uuid]
	if !ok || (tenantID > 0 && asset.TenantID != tenantID) {
		return nil, gorm.ErrRecordNotFound
	}
	if !includeDeleted && asset.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneAsset(asset), nil
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

func (m *memoryAssetRepo) SoftDeleteByUUID(_ context.Context, tenantID uint64, uuid string, deletedBy *uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	asset, ok := m.assets[uuid]
	if !ok || (tenantID > 0 && asset.TenantID != tenantID) {
		return gorm.ErrRecordNotFound
	}
	now := time.Now()
	asset.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	if deletedBy != nil {
		asset.DeletedBy = deletedBy
	}
	return nil
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
