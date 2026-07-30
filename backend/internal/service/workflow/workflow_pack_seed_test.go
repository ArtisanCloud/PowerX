package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

type packDefinitionStore struct {
	nextVersion int32
	created     []*modelworkflow.WorkflowDefinition
}

func (s *packDefinitionStore) CreateDefinition(_ context.Context, def *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error) {
	if def.UUID == uuid.Nil {
		def.PowerUUIDModel.UUID = uuid.New()
	}
	s.created = append(s.created, def)
	return def, nil
}

func (s *packDefinitionStore) NextVersion(context.Context, string, string) (int32, error) {
	if s.nextVersion <= 0 {
		s.nextVersion = 1
	}
	return s.nextVersion, nil
}

func (s *packDefinitionStore) GetByUUID(_ context.Context, _ string, definitionUUID uuid.UUID, version *int32) (*modelworkflow.WorkflowDefinition, error) {
	for _, definition := range s.created {
		if definition.UUID != definitionUUID {
			continue
		}
		if version != nil && definition.Version != *version {
			continue
		}
		return definition, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *packDefinitionStore) GetLatestPublished(context.Context, string, uuid.UUID) (*modelworkflow.WorkflowDefinition, error) {
	return nil, errors.New("not used")
}

func (s *packDefinitionStore) ListByTenant(context.Context, workflowrepo.DefinitionListFilter) ([]modelworkflow.WorkflowDefinition, int64, error) {
	return nil, 0, errors.New("not used")
}

func (s *packDefinitionStore) UpdateStatus(context.Context, string, uuid.UUID, int32, string, map[string]interface{}) error {
	return errors.New("not used")
}

type packSeedStore struct {
	latest  map[string]*modelworkflow.WorkflowPackSeedRecord
	records []modelworkflow.WorkflowPackSeedRecord
}

func newPackSeedStore() *packSeedStore {
	return &packSeedStore{latest: map[string]*modelworkflow.WorkflowPackSeedRecord{}}
}

func (s *packSeedStore) CreateRecord(_ context.Context, record *modelworkflow.WorkflowPackSeedRecord) (*modelworkflow.WorkflowPackSeedRecord, error) {
	if record.UUID == uuid.Nil {
		record.PowerUUIDModel = coremodel.PowerUUIDModel{UUID: uuid.New()}
	}
	s.records = append(s.records, *record)
	copied := *record
	s.latest[record.WorkflowKey] = &copied
	return record, nil
}

func (s *packSeedStore) GetLatestByKey(_ context.Context, _ string, workflowKey string) (*modelworkflow.WorkflowPackSeedRecord, error) {
	if record, ok := s.latest[workflowKey]; ok {
		return record, nil
	}
	return nil, gormRecordNotFound()
}

func (s *packSeedStore) ListByTenant(context.Context, string, string, int, int) ([]modelworkflow.WorkflowPackSeedRecord, int64, error) {
	return s.records, int64(len(s.records)), nil
}

func (s *packSeedStore) ListByDefinition(context.Context, uuid.UUID) ([]modelworkflow.WorkflowPackSeedRecord, error) {
	return nil, nil
}

type packInstallStore struct {
	items map[string]*modelworkflow.WorkflowPackInstallation
}

func newPackInstallStore() *packInstallStore {
	return &packInstallStore{items: map[string]*modelworkflow.WorkflowPackInstallation{}}
}

func (s *packInstallStore) key(tenantUUID string, workflowKey string) string {
	return tenantUUID + "\x00" + workflowKey
}

func (s *packInstallStore) GetByTenantKey(_ context.Context, tenantUUID string, workflowKey string) (*modelworkflow.WorkflowPackInstallation, error) {
	if item, ok := s.items[s.key(tenantUUID, workflowKey)]; ok {
		copied := *item
		return &copied, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *packInstallStore) UpsertEnabled(_ context.Context, installation *modelworkflow.WorkflowPackInstallation) (*modelworkflow.WorkflowPackInstallation, error) {
	if installation.UUID == uuid.Nil {
		installation.PowerUUIDModel = coremodel.PowerUUIDModel{UUID: uuid.New()}
	}
	installation.Status = modelworkflow.WorkflowPackInstallationStatusEnabled
	copied := *installation
	s.items[s.key(installation.TenantUUID, installation.WorkflowKey)] = &copied
	return installation, nil
}

func (s *packInstallStore) MarkDeleted(_ context.Context, tenantUUID string, workflowKey string, actorUUID uuid.UUID) error {
	item, ok := s.items[s.key(tenantUUID, workflowKey)]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	now := testClock()
	item.Status = modelworkflow.WorkflowPackInstallationStatusDeleted
	item.RemovedAt = &now
	item.RemovedBy = actorUUID
	item.LastAction = "delete"
	return nil
}

func (s *packInstallStore) ListByTenant(_ context.Context, tenantUUID string, _ string, _ int, _ int) ([]modelworkflow.WorkflowPackInstallation, int64, error) {
	var items []modelworkflow.WorkflowPackInstallation
	for _, item := range s.items {
		if item.TenantUUID == tenantUUID {
			items = append(items, *item)
		}
	}
	return items, int64(len(items)), nil
}

func TestLoadWorkflowPacksReadsBuiltinConfig(t *testing.T) {
	packs, err := LoadWorkflowPacks("../../../config/workflow_packs")
	if err != nil {
		t.Fatalf("load workflow packs: %v", err)
	}
	if len(packs) != 6 {
		t.Fatalf("expected 6 packs, got %d", len(packs))
	}
	if packs[0].WorkflowKey != "approval_guarded_capability" {
		t.Fatalf("packs should be sorted by key, got %s", packs[0].WorkflowKey)
	}
	keys := map[string]bool{}
	for _, pack := range packs {
		keys[pack.WorkflowKey] = true
	}
	for _, key := range []string{
		"approval_guarded_capability",
		"campaign_review_to_methodology",
		"expert_knowledge_capture",
		"intake_classify_review",
		"marketing_knowledge_capture",
		"skill_review_publish_event",
	} {
		if !keys[key] {
			t.Fatalf("expected workflow pack %s", key)
		}
	}
}

func TestSeedWorkflowPacksCreatesPublishedDefinitionAndSkipsUnchangedChecksum(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := RegisterWorkflowNodeAdapters(registry, WorkflowNodeAdapterDeps{}); err != nil {
		t.Fatalf("register adapters: %v", err)
	}
	definitions := &packDefinitionStore{}
	packSeeds := newPackSeedStore()
	packInstalls := newPackInstallStore()
	service := &Service{
		definitions:  definitions,
		packSeeds:    packSeeds,
		packInstalls: packInstalls,
		adapters:     registry,
		now:          testClock,
	}

	result, err := service.SeedWorkflowPacks(context.Background(), WorkflowPackSeedInput{
		TenantUUID: "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		ConfigDir:  "../../../config/workflow_packs",
		Keys:       []string{"marketing_knowledge_capture"},
	})
	if err != nil {
		t.Fatalf("seed workflow packs: %v", err)
	}
	if len(result.Seeded) != 1 || len(definitions.created) != 1 {
		t.Fatalf("expected one seeded definition, result=%#v definitions=%d", result, len(definitions.created))
	}
	if definitions.created[0].Status != "published" || definitions.created[0].WorkflowPackKey != "marketing_knowledge_capture" {
		t.Fatalf("unexpected definition: %#v", definitions.created[0])
	}
	var metadata map[string]any
	if err := json.Unmarshal(definitions.created[0].Metadata, &metadata); err != nil {
		t.Fatalf("unmarshal definition metadata: %v", err)
	}
	if metadata["category"] != "knowledge_curation" || metadata["workflow_pack_key"] != "marketing_knowledge_capture" {
		t.Fatalf("unexpected workflow pack metadata: %#v", metadata)
	}

	result, err = service.SeedWorkflowPacks(context.Background(), WorkflowPackSeedInput{
		TenantUUID: "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		ConfigDir:  "../../../config/workflow_packs",
		Keys:       []string{"marketing_knowledge_capture"},
	})
	if err != nil {
		t.Fatalf("seed unchanged workflow packs: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "marketing_knowledge_capture" {
		t.Fatalf("expected unchanged pack skipped, got %#v", result)
	}
}

func TestSeedWorkflowPacksSkipsDeletedInstallation(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := RegisterWorkflowNodeAdapters(registry, WorkflowNodeAdapterDeps{}); err != nil {
		t.Fatalf("register adapters: %v", err)
	}
	definitions := &packDefinitionStore{}
	packInstalls := newPackInstallStore()
	packInstalls.items[packInstalls.key("9ddc11dc-a205-49df-851a-3ec47a58d9d4", "marketing_knowledge_capture")] = &modelworkflow.WorkflowPackInstallation{
		TenantUUID:  "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		WorkflowKey: "marketing_knowledge_capture",
		Version:     1,
		Checksum:    "old",
		Status:      modelworkflow.WorkflowPackInstallationStatusDeleted,
		Source:      "builtin",
		LastAction:  "delete",
	}
	service := &Service{
		definitions:  definitions,
		packSeeds:    newPackSeedStore(),
		packInstalls: packInstalls,
		adapters:     registry,
		now:          testClock,
	}

	result, err := service.SeedWorkflowPacks(context.Background(), WorkflowPackSeedInput{
		TenantUUID: "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		ConfigDir:  "../../../config/workflow_packs",
		Keys:       []string{"marketing_knowledge_capture"},
	})
	if err != nil {
		t.Fatalf("seed deleted workflow pack: %v", err)
	}
	if len(result.Seeded) != 0 || len(definitions.created) != 0 {
		t.Fatalf("deleted installation must not seed definitions, result=%#v definitions=%d", result, len(definitions.created))
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "marketing_knowledge_capture" {
		t.Fatalf("expected deleted pack skipped, got %#v", result)
	}
}

func TestSeedWorkflowPacksUpdatesEnabledInstallationWhenChecksumChanges(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := RegisterWorkflowNodeAdapters(registry, WorkflowNodeAdapterDeps{}); err != nil {
		t.Fatalf("register adapters: %v", err)
	}
	definitions := &packDefinitionStore{nextVersion: 2}
	packSeeds := newPackSeedStore()
	packSeeds.latest["marketing_knowledge_capture"] = &modelworkflow.WorkflowPackSeedRecord{
		TenantUUID:        "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		WorkflowKey:       "marketing_knowledge_capture",
		Version:           1,
		DefinitionUUID:    uuid.New(),
		DefinitionVersion: 1,
		Checksum:          "old",
		Source:            "builtin",
		SeededAt:          testClock(),
	}
	packInstalls := newPackInstallStore()
	packInstalls.items[packInstalls.key("9ddc11dc-a205-49df-851a-3ec47a58d9d4", "marketing_knowledge_capture")] = &modelworkflow.WorkflowPackInstallation{
		TenantUUID:        "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		WorkflowKey:       "marketing_knowledge_capture",
		Version:           1,
		Checksum:          "old",
		Status:            modelworkflow.WorkflowPackInstallationStatusEnabled,
		DefinitionUUID:    uuid.New(),
		DefinitionVersion: 1,
		Source:            "builtin",
		LastAction:        "install",
	}
	service := &Service{
		definitions:  definitions,
		packSeeds:    packSeeds,
		packInstalls: packInstalls,
		adapters:     registry,
		now:          testClock,
	}

	result, err := service.SeedWorkflowPacks(context.Background(), WorkflowPackSeedInput{
		TenantUUID: "9ddc11dc-a205-49df-851a-3ec47a58d9d4",
		ConfigDir:  "../../../config/workflow_packs",
		Keys:       []string{"marketing_knowledge_capture"},
	})
	if err != nil {
		t.Fatalf("seed changed workflow pack: %v", err)
	}
	if len(result.Seeded) != 1 || len(definitions.created) != 1 {
		t.Fatalf("expected changed pack seeded once, result=%#v definitions=%d", result, len(definitions.created))
	}
	installation := packInstalls.items[packInstalls.key("9ddc11dc-a205-49df-851a-3ec47a58d9d4", "marketing_knowledge_capture")]
	if installation.Checksum == "old" || installation.DefinitionVersion != 2 {
		t.Fatalf("expected installation updated to new definition, got %#v", installation)
	}
}

func testClock() time.Time {
	return time.Unix(3000, 0).UTC()
}

func gormRecordNotFound() error {
	return gorm.ErrRecordNotFound
}
