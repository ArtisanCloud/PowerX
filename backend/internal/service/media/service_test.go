package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	mediarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
)

const mediaTenantUUID = "8a21845e-d1b6-4df1-b2ce-1d3bde3b8a03"

type stubAssetRepo struct {
	mu          sync.Mutex
	assets      map[string]*mediamodel.MediaAsset
	createCalls int
	updateCalls int
	deleteCalls int
	listFilter  mediarepo.AssetListFilter
}

func newStubAssetRepo() *stubAssetRepo {
	return &stubAssetRepo{assets: make(map[string]*mediamodel.MediaAsset)}
}

func (s *stubAssetRepo) List(_ context.Context, filter mediarepo.AssetListFilter) ([]mediamodel.MediaAsset, int64, error) {
	s.mu.Lock()
	s.listFilter = filter
	defer s.mu.Unlock()
	items := make([]mediamodel.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		items = append(items, *cloneAsset(asset))
	}
	return items, int64(len(items)), nil
}

func (s *stubAssetRepo) FindByUUID(_ context.Context, tenantUUID string, id string, includeDeleted bool) (*mediamodel.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if tenantUUID != "" && asset.TenantUUID != tenantUUID {
		return nil, gorm.ErrRecordNotFound
	}
	if !includeDeleted && asset.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneAsset(asset), nil
}

func (s *stubAssetRepo) CreateAsset(_ context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	clone := cloneAsset(asset)
	if clone.UUID == uuid.Nil {
		clone.UUID = uuid.New()
	}
	s.assets[clone.UUID.String()] = clone
	return cloneAsset(clone), nil
}

func (s *stubAssetRepo) UpdateAsset(_ context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls++
	clone := cloneAsset(asset)
	s.assets[clone.UUID.String()] = clone
	return cloneAsset(clone), nil
}

func (s *stubAssetRepo) SoftDeleteByUUID(_ context.Context, tenantUUID string, id string, _ *uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls++
	asset, ok := s.assets[id]
	if !ok || (tenantUUID != "" && asset.TenantUUID != tenantUUID) {
		return gorm.ErrRecordNotFound
	}
	asset.DeletedAt = gorm.DeletedAt{Valid: true, Time: time.Now()}
	return nil
}

type stubAuditService struct {
	mu     sync.Mutex
	events []*dbmaudit.AuditEvent
}

func (s *stubAuditService) Emit(_ context.Context, evt *dbmaudit.AuditEvent) error {
	if evt == nil {
		return nil
	}
	s.mu.Lock()
	s.events = append(s.events, evt)
	s.mu.Unlock()
	return nil
}

func (s *stubAuditService) Close() {}

func (s *stubAuditService) Operations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ops := make([]string, 0, len(s.events))
	for _, evt := range s.events {
		ops = append(ops, evt.Operation)
	}
	return ops
}

func TestUpdateAsset_InvalidTransition(t *testing.T) {
	repo := newStubAssetRepo()
	audit := &stubAuditService{}
	assetID := uuid.New().String()
	repo.assets[assetID] = &mediamodel.MediaAsset{
		TenantUUID:     mediaTenantUUID,
		BusinessStatus: coremodel.MediaAssetStatusDraft,
		Tags:           datatypes.JSON([]byte("[]")),
	}
	// 设置 UUID（在结构体字面量中不能直接设置嵌入字段）
	repo.assets[assetID].UUID = uuid.MustParse(assetID)

	svc := NewMediaService(nil, repo, nil, audit, 12*time.Hour)
	target := coremodel.MediaAssetStatusPublished
	_, err := svc.UpdateAsset(context.Background(), UpdateAssetInput{TenantUUID: mediaTenantUUID, UUID: assetID, BusinessStatus: &target})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidStatusTransition))
	assert.Equal(t, 0, repo.updateCalls)
}

func TestCreateAsset_TenantRequired(t *testing.T) {
	repo := newStubAssetRepo()
	audit := &stubAuditService{}
	svc := NewMediaService(nil, repo, nil, audit, 12*time.Hour)

	_, err := svc.CreateAsset(context.Background(), CreateAssetInput{TenantUUID: "", Name: "demo", Driver: "local"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant uuid")
	assert.Equal(t, 0, repo.createCalls)
}

func TestDeleteAsset_EmitAudit(t *testing.T) {
	repo := newStubAssetRepo()
	assetID := uuid.New().String()
	repo.assets[assetID] = &mediamodel.MediaAsset{
		TenantUUID: mediaTenantUUID,
	}
	// 设置 UUID（在结构体字面量中不能直接设置嵌入字段）
	repo.assets[assetID].UUID = uuid.MustParse(assetID)
	audit := &stubAuditService{}
	svc := NewMediaService(nil, repo, nil, audit, 12*time.Hour)

	require.NoError(t, svc.DeleteAsset(context.Background(), DeleteAssetInput{TenantUUID: mediaTenantUUID, UUID: assetID}))
	assert.Equal(t, 1, repo.deleteCalls)
	ops := audit.Operations()
	require.Len(t, ops, 1)
	assert.Equal(t, "media.asset.delete", ops[0])
}

func cloneAsset(src *mediamodel.MediaAsset) *mediamodel.MediaAsset {
	if src == nil {
		return nil
	}
	clone := *src
	clone.Meta = append(datatypes.JSON(nil), src.Meta...)
	clone.Tags = append(datatypes.JSON(nil), src.Tags...)
	return &clone
}

var _ assetRepository = (*stubAssetRepo)(nil)
var _ auditsvc.Service = (*stubAuditService)(nil)
