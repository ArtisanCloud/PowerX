package knowledge_space

import (
	"context"
	"errors"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
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
	ready := err == nil && s.vectors != nil
	if ready {
		err = s.checkVectorDatabase(ctx)
		ready = err == nil
	}
	if ready {
		index, e := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, row.UUID, binding.VectorIndexKey)
		if e != nil || index == nil {
			ready = false
		} else {
			store, e := s.vectors.storeForIndexRecord(index.VectorTable, index.Dimensions)
			ready = e == nil && store != nil && store.Health(ctx) == nil
		}
	}
	var counts []struct {
		Mode      string `json:"mode"`
		Documents int64  `json:"documents"`
		Chunks    int64  `json:"chunks"`
	}
	query := "SELECT c.metadata->'semantic'->>'mode' AS mode, COUNT(DISTINCT d.uuid) AS documents, COUNT(c.uuid) AS chunks FROM " + models.HostDocumentChunk{}.TableName() + " c JOIN " + models.TenantDocument{}.TableName() + " d ON d.uuid=c.document_uuid AND d.tenant_uuid=c.tenant_uuid AND d.active_index_job_uuid=c.job_uuid JOIN " + models.IndexJob{}.TableName() + " j ON j.uuid=c.job_uuid AND j.tenant_uuid=c.tenant_uuid AND j.status='succeeded' WHERE c.tenant_uuid=? AND c.space_uuid=? AND d.deleted_at IS NULL AND c.deleted_at IS NULL AND d.queryable=TRUE AND d.index_status='indexed' AND c.metadata->'semantic'->>'configuration_generation'=? GROUP BY c.metadata->'semantic'->>'mode'"
	if e := s.db.WithContext(ctx).Raw(query, tenant, space, binding.ConfigurationGeneration).Scan(&counts).Error; e != nil {
		return nil, e
	}
	count := int64(0)
	hybridCount := int64(0)
	for _, c := range counts {
		count += c.Documents
		if c.Mode == HybridMode {
			hybridCount += c.Documents
		}
	}
	indexedModes := []string{}
	if ready && count > 0 {
		indexedModes = append(indexedModes, SemanticMode)
	}
	if ready && hybridCount > 0 {
		indexedModes = append(indexedModes, HybridMode)
	}
	code := ""
	modes := []string{SemanticMode, HybridMode}
	if !ready {
		code = "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY"
		if known := dto.CodeOf(err); known != "" {
			code = known
		}
		modes = []string{}
	}
	var pendingGeneration *SemanticGeneration
	if pending, ok := semanticPendingBinding(binding); ok {
		g, e := s.generation(ctx, pending)
		if e != nil {
			return nil, e
		}
		pendingGeneration = &g
	}
	return map[string]any{"pending_generation": pendingGeneration, "schema": "powerx.knowledge.semantic-capabilities/v1", "configured": true, "ready": ready, "indexed": ready && count > 0, "query_modes": modes, "indexed_query_modes": indexedModes, "index_counts": counts, "index_modes": modes, "artifact_roles": []string{"knowledge_profile", "source_chunk"}, "supported_filters": []string{"document_uuids", "category_codes", "tag_uuids", "artifact_roles"}, "tag_match": "any", "position_unit": "unicode_codepoint", "strict_sources": true, "generation": generation, "reason_code": code, "limits": map[string]int{"max_query_codepoints": 8192, "max_top_k": 100, "max_spaces": 16, "max_artifacts": 64}}, nil
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
		if doc.IndexStatus == HostDocumentStatusDeleted {
			return KnowledgeDocumentNotFoundError(errors.New("document deleted"))
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
