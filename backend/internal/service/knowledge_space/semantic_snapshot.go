package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
)

func validSemanticUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil
}

func validateSemanticArtifacts(input HostDocumentInput) error {
	if input.Indexing == nil {
		if len(input.Artifacts) > 0 || input.ExternalRef != nil {
			return KnowledgeInvalidArgumentError(errors.New("indexing is required for semantic artifacts"))
		}
		return nil
	}
	if input.Indexing.Mode != SemanticMode && input.Indexing.Mode != HybridMode {
		return semanticError(400, "KNOWLEDGE_INDEX_MODE_INVALID")
	}
	if len(input.Indexing.ArtifactRoles) == 0 || len(input.Indexing.ArtifactRoles) > 2 || len(input.Artifacts) == 0 || len(input.Artifacts) > 64 {
		return semanticError(400, "KNOWLEDGE_ARTIFACT_SET_INVALID")
	}
	roles := map[string]bool{}
	for _, role := range input.Indexing.ArtifactRoles {
		if (role != "source_chunk" && role != "knowledge_profile") || roles[role] {
			return semanticError(400, "KNOWLEDGE_ARTIFACT_SET_INVALID")
		}
		roles[role] = true
	}
	if input.ExternalRef != nil && !validSemanticUUID(input.ExternalRef.DocumentUUID) {
		return semanticError(400, "KNOWLEDGE_EXTERNAL_REF_INVALID")
	}
	seen, found := map[string]bool{}, map[string]bool{}
	total := 0
	for _, a := range input.Artifacts {
		if !validSemanticUUID(a.UUID) || seen[a.UUID] || !roles[a.Role] || strings.TrimSpace(a.Text) == "" || a.Version == "" || checksumBytes([]byte(a.Text)) != a.Checksum {
			return semanticError(400, "KNOWLEDGE_ARTIFACT_INVALID")
		}
		seen[a.UUID], found[a.Role] = true, true
		total += len(a.Text)
		if total > 8<<20 || len(a.CategoryCodes) > 32 || len(a.TagUUIDs) > 64 {
			return semanticError(400, "KNOWLEDGE_ARTIFACT_LIMIT_EXCEEDED")
		}
		for _, id := range a.TagUUIDs {
			if !validSemanticUUID(id) {
				return semanticError(400, "KNOWLEDGE_FILTER_UUID_INVALID")
			}
		}
		for _, id := range []*string{a.KnowledgeProfileUUID, a.CaseUUID} {
			if id != nil && !validSemanticUUID(*id) {
				return semanticError(400, "KNOWLEDGE_ARTIFACT_REF_INVALID")
			}
		}
		r := a.SourceRef
		if !validSemanticUUID(r.SourceUUID) || r.Version != a.Version || r.Checksum != a.Checksum || r.PositionUnit != "unicode_codepoint" || r.CharStart != 0 || r.CharEnd != utf8.RuneCountInString(a.Text) {
			return semanticError(400, "KNOWLEDGE_SOURCE_POSITION_INVALID")
		}
		if a.Role == "knowledge_profile" && (r.Kind != "knowledge_profile" || a.KnowledgeProfileUUID == nil || *a.KnowledgeProfileUUID != r.SourceUUID) {
			return semanticError(400, "KNOWLEDGE_SOURCE_KIND_INVALID")
		}
		if a.Role == "source_chunk" && r.Kind != "raw_document" && r.Kind != "cleaned_document" {
			return semanticError(400, "KNOWLEDGE_SOURCE_KIND_INVALID")
		}
	}
	for role := range roles {
		if !found[role] {
			return semanticError(400, "KNOWLEDGE_ARTIFACT_SET_INCOMPLETE")
		}
	}
	return nil
}

func (s *SemanticRuntime) freeze(ctx context.Context, tenant, space string, input SemanticIndexingSettings) (*SemanticIndexSnapshot, error) {
	var binding models.SemanticSpaceBinding
	if s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).First(&binding).Error != nil {
		return nil, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
	}
	var profile models.SemanticEmbeddingProfile
	if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, binding.EmbeddingProfileUUID, models.ProfileStatusPublished).First(&profile).Error != nil {
		return nil, semanticError(422, "KNOWLEDGE_EMBEDDING_PROFILE_UNAVAILABLE")
	}
	if input.EmbeddingProfile.UUID != profile.UUID.String() || input.EmbeddingProfile.Version != profile.Version {
		return nil, semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
	}
	return &SemanticIndexSnapshot{SemanticIndexingSettings: input, ConfigurationGeneration: binding.ConfigurationGeneration, ModelKey: profile.ProfileKey, ModelRevision: profile.ModelRevision, ConfigChecksum: profile.ConfigChecksum, Dimensions: profile.Dimensions, VectorIndexKey: binding.VectorIndexKey}, nil
}

func (s *SemanticRuntime) validateFrozen(ctx context.Context, tenant, space string, snapshot *SemanticIndexSnapshot) (agentSvcEmbedVectorizer, error) {
	if snapshot == nil {
		return nil, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
	}
	current, err := s.freeze(ctx, tenant, space, snapshot.SemanticIndexingSettings)
	if err != nil {
		return nil, err
	}
	currentBytes, _ := json.Marshal(current)
	frozenBytes, _ := json.Marshal(snapshot)
	if checksumJSON(currentBytes) != checksumJSON(frozenBytes) {
		return nil, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
	}
	resolved, embed, revision, hash, err := s.resolveModel(ctx, tenant, snapshot.ModelKey)
	if err != nil {
		return nil, err
	}
	if revision != snapshot.ModelRevision || hash != snapshot.ConfigChecksum || (resolved.Dimensions > 0 && resolved.Dimensions != snapshot.Dimensions) {
		return nil, semanticError(409, "KNOWLEDGE_MODEL_REVISION_CONFLICT")
	}
	return embed, nil
}
