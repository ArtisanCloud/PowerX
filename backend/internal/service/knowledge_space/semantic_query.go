package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

var semanticIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func quoteSemanticIdentifier(value string) (string, error) {
	if !semanticIdentifier.MatchString(value) {
		return "", semanticError(422, "KNOWLEDGE_VECTOR_LAYOUT_INVALID")
	}
	return `"` + value + `"`, nil
}

type semanticCandidate struct {
	ChunkUUID    string
	DocumentUUID string
	Content      string
	Metadata     []byte
	Distance     float64
	LexicalScore float64
}

func validateSemanticQuery(in SemanticQuery) error {
	if in.Schema != SemanticQuerySchema || strings.TrimSpace(in.Query) == "" || utf8.RuneCountInString(in.Query) > 8192 || in.TopK < 1 || in.TopK > 100 || len(in.SpaceUUIDs) == 0 || len(in.SpaceUUIDs) > 16 || (in.Mode != SemanticMode && in.Mode != HybridMode) {
		return semanticError(400, "KNOWLEDGE_RETRIEVAL_INVALID_ARGUMENT")
	}
	if _, err := parseUniqueUUIDs(in.SpaceUUIDs); err != nil {
		return KnowledgeInvalidArgumentError(err)
	}
	for _, values := range [][]string{in.Filters.DocumentUUIDs, in.Filters.TagUUIDs} {
		if len(values) > 100 {
			return semanticError(400, "KNOWLEDGE_FILTER_LIMIT_EXCEEDED")
		}
		if _, err := parseUniqueUUIDs(values); err != nil {
			return KnowledgeInvalidArgumentError(err)
		}
	}
	if len(in.Filters.ArtifactRoles) > 100 || len(in.Filters.CategoryCodes) > 100 {
		return semanticError(400, "KNOWLEDGE_FILTER_LIMIT_EXCEEDED")
	}
	for _, role := range in.Filters.ArtifactRoles {
		if role != "knowledge_profile" && role != "source_chunk" {
			return semanticError(400, "KNOWLEDGE_ARTIFACT_ROLE_INVALID")
		}
	}
	for _, code := range in.Filters.CategoryCodes {
		if strings.TrimSpace(code) == "" || len(code) > 128 {
			return semanticError(400, "KNOWLEDGE_FILTER_INVALID")
		}
	}
	for space, generation := range in.RequiredGenerations {
		if !validSemanticUUID(generation) || !containsString(in.SpaceUUIDs, space) {
			return semanticError(400, "KNOWLEDGE_GENERATION_INVALID")
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (s *SemanticRuntime) Query(ctx context.Context, tenant string, in SemanticQuery) (SemanticResult, error) {
	in.SpaceUUIDs = cloneSemanticValues(in.SpaceUUIDs)
	for i := range in.SpaceUUIDs {
		in.SpaceUUIDs[i] = canonicalSemanticUUID(in.SpaceUUIDs[i])
	}
	in.Filters.DocumentUUIDs = cloneSemanticValues(in.Filters.DocumentUUIDs)
	for i := range in.Filters.DocumentUUIDs {
		in.Filters.DocumentUUIDs[i] = canonicalSemanticUUID(in.Filters.DocumentUUIDs[i])
	}
	in.Filters.TagUUIDs = cloneSemanticValues(in.Filters.TagUUIDs)
	for i := range in.Filters.TagUUIDs {
		in.Filters.TagUUIDs[i] = canonicalSemanticUUID(in.Filters.TagUUIDs[i])
	}
	if in.RequiredGenerations != nil {
		canonical := map[string]string{}
		for space, gen := range in.RequiredGenerations {
			canonical[canonicalSemanticUUID(space)] = canonicalSemanticUUID(gen)
		}
		in.RequiredGenerations = canonical
	}
	out := SemanticResult{Schema: SemanticResultSchema, QueryUUID: uuid.NewString(), RequestedMode: in.Mode, EffectiveMode: in.Mode, Generations: []SemanticGeneration{}, Items: []SemanticMatch{}, TraceID: reqctx.GetTraceID(ctx)}
	if err := validateSemanticQuery(in); err != nil {
		return out, err
	}
	if s.vectors == nil || s.vectors.Driver() != "pgvector" {
		return out, semanticError(501, "KNOWLEDGE_SEMANTIC_UNSUPPORTED")
	}
	if err := s.checkVectorDatabase(ctx); err != nil {
		return out, err
	}
	all := []SemanticMatch{}
	for _, space := range in.SpaceUUIDs {
		var binding models.SemanticSpaceBinding
		var spaceRow models.KnowledgeSpace
		if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status <> ?", tenant, space, models.KnowledgeSpaceStatusRetired).First(&spaceRow).Error != nil {
			return out, KnowledgeSpaceNotFoundError(errors.New("space unavailable"))
		}
		if s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).First(&binding).Error != nil {
			return out, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
		}
		generation, err := s.generation(ctx, binding)
		if err != nil {
			return out, err
		}
		if expected := in.RequiredGenerations[space]; expected != "" && expected != generation.CorpusGeneration {
			return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
		}
		frozen, err := s.freeze(ctx, tenant, space, SemanticIndexingSettings{Mode: in.Mode, EmbeddingProfile: generation.EmbeddingProfile, ArtifactRoles: []string{"source_chunk"}})
		if err != nil {
			return out, err
		}
		embed, err := s.validateFrozen(ctx, tenant, space, frozen)
		if err != nil {
			return out, err
		}
		vectors, err := embed.Embed(ctx, []string{in.Query})
		if err != nil || len(vectors) != 1 || len(vectors[0]) != generation.Dimensions {
			return out, semanticError(503, "KNOWLEDGE_QUERY_EMBEDDING_FAILED")
		}
		if _, err := s.validateFrozen(ctx, tenant, space, frozen); err != nil {
			return out, err
		}
		norm := float64(0)
		for _, v := range vectors[0] {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return out, semanticError(503, "KNOWLEDGE_EMBEDDING_INVALID")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return out, semanticError(503, "KNOWLEDGE_EMBEDDING_INVALID")
		}
		index, err := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, spaceRow.UUID, binding.VectorIndexKey)
		if err != nil || index == nil {
			return out, semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
		}
		schema, err := quoteSemanticIdentifier(s.vectors.pgBase.WithDefaults().Schema)
		if err != nil {
			return out, err
		}
		table, err := quoteSemanticIdentifier(index.VectorTable)
		if err != nil {
			return out, err
		}
		from := fmt.Sprintf(" FROM %s.%s v JOIN %s c ON c.uuid = v.chunk_uuid AND c.space_uuid = v.space_uuid JOIN %s d ON d.uuid = c.document_uuid AND d.tenant_uuid = c.tenant_uuid AND d.active_index_job_uuid = c.job_uuid JOIN %s j ON j.uuid = c.job_uuid AND j.tenant_uuid = c.tenant_uuid AND j.status = 'succeeded' WHERE c.tenant_uuid = ? AND c.space_uuid = ? AND d.deleted_at IS NULL AND c.deleted_at IS NULL AND d.queryable = TRUE AND d.index_status = 'indexed' AND c.metadata->'semantic'->>'configuration_generation' = ?", schema, table, models.HostDocumentChunk{}.TableName(), models.TenantDocument{}.TableName(), models.IndexJob{}.TableName())
		args := []any{tenant, space, binding.ConfigurationGeneration}
		if in.Mode == HybridMode {
			from += " AND c.metadata->'semantic'->>'mode' = 'hybrid'"
		}
		if len(in.Filters.DocumentUUIDs) > 0 {
			from += " AND d.uuid IN ?"
			args = append(args, in.Filters.DocumentUUIDs)
		}
		if len(in.Filters.ArtifactRoles) > 0 {
			from += " AND c.metadata->'semantic'->>'artifact_role' IN ?"
			args = append(args, in.Filters.ArtifactRoles)
		}
		for _, filter := range []struct {
			key    string
			values []string
		}{{"category_codes", in.Filters.CategoryCodes}, {"tag_uuids", in.Filters.TagUUIDs}} {
			if len(filter.values) == 0 {
				continue
			}
			parts := []string{}
			for _, value := range filter.values {
				parts = append(parts, "c.metadata->'semantic'->'"+filter.key+"' @> CAST(? AS jsonb)")
				body, _ := json.Marshal([]string{value})
				args = append(args, string(body))
			}
			from += " AND (" + strings.Join(parts, " OR ") + ")"
		}
		var vectorRows []semanticCandidate
		queryArgs := append([]any{pgvector.NewVector(vectors[0])}, args...)
		queryArgs = append(queryArgs, in.TopK)
		sql := "SELECT c.uuid AS chunk_uuid,c.document_uuid,c.content,c.metadata,v.embedding <=> CAST(? AS vector) AS distance" + from + " ORDER BY distance ASC,c.uuid ASC LIMIT ?"
		if s.db.WithContext(ctx).Raw(sql, queryArgs...).Scan(&vectorRows).Error != nil {
			return out, semanticError(503, "KNOWLEDGE_VECTOR_QUERY_FAILED")
		}
		lexicalRows := []semanticCandidate{}
		if in.Mode == HybridMode {
			lexFrom := from + " AND c.metadata->'semantic'->>'mode' = 'hybrid'"
			lexArgs := append([]any{in.Query}, args...)
			lexArgs = append(lexArgs, in.TopK)
			lexSQL := "SELECT c.uuid AS chunk_uuid,c.document_uuid,c.content,c.metadata,ts_rank_cd(to_tsvector('simple',c.content),plainto_tsquery('simple',?)) AS lexical_score" + lexFrom + " ORDER BY lexical_score DESC,c.uuid ASC LIMIT ?"
			if s.db.WithContext(ctx).Raw(lexSQL, lexArgs...).Scan(&lexicalRows).Error != nil {
				return out, semanticError(503, "KNOWLEDGE_LEXICAL_QUERY_FAILED")
			}
		}
		matches := map[string]*SemanticMatch{}
		sources := map[string]hostDocumentSource{}
		add := func(row semanticCandidate, rank int, source string) error {
			if source == "vector" && (math.IsNaN(row.Distance) || math.IsInf(row.Distance, 0)) {
				return semanticError(503, "KNOWLEDGE_VECTOR_RESULT_INVALID")
			}
			match := matches[row.ChunkUUID]
			if match == nil {
				var metadata struct {
					Semantic semanticChunkMetadata `json:"semantic"`
					Title    string                `json:"source_title"`
				}
				if json.Unmarshal(row.Metadata, &metadata) != nil || metadata.Semantic.ArtifactUUID == "" {
					return semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
				}
				m := metadata.Semantic
				match = &SemanticMatch{SpaceUUID: space, DocumentUUID: row.DocumentUUID, ChunkUUID: row.ChunkUUID, ArtifactUUID: m.ArtifactUUID, ArtifactRole: m.ArtifactRole, KnowledgeProfileUUID: m.KnowledgeProfileUUID, CaseUUID: m.CaseUUID, Title: metadata.Title, Text: row.Content, ExternalRef: m.ExternalRef, SourceRef: m.SourceRef, DocumentVersion: m.DocumentVersion, BundleGeneration: "", ScoreType: "rrf", Scores: []SemanticScore{}, RetrievalSources: []string{}}
				var chunk models.HostDocumentChunk
				if s.db.WithContext(ctx).Select("job_uuid").Where("tenant_uuid = ? AND uuid = ?", tenant, row.ChunkUUID).First(&chunk).Error != nil {
					return semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
				}
				source, ok := sources[chunk.JobUUID+":"+row.DocumentUUID]
				if !ok {
					var savedJob models.IndexJob
					if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, chunk.JobUUID, HostIndexStatusSucceeded).First(&savedJob).Error != nil {
						return semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
					}
					var err error
					source, err = semanticJobSource(savedJob, row.DocumentUUID)
					if err != nil {
						return err
					}
					sources[chunk.JobUUID+":"+row.DocumentUUID] = source
				}
				if err := validateSemanticHydration(row, m, source); err != nil {
					return err
				}
				match.BundleGeneration = chunk.JobUUID
				matches[row.ChunkUUID] = match
			}
			if in.Mode == SemanticMode {
				match.Score = 1 - row.Distance
				match.ScoreType = "cosine_similarity"
			} else {
				match.Score += 1 / float64(60+rank)
			}
			if source == "vector" {
				match.Scores = append(match.Scores, SemanticScore{"vector", row.Distance, "cosine_distance", "lower_better"}, SemanticScore{"vector", 1 - row.Distance, "cosine_similarity", "higher_better"})
			} else {
				match.Scores = append(match.Scores, SemanticScore{"lexical", row.LexicalScore, "postgres_text_rank", "higher_better"})
			}
			match.RetrievalSources = append(match.RetrievalSources, source)
			return nil
		}
		for i, row := range vectorRows {
			if err := add(row, i+1, "vector"); err != nil {
				return out, err
			}
		}
		for i, row := range lexicalRows {
			if row.LexicalScore > 0 {
				if err := add(row, i+1, "lexical"); err != nil {
					return out, err
				}
			}
		}
		for _, match := range matches {
			all = append(all, *match)
		}
		var finalBinding models.SemanticSpaceBinding
		if s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).First(&finalBinding).Error != nil || finalBinding.ConfigurationGeneration != binding.ConfigurationGeneration || finalBinding.CorpusGeneration != binding.CorpusGeneration {
			return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
		}
		out.Generations = append(out.Generations, generation)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score == all[j].Score {
			return all[i].ChunkUUID < all[j].ChunkUUID
		}
		return all[i].Score > all[j].Score
	})
	if len(all) > in.TopK {
		all = all[:in.TopK]
	}
	// Check all spaces again after the last embedding/hydration. Revocation of
	// an earlier space must not race a later space and return stale evidence.
	for _, g := range out.Generations {
		var b models.SemanticSpaceBinding
		var active models.KnowledgeSpace
		if s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ?", tenant, g.SpaceUUID).First(&b).Error != nil || b.ConfigurationGeneration != g.ConfigurationGeneration || b.CorpusGeneration != g.CorpusGeneration {
			return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
		}
		if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status <> ?", tenant, g.SpaceUUID, models.KnowledgeSpaceStatusRetired).First(&active).Error != nil {
			return out, KnowledgeSpaceNotFoundError(errors.New("space unavailable"))
		}
	}
	out.Items = all
	return out, nil
}
