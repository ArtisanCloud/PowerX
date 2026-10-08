package knowledge_space

import (
	"context"
	"errors"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *SemanticRuntime) Capabilities(ctx context.Context, tenant, space string) (map[string]any, error) {
	var row models.KnowledgeSpace
	if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status <> ?", tenant, space, models.KnowledgeSpaceStatusRetired).First(&row).Error != nil {
		return nil, KnowledgeSpaceNotFoundError(errors.New("space unavailable"))
	}
	var binding models.SemanticSpaceBinding
	if s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).First(&binding).Error != nil {
		return map[string]any{"schema": "powerx.knowledge.semantic-capabilities/v1", "configured": false, "ready": false, "indexed": false, "query_modes": []string{}, "reason_code": "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY"}, nil
	}
	generation, err := s.generation(ctx, binding)
	if err != nil {
		return nil, err
	}
	frozen, e := s.freeze(ctx, tenant, space, SemanticIndexingSettings{Mode: SemanticMode, EmbeddingProfile: generation.EmbeddingProfile, ArtifactRoles: []string{"source_chunk"}})
	if e != nil {
		return nil, e
	}
	_, err = s.validateFrozen(ctx, tenant, space, frozen)
	ready := err == nil && row.ActiveVectorIndexKey == binding.VectorIndexKey
	if ready {
		store, _, e := s.vectors.resolveActiveStore(ctx, row.UUID)
		ready = e == nil && store != nil && store.Health(ctx) == nil
	}
	var count int64
	if e := s.db.WithContext(ctx).Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND space_uuid = ? AND active_index_job_uuid IS NOT NULL AND queryable = ? AND index_status = ?", tenant, space, true, HostDocumentStatusIndexed).Count(&count).Error; e != nil {
		return nil, e
	}
	code := ""
	modes := []string{SemanticMode, HybridMode}
	if !ready {
		code = "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY"
		modes = []string{}
	}
	return map[string]any{"schema": "powerx.knowledge.semantic-capabilities/v1", "configured": true, "ready": ready, "indexed": ready && count > 0, "query_modes": modes, "index_modes": modes, "artifact_roles": []string{"knowledge_profile", "source_chunk"}, "supported_filters": []string{"document_uuids", "category_codes", "tag_uuids", "artifact_roles"}, "tag_match": "any", "position_unit": "unicode_codepoint", "strict_sources": true, "generation": generation, "reason_code": code, "limits": map[string]int{"max_query_codepoints": 8192, "max_top_k": 100, "max_spaces": 16, "max_artifacts": 64}}, nil
}

type SemanticVisibilityInput struct {
	Queryable     bool   `json:"queryable"`
	ExpectedEpoch string `json:"expected_epoch,omitempty"`
}

func (s *HostContractService) SetDocumentVisibility(ctx context.Context, tenant, space, document string, in SemanticVisibilityInput) (map[string]any, error) {
	if !validSemanticUUID(document) {
		return nil, KnowledgeInvalidArgumentError(errors.New("document UUID invalid"))
	}
	epoch := uuid.NewString()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockHostSpace(ctx, tx, tenant, space); err != nil {
			return err
		}
		var doc models.TenantDocument
		if tx.Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", tenant, space, document).First(&doc).Error != nil {
			return KnowledgeDocumentNotFoundError(errors.New("document unavailable"))
		}
		if in.ExpectedEpoch != "" && doc.VisibilityEpoch != in.ExpectedEpoch {
			return semanticError(409, "KNOWLEDGE_VISIBILITY_CONFLICT")
		}
		if err := tx.Model(&doc).Updates(map[string]any{"queryable": in.Queryable, "visibility_epoch": epoch}).Error; err != nil {
			return err
		}
		return tx.Model(&models.SemanticSpaceBinding{}).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).Update("corpus_generation", uuid.NewString()).Error
	})
	return map[string]any{"document_uuid": document, "queryable": in.Queryable, "visibility_epoch": epoch}, err
}
