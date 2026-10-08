package knowledge_space

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
)

type semanticChunkMetadata struct {
	ArtifactUUID            string               `json:"artifact_uuid"`
	ArtifactRole            string               `json:"artifact_role"`
	KnowledgeProfileUUID    *string              `json:"knowledge_profile_uuid,omitempty"`
	CaseUUID                *string              `json:"case_uuid,omitempty"`
	CategoryCodes           []string             `json:"category_codes"`
	TagUUIDs                []string             `json:"tag_uuids"`
	ExternalRef             *SemanticExternalRef `json:"external_ref,omitempty"`
	SourceRef               SemanticSourceRef    `json:"source_ref"`
	DocumentVersion         string               `json:"document_version"`
	ConfigurationGeneration string               `json:"configuration_generation"`
	Mode                    string               `json:"mode"`
}

func (s *SemanticRuntime) build(ctx context.Context, job models.IndexJob, document string, source hostDocumentSource, config HostIngestionSnapshot, ordinal int) ([]models.HostDocumentChunk, error) {
	if validateSemanticArtifacts(HostDocumentInput{Indexing: &config.Indexing.SemanticIndexingSettings, Artifacts: source.Artifacts, ExternalRef: source.ExternalRef}) != nil {
		return nil, semanticError(422, KnowledgeReasonSnapshotInvalid)
	}
	embed, err := s.validateFrozen(ctx, job.TenantUUID, job.SpaceUUID, config.Indexing)
	if err != nil {
		return nil, err
	}
	rows := []models.HostDocumentChunk{}
	for _, artifact := range source.Artifacts {
		artSource := source
		artSource.Content, artSource.Checksum, artSource.Version = artifact.Text, artifact.Checksum, artifact.Version
		artConfig := config
		artConfig.SourceChecksum, artConfig.SourceVersion = artifact.Checksum, artifact.Version
		built, err := buildHostDocumentChunks(job, document, artSource, artConfig, ordinal+len(rows))
		if err != nil {
			return nil, err
		}
		cursor := 0
		for i := range built {
			text := built[i].Content
			start := strings.Index(artifact.Text[cursor:], text)
			if start < 0 {
				return nil, semanticError(422, "KNOWLEDGE_SOURCE_POSITION_UNAVAILABLE")
			}
			start += cursor
			position := artifact.SourceRef
			position.CharStart = utf8.RuneCountInString(artifact.Text[:start])
			position.CharEnd = position.CharStart + utf8.RuneCountInString(text)
			meta := semanticChunkMetadata{ArtifactUUID: artifact.UUID, ArtifactRole: artifact.Role, KnowledgeProfileUUID: artifact.KnowledgeProfileUUID, CaseUUID: artifact.CaseUUID, CategoryCodes: artifact.CategoryCodes, TagUUIDs: artifact.TagUUIDs, ExternalRef: source.ExternalRef, SourceRef: position, DocumentVersion: source.Version, ConfigurationGeneration: config.Indexing.ConfigurationGeneration, Mode: config.Indexing.Mode}
			var root map[string]any
			_ = json.Unmarshal(built[i].Metadata, &root)
			root["semantic"] = meta
			root["tenant_uuid"] = job.TenantUUID
			root["document_uuid"] = document
			built[i].UUID = uuid.NewSHA1(job.UUID, []byte(document+":"+artifact.UUID+":"+built[i].UUID.String()))
			built[i].Metadata, _ = json.Marshal(root)
			cursor = start + len(text)
			for n := 0; n < config.ChunkOverlap && cursor > start; n++ {
				_, size := utf8.DecodeLastRuneInString(artifact.Text[:cursor])
				cursor -= size
			}
		}
		rows = append(rows, built...)
	}
	if len(rows) > 4096 {
		return nil, semanticError(400, "KNOWLEDGE_ARTIFACT_LIMIT_EXCEEDED")
	}
	records := []vectorstore.VectorRecord{}
	for begin := 0; begin < len(rows); begin += 16 {
		end := begin + 16
		if end > len(rows) {
			end = len(rows)
		}
		texts := []string{}
		for _, row := range rows[begin:end] {
			texts = append(texts, row.Content)
		}
		vectors, err := embed.Embed(ctx, texts)
		if err != nil || len(vectors) != len(texts) {
			return nil, semanticError(503, "KNOWLEDGE_EMBEDDING_FAILED")
		}
		for i, v := range vectors {
			if len(v) != config.Indexing.Dimensions {
				return nil, semanticError(409, "KNOWLEDGE_MODEL_DIMENSION_CONFLICT")
			}
			norm := float64(0)
			for _, x := range v {
				if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
					return nil, semanticError(503, "KNOWLEDGE_EMBEDDING_INVALID")
				}
				norm += float64(x) * float64(x)
			}
			if norm == 0 {
				return nil, semanticError(503, "KNOWLEDGE_EMBEDDING_INVALID")
			}
			var meta map[string]any
			_ = json.Unmarshal(rows[begin+i].Metadata, &meta)
			records = append(records, vectorstore.VectorRecord{ChunkID: rows[begin+i].UUID, Embedding: v, Metadata: meta})
		}
	}
	if s.vectorWriter == nil || s.vectorWriter.Upsert(ctx, uuid.MustParse(job.SpaceUUID), records) != nil {
		return nil, semanticError(503, "KNOWLEDGE_VECTOR_WRITE_FAILED")
	}
	return rows, nil
}
