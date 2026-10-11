package knowledge_space

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"

	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iammodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	pgcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func contentPostgresFixture(t *testing.T) (*ContentOverviewService, *gorm.DB, context.Context, string, string, models.SemanticSpaceBinding) {
	t.Helper()
	path := os.Getenv("POWERX_CONTENT_POSTGRES_CONFIG")
	if path == "" {
		t.Skip("opt-in isolated PostgreSQL/pgvector content overview tests")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg struct {
		Database struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			Username string `yaml:"username"`
			Password string `yaml:"password"`
			Database string `yaml:"database"`
		} `yaml:"database"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	d := cfg.Database
	address := url.URL{Scheme: "postgres", User: url.UserPassword(d.Username, d.Password), Host: net.JoinHostPort(d.Host, fmt.Sprint(d.Port)), Path: d.Database, RawQuery: "sslmode=disable"}
	db, err := gorm.Open(postgres.Open(address.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	old := core.PowerXSchema
	schema := "content_accept_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	core.PowerXSchema = schema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		sql, err := db.DB()
		require.NoError(t, err)
		_ = sql.Close()
		core.PowerXSchema = old
	})
	require.NoError(t, db.AutoMigrate(&models.KnowledgeSpace{}, &models.SemanticSpaceBinding{}, &models.SemanticEmbeddingProfile{}, &models.TenantDocument{}, &models.IndexJob{}, &models.HostDocumentChunk{}, &models.KnowledgeVectorIndex{}))
	require.NoError(t, db.Exec(`CREATE TABLE "`+schema+`"."vectors" (space_uuid uuid NOT NULL,chunk_uuid uuid NOT NULL,embedding public.vector(2) NOT NULL)`).Error)
	tenant, space := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.KnowledgeSpace{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(space)}, TenantUUID: tenant, SpaceName: "content test", Status: models.KnowledgeSpaceStatusActive, ActiveVectorIndexKey: "dense"}).Error)
	profile := models.SemanticEmbeddingProfile{TenantUUID: tenant, Env: "dev", ProfileKey: "fixture/semantic", Version: 1, Status: "published", Dimensions: 2, ModelRevision: "immutable", ConfigChecksum: hostChecksum("config")}
	require.NoError(t, db.Create(&profile).Error)
	binding := models.SemanticSpaceBinding{TenantUUID: tenant, SpaceUUID: space, EmbeddingProfileUUID: profile.UUID.String(), ConfigurationGeneration: uuid.NewString(), CorpusGeneration: uuid.NewString(), VectorIndexKey: "dense", Modes: datatypes.JSON(`["semantic","hybrid"]`)}
	require.NoError(t, db.Create(&binding).Error)
	require.NoError(t, db.Create(&models.KnowledgeVectorIndex{SpaceUUID: uuid.MustParse(space), IndexKey: "dense", Dimensions: 2, VectorTable: "vectors", Status: models.KnowledgeVectorIndexStatusActive}).Error)
	routed := NewRoutedVectorStore(RoutedVectorStoreOptions{DB: db, BaseDriver: "pgvector", PGVector: pgcfg.Config{DSN: address.String(), Schema: schema}})
	service := NewContentOverviewService(db, NewSemanticRuntime(db, nil, routed), []byte(strings.Repeat("test-secret", 4)))
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, UserID: 1, IsRoot: true})
	return service, db, ctx, tenant, space, binding
}

func addContentFixture(t *testing.T, db *gorm.DB, tenant, space, configuration, mode, title, text string, queryable, vector bool) string {
	t.Helper()
	document := uuid.NewString()
	jobID := uuid.NewString()
	artifactID := uuid.NewString()
	ref := SemanticSourceRef{Kind: "cleaned_document", SourceUUID: uuid.NewString(), Version: "source-v1", Checksum: hostChecksum(text), PositionUnit: "unicode_codepoint", CharEnd: len([]rune(text))}
	source := hostDocumentSource{Title: title, URI: "powerx://fixture/" + document, Version: "source-v1", Artifacts: []SemanticArtifactInput{{UUID: artifactID, Role: "source_chunk", Text: text, Version: "source-v1", Checksum: hostChecksum(text), SourceRef: ref}}}
	sourceBytes, _ := json.Marshal(source)
	job := models.IndexJob{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(jobID)}, TenantUUID: tenant, SpaceUUID: space, Operation: HostIndexOperationUpsert, Status: HostIndexStatusSucceeded, SourceSnapshot: sourceBytes, ConfigSnapshot: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&job).Error)
	doc := models.TenantDocument{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(document)}, TenantUUID: tenant, SpaceUUID: space, Title: "unpublished title must not leak", URI: source.URI, Version: "future-v2", Content: "future text", ContentType: "text/plain", Checksum: hostChecksum("future text"), Queryable: true, VisibilityEpoch: uuid.NewString(), IndexStatus: HostDocumentStatusIndexed, ActiveIndexJobUUID: &jobID}
	require.NoError(t, db.Create(&doc).Error)
	if !queryable {
		require.NoError(t, db.Model(&doc).Update("queryable", false).Error)
	}
	meta, _ := json.Marshal(map[string]any{"semantic": semanticChunkMetadata{ArtifactUUID: artifactID, ArtifactRole: "source_chunk", SourceRef: ref, DocumentVersion: source.Version, ConfigurationGeneration: configuration, Mode: mode}})
	chunk := models.HostDocumentChunk{TenantUUID: tenant, SpaceUUID: space, DocumentUUID: document, JobUUID: jobID, Ordinal: 0, Kind: "unit", Content: text, Checksum: hostChecksum(text), Metadata: meta}
	require.NoError(t, db.Create(&chunk).Error)
	if vector {
		require.NoError(t, db.Exec(`INSERT INTO "`+core.PowerXSchema+`"."vectors" (space_uuid,chunk_uuid,embedding) VALUES (?,?,'[1,0]')`, space, chunk.UUID).Error)
	}
	return document
}

func TestContentOverviewPostgresPublicationPaginationAndSource(t *testing.T) {
	s, db, ctx, tenant, space, binding := contentPostgresFixture(t)
	for i := 0; i < 5; i++ {
		addContentFixture(t, db, tenant, space, binding.ConfigurationGeneration, HybridMode, fmt.Sprint("article-", i), "中文😀é 原文预览", true, true)
	}
	addContentFixture(t, db, tenant, space, binding.ConfigurationGeneration, HybridMode, "hidden", "hidden text", false, true)
	addContentFixture(t, db, tenant, space, binding.ConfigurationGeneration, HybridMode, "no vector", "not indexed", true, false)
	addContentFixture(t, db, tenant, space, uuid.NewString(), HybridMode, "old generation", "old corpus", true, true)
	size, preview := 2, 3
	input := ContentOverviewQuery{Schema: ContentOverviewQuerySchema, SpaceUUID: space, Mode: HybridMode, PageSize: &size, PreviewCodepoints: &preview, RequiredCorpusGeneration: binding.CorpusGeneration}
	seen := map[string]bool{}
	for {
		result, err := s.Read(ctx, tenant, input)
		require.NoError(t, err)
		require.Equal(t, 5, result.Pagination.Total)
		require.Equal(t, "not_available", result.Overview.Status)
		for _, item := range result.Items {
			require.False(t, seen[item.DocumentUUID])
			seen[item.DocumentUUID] = true
			require.True(t, strings.HasPrefix(item.Title, "article-"))
			require.Equal(t, "source-v1", item.DocumentVersion)
			require.Equal(t, "中文😀", item.Preview.Text)
			require.Equal(t, 3, item.Preview.CharEnd)
			require.True(t, item.Preview.Truncated)
		}
		if !result.Pagination.HasMore {
			require.Nil(t, result.Pagination.NextCursor)
			break
		}
		require.NotNil(t, result.Pagination.NextCursor)
		input.Cursor = *result.Pagination.NextCursor
	}
	require.Len(t, seen, 5)
	input.Cursor = ""
	first, err := s.Read(ctx, tenant, input)
	require.NoError(t, err)
	input.Cursor = *first.Pagination.NextCursor
	input.Mode = SemanticMode
	_, err = s.Read(ctx, tenant, input)
	require.Equal(t, "KNOWLEDGE_CONTENT_CURSOR_SCOPE_MISMATCH", dto.CodeOf(err))
	input.Mode = HybridMode
	input.RequiredCorpusGeneration = uuid.NewString()
	_, err = s.Read(ctx, tenant, input)
	require.Equal(t, "KNOWLEDGE_INDEX_GENERATION_CONFLICT", dto.CodeOf(err))
	input.RequiredCorpusGeneration = binding.CorpusGeneration
	require.NoError(t, db.Model(&models.SemanticSpaceBinding{}).Where("space_uuid=?", space).Update("corpus_generation", uuid.NewString()).Error)
	_, err = s.Read(ctx, tenant, input)
	require.Equal(t, "KNOWLEDGE_INDEX_GENERATION_CONFLICT", dto.CodeOf(err))
}

func TestContentOverviewPostgresEmptyIntegrityVisibilityAndTenant(t *testing.T) {
	s, db, ctx, tenant, space, binding := contentPostgresFixture(t)
	input := ContentOverviewQuery{Schema: ContentOverviewQuerySchema, SpaceUUID: space, Mode: SemanticMode}
	result, err := s.Read(ctx, tenant, input)
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.Zero(t, result.Pagination.Total)
	foreign := uuid.NewString()
	foreignCtx := reqctx.WithTenantUUID(ctx, foreign)
	_, err = s.Read(foreignCtx, foreign, input)
	require.Equal(t, 404, dto.StatusCode(err))
	document := addContentFixture(t, db, tenant, space, binding.ConfigurationGeneration, HybridMode, "published", "evidence text", true, true)
	addContentFixture(t, db, tenant, space, binding.ConfigurationGeneration, HybridMode, "published 2", "evidence text 2", true, true)
	size := 1
	input.PageSize = &size
	result, err = s.Read(ctx, tenant, input)
	require.NoError(t, err)
	input.Cursor = *result.Pagination.NextCursor
	require.NoError(t, db.Model(&models.TenantDocument{}).Where("uuid=?", document).Update("queryable", false).Error)
	_, err = s.Read(ctx, tenant, input)
	require.Equal(t, "KNOWLEDGE_CONTENT_SNAPSHOT_CONFLICT", dto.CodeOf(err))
	input.Cursor = ""
	require.NoError(t, db.Model(&models.HostDocumentChunk{}).Where("document_uuid <> ?", document).Update("content", "tampered").Error)
	_, err = s.Read(ctx, tenant, input)
	require.Equal(t, "KNOWLEDGE_RESULT_HYDRATION_FAILED", dto.CodeOf(err))
}

func TestContentOverviewPostgresAdminPermissionSource(t *testing.T) {
	s, db, _, tenant, _, _ := contentPostgresFixture(t)
	require.NoError(t, db.AutoMigrate(&iammodels.Role{}, &iammodels.Permission{}, &iammodels.RolePermission{}, &iammodels.RoleBinding{}, &iammodels.MemberAssignment{}))
	memberID := uint64(21)
	ctx := reqctx.WithClaims(reqctx.WithMemberID(reqctx.WithTenantUUID(context.Background(), tenant), memberID), &reqctx.CoreXClaims{TenantUUID: tenant, UserID: 7, MemberID: memberID})
	permission := iammodels.Permission{Module: "knowledge_space", Resource: "knowledge_content_overview", Action: "read", Effect: "allow", Status: iammodels.PermissionStatusActive, Source: "plugin", Meta: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&permission).Error)
	role := iammodels.Role{TenantUUID: tenant, Name: "content-read"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&iammodels.RolePermission{RoleID: role.ID, PermissionID: permission.ID, RoleUUID: role.UUID, PermissionUUID: permission.PermissionUUID}).Error)
	require.NoError(t, db.Create(&iammodels.RoleBinding{TenantUUID: tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: iammodels.SubMember, SubjectID: memberID, SubjectUUID: uuid.NewString()}).Error)
	require.Equal(t, 403, dto.StatusCode(s.authorize(ctx, tenant)), "plugin permission must not replace the platform grant")
	require.NoError(t, db.Model(&permission).Update("source", "platform_capability").Error)
	require.NoError(t, s.authorize(ctx, tenant))
	require.NoError(t, db.Model(&permission).Update("status", iammodels.PermissionStatusDeprecated).Error)
	require.Equal(t, 403, dto.StatusCode(s.authorize(ctx, tenant)))
}
