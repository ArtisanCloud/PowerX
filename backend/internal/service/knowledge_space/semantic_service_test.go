package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type semanticTestVectorizer struct{ fail bool }

func (v *semanticTestVectorizer) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if v.fail {
		return nil, errors.New("embedding unavailable")
	}
	out := [][]float32{}
	for range texts {
		out = append(out, []float32{1, .5})
	}
	return out, nil
}

type semanticTestStore struct {
	vectors []vectorstore.VectorRecord
	fail    bool
}

func (s *semanticTestStore) Driver() string { return "test" }
func (s *semanticTestStore) Upsert(_ context.Context, _ uuid.UUID, rows []vectorstore.VectorRecord) error {
	if s.fail {
		return errors.New("store unavailable")
	}
	s.vectors = append(s.vectors, rows...)
	return nil
}
func (s *semanticTestStore) DeleteByChunkIDs(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (s *semanticTestStore) DropSpace(context.Context, uuid.UUID) error { return nil }
func (s *semanticTestStore) Query(context.Context, vectorstore.QueryRequest) (vectorstore.QueryResponse, error) {
	return vectorstore.QueryResponse{}, nil
}
func (s *semanticTestStore) Health(context.Context) error { return nil }
func (s *semanticTestStore) Close(context.Context) error  { return nil }

func TestSemanticBundlePublicationFreezesSourcesAndProtectsOldVersion(t *testing.T) {
	host, db, tenant, space := newHostContractTestService(t, false)
	require.NoError(t, db.AutoMigrate(&models.SemanticSpaceBinding{}, &models.SemanticEmbeddingProfile{}))
	profile := models.SemanticEmbeddingProfile{TenantUUID: tenant, ProfileKey: "test/model", Version: 1, Status: "published", Dimensions: 2, ModelRevision: "immutable-model", ConfigChecksum: hostChecksum("config")}
	require.NoError(t, db.Create(&profile).Error)
	binding := models.SemanticSpaceBinding{TenantUUID: tenant, SpaceUUID: space, EmbeddingProfileUUID: profile.UUID.String(), ConfigurationGeneration: uuid.NewString(), CorpusGeneration: uuid.NewString(), VectorIndexKey: "dense", Modes: []byte(`["semantic","hybrid"]`)}
	require.NoError(t, db.Create(&binding).Error)
	vectorizer, store := &semanticTestVectorizer{}, &semanticTestStore{}
	runtime := NewSemanticRuntime(db, nil, store)
	runtime.embedOverride = func(context.Context, string, string) (*resolvedEmbeddingProfile, agentSvcEmbedVectorizer, string, string, error) {
		return &resolvedEmbeddingProfile{Dimensions: 2}, vectorizer, profile.ModelRevision, profile.ConfigChecksum, nil
	}
	host.WithSemantic(runtime)
	text := "品牌舆情发酵，需要危机公关应对和客户沟通。"
	artifact := SemanticArtifactInput{UUID: uuid.NewString(), Role: "source_chunk", Text: text, Checksum: hostChecksum(text), Version: "v1", SourceRef: SemanticSourceRef{Kind: "cleaned_document", SourceUUID: uuid.NewString(), Version: "v1", Checksum: hostChecksum(text), PositionUnit: "unicode_codepoint", CharEnd: len([]rune(text))}, CategoryCodes: []string{"public_relations"}, TagUUIDs: []string{}}
	input := HostDocumentInput{Title: "危机公关", URI: "powerx://semantic/test", Content: text, Checksum: hostChecksum(text), ContentType: "text/plain", Version: "v1", Indexing: &SemanticIndexingSettings{Mode: HybridMode, EmbeddingProfile: HostTaskProfileRef{UUID: profile.UUID.String(), Version: 1}, ArtifactRoles: []string{"source_chunk"}}, Artifacts: []SemanticArtifactInput{artifact}, IdempotencyKey: "first"}
	accepted, err := host.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	repeated, err := host.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	require.Equal(t, accepted.JobUUID, repeated.JobUUID)
	handled, err := host.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	require.True(t, handled)
	job, err := host.GetIndexJob(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", job.Status)
	require.NotEmpty(t, store.vectors)
	chunks, err := host.GetJobChunks(context.Background(), tenant, accepted.JobUUID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	var metadata struct {
		Semantic semanticChunkMetadata `json:"semantic"`
	}
	require.NoError(t, json.Unmarshal(chunks[0].Metadata, &metadata))
	require.Equal(t, artifact.SourceRef, metadata.Semantic.SourceRef)
	vectorizer.fail = true
	input.IdempotencyKey = "retry"
	input.Version = "v2"
	input.Content += "新增内容"
	input.Checksum = hostChecksum(input.Content)
	next, err := host.UpsertDocument(context.Background(), tenant, space, input)
	require.NoError(t, err)
	_, err = host.ProcessNextDocumentJob(context.Background())
	require.NoError(t, err)
	failed, err := host.GetIndexJob(context.Background(), tenant, next.JobUUID)
	require.NoError(t, err)
	require.Equal(t, "failed", failed.Status)
	require.Equal(t, "KNOWLEDGE_EMBEDDING_FAILED", failed.ErrorCode)
	var document models.TenantDocument
	require.NoError(t, db.Where("uuid = ?", accepted.DocumentUUID).First(&document).Error)
	require.Equal(t, accepted.JobUUID, *document.ActiveIndexJobUUID)
	_, err = host.SetDocumentVisibility(context.Background(), tenant, space, accepted.DocumentUUID, SemanticVisibilityInput{Queryable: false})
	require.NoError(t, err)
	require.NoError(t, db.Where("uuid = ?", accepted.DocumentUUID).First(&document).Error)
	require.False(t, document.Queryable)
}

func TestSemanticContractRejectsFakeLocationsAndUnreadyProfile(t *testing.T) {
	host, _, tenant, space := newHostContractTestService(t, false)
	in := HostDocumentInput{Title: "x", URI: "powerx://x", Content: "x", ContentType: "text/plain", Checksum: hostChecksum("x"), Version: "v1", Indexing: &SemanticIndexingSettings{Mode: "semantic", ArtifactRoles: []string{"source_chunk"}}, Artifacts: []SemanticArtifactInput{{UUID: uuid.NewString(), Role: "source_chunk", Text: "x", Checksum: hostChecksum("x"), Version: "v1"}}}
	_, err := host.UpsertDocument(context.Background(), tenant, space, in)
	require.Equal(t, "KNOWLEDGE_SOURCE_POSITION_INVALID", dto.CodeOf(err))
	query := SemanticQuery{Schema: SemanticQuerySchema, Query: "完整自然语言需求", Mode: "semantic", SpaceUUIDs: []string{space}, TopK: 10, Filters: SemanticFilters{DocumentUUIDs: []string{"123"}}}
	require.Error(t, validateSemanticQuery(query))
}
