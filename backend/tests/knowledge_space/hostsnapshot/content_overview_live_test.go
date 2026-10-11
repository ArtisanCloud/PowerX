package hostsnapshot_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	ks "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	igw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Uses actual development articles or explicitly requested, fully cleaned
// publication fixtures. Fixed vectors in fixtures verify reading only, not embedding.
func TestLiveContentOverviewAPIKeyAndSTS(t *testing.T) {
	path, base, grpcAddr := os.Getenv("POWERX_KNOWLEDGE_CONFIG"), os.Getenv("POWERX_KNOWLEDGE_HTTP"), os.Getenv("POWERX_KNOWLEDGE_GRPC")
	if path == "" || base == "" || grpcAddr == "" {
		t.Skip("set isolated Core endpoints and private config")
	}
	cfg, err := config.Load(path)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sql.Close() })
	raw, err := os.ReadFile("../../../cmd/database/seed/seed_dev_api_keys.go")
	require.NoError(t, err)
	match := regexp.MustCompile(`Key:\s*"(pxk_[^"]+)"`).FindSubmatch(raw)
	require.Len(t, match, 2)
	apiKey := string(match[1])
	var key igw.IntegrationGatewayAPIKey
	require.NoError(t, db.Where("key_hash=? AND status='active'", sha(apiKey)).First(&key).Error)
	tenant := key.TenantUUID
	var permissionRows []iam.Permission
	require.NoError(t, db.Where("module=? AND allow_api_key=TRUE AND status=?", "knowledge_space", iam.PermissionStatusActive).Find(&permissionRows).Error)
	var grant igw.IntegrationGatewayAPIKeyPermission
	for _, p := range permissionRows {
		resolved, ok := apikeypermissions.ResolvePermission(p)
		if !ok || resolved.Scope != "_scope.knowledge.content_overview.service_read" {
			continue
		}
		var meta map[string]any
		require.NoError(t, json.Unmarshal(p.Meta, &meta))
		require.Equal(t, true, meta["api_key_explicit"])
		grant = igw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: resolved.Scope, Action: resolved.Action, ResourceType: resolved.ResourceType, ResourcePattern: resolved.ResourcePattern, Effect: resolved.Effect}
		break
	}
	require.NotEmpty(t, grant.Scope, "formal IAM materialization required")
	var count int64
	require.NoError(t, db.Model(&grant).Where("api_key_uuid=? AND scope=?", key.UUID, grant.Scope).Count(&count).Error)
	if count == 0 {
		require.NoError(t, db.Create(&grant).Error)
	}
	// grant-status is a separately authorized self-inspection capability.
	statusGrant := igw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"}
	require.NoError(t, db.Model(&statusGrant).Where("api_key_uuid=? AND scope=?", key.UUID, statusGrant.Scope).Count(&count).Error)
	if count == 0 {
		require.NoError(t, db.Create(&statusGrant).Error)
	}
	var space models.KnowledgeSpace
	origin := "existing_dev_corpus"
	if os.Getenv("POWERX_CONTENT_ACCEPTANCE_FIXTURE") == "1" {
		space = createContentReadFixture(t, db, tenant)
		origin = "isolated_publication_fixture_fixed_vectors_not_embedding_acceptance"
	} else {
		require.NoError(t, db.Where("tenant_uuid=? AND space_name=?", tenant, "dev").First(&space).Error, "dev corpus is required; fixture mode must be explicitly selected")
	}
	root := "/api/v1/tenant/knowledge/spaces/" + space.UUID.String() + "/content-overview"
	records := []evidenceRecord{}
	client := &http.Client{Timeout: 30 * time.Second}
	call := func(name, method, route, authorization string, body any) (int, map[string]json.RawMessage) {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequest(method, base+route, bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Trace-ID", uuid.NewString())
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		response, err := client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		require.NoError(t, err)
		parsed := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		trace := response.Header.Get("X-Trace-ID")
		if trace == "" {
			trace = req.Header.Get("X-Trace-ID")
		}
		records = append(records, evidenceRecord{Name: name, HTTP: response.StatusCode, Trace: trace, Response: data})
		return response.StatusCode, parsed
	}
	if output := os.Getenv("POWERX_KNOWLEDGE_EVIDENCE"); output != "" {
		t.Cleanup(func() {
			data, _ := json.MarshalIndent(map[string]any{"schema": "powerx.knowledge.content-overview-acceptance/v1", "http_endpoint": base, "space_uuid": space.UUID, "corpus_origin": origin, "credentials_redacted": true, "records": records}, "", "  ")
			require.NoError(t, os.WriteFile(output, data, 0600))
		})
	}
	authorization := "ApiKey " + apiKey
	status, result := call("development_grant_status", "POST", "/api/v1/tenant/capabilities:grant-status", authorization, map[string]any{"capability_ids": []string{ks.KnowledgeContentOverviewCapabilityID}})
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), `"granted"`)
	status, result = call("dev_api_key_first_page", "GET", root+"?mode=hybrid&page_size=2", authorization, nil)
	require.Equal(t, 200, status, "%s", result)
	var page ks.ContentOverviewResult
	require.NoError(t, json.Unmarshal(result["data"], &page))
	require.GreaterOrEqual(t, page.Pagination.Total, 1)
	if origin != "existing_dev_corpus" {
		require.Equal(t, 5, page.Pagination.Total, "the sixth fixture article is hidden")
	}
	firstCursor := page.Pagination.NextCursor
	seen := map[string]bool{}
	for {
		for _, item := range page.Items {
			require.False(t, seen[item.DocumentUUID])
			seen[item.DocumentUUID] = true
			require.NotEmpty(t, item.Title)
			require.NotEmpty(t, item.Preview.Text)
			var job models.IndexJob
			require.NoError(t, db.Where("uuid=? AND tenant_uuid=? AND space_uuid=? AND status='succeeded'", item.BundleGeneration, tenant, space.UUID).First(&job).Error)
			var source struct {
				Title   string `json:"title"`
				Version string `json:"version"`
			}
			require.NoError(t, json.Unmarshal(job.SourceSnapshot, &source))
			require.Equal(t, source.Title, item.Title)
			require.Equal(t, source.Version, item.DocumentVersion)
		}
		if !page.Pagination.HasMore {
			break
		}
		status, result = call("dev_api_key_next_page", "GET", root+"?mode=hybrid&page_size=2&cursor="+url.QueryEscape(*page.Pagination.NextCursor), authorization, nil)
		require.Equal(t, 200, status)
		require.NoError(t, json.Unmarshal(result["data"], &page))
	}
	require.Len(t, seen, page.Pagination.Total)
	t.Logf("verified directory (%s): %d distinct published articles", origin, len(seen))
	status, result = call("explicit_zero_preview", "GET", root+"?mode=semantic&preview_codepoints=0", authorization, nil)
	require.Equal(t, 200, status)
	var zero ks.ContentOverviewResult
	require.NoError(t, json.Unmarshal(result["data"], &zero))
	for _, item := range zero.Items {
		require.Empty(t, item.Preview.Text)
	}
	status, _ = call("stale_generation_rejected", "GET", root+"?mode=hybrid&required_corpus_generation="+uuid.NewString(), authorization, nil)
	require.Equal(t, 409, status)
	status, _ = call("unknown_tenant_override_rejected", "GET", root+"?mode=hybrid&tenant_uuid="+uuid.NewString(), authorization, nil)
	require.Equal(t, 400, status)
	status, _ = call("no_credential_rejected", "GET", root+"?mode=hybrid", "", nil)
	require.Equal(t, 401, status)
	var foreign models.KnowledgeSpace
	foreign = models.KnowledgeSpace{TenantUUID: uuid.NewString(), SpaceName: "content-acceptance-foreign-" + uuid.NewString(), Status: models.KnowledgeSpaceStatusActive}
	require.NoError(t, db.Create(&foreign).Error)
	t.Cleanup(func() { require.NoError(t, db.Unscoped().Delete(&foreign).Error) })
	status, _ = call("foreign_space_rejected", "GET", "/api/v1/tenant/knowledge/spaces/"+foreign.UUID.String()+"/content-overview?mode=hybrid", authorization, nil)
	require.Equal(t, 404, status)
	readKeyText := "pxk_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	readKey := igw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: key.ProfileID, Name: "content-read-acceptance", KeyPrefix: readKeyText[:12], KeyHash: sha(readKeyText), Status: "active"}
	require.NoError(t, db.Create(&readKey).Error)
	legacyKey := iam.APIKey{TenantUUID: tenant, ProfileID: key.ProfileID, KeyHash: sha(readKeyText)}
	require.NoError(t, db.Create(&legacyKey).Error)
	readGrant := igw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: readKey.UUID, Scope: grant.Scope, Action: grant.Action, ResourceType: grant.ResourceType, ResourcePattern: grant.ResourcePattern, Effect: grant.Effect}
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("api_key_uuid=?", readKey.UUID).Delete(&igw.IntegrationGatewayAPIKeyPermission{}).Error)
		require.NoError(t, db.Unscoped().Delete(&legacyKey).Error)
		require.NoError(t, db.Unscoped().Delete(&readKey).Error)
	})
	readAuthorization := "ApiKey " + readKeyText
	status, _ = call("api_key_missing_content_grant", "GET", root+"?mode=hybrid", readAuthorization, nil)
	require.Equal(t, 403, status)
	require.NoError(t, db.Create(&readGrant).Error)
	status, _ = call("least_privilege_content_read", "GET", root+"?mode=hybrid", readAuthorization, nil)
	require.Equal(t, 200, status)
	if firstCursor != nil {
		status, _ = call("cursor_other_credential_rejected", "GET", root+"?mode=hybrid&page_size=2&cursor="+url.QueryEscape(*firstCursor), readAuthorization, nil)
		require.Equal(t, 400, status)
	}
	status, _ = call("read_key_document_write_denied", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", readAuthorization, map[string]any{})
	require.Equal(t, 403, status)
	require.NoError(t, db.Delete(&readGrant).Error)
	status, _ = call("api_key_live_grant_revoked", "GET", root+"?mode=hybrid", readAuthorization, nil)
	require.Equal(t, 403, status)
	plugin := "com.powerx.acceptance.content." + strings.ReplaceAll(uuid.NewString(), "-", "")
	creds := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	expires := time.Now().Add(15 * time.Minute).Unix()
	cid, secret, err := creds.EnsureCredentials(context.Background(), tenant, plugin, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{ks.KnowledgeContentOverviewCapabilityID}, ExpiresAt: &expires})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, creds.DeleteCredentials(context.Background(), tenant, plugin, false)) })
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	exchange, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{Ctx: &commonv1.RequestContext{RequestId: uuid.NewString()}, ClientId: cid, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 600})
	require.NoError(t, err)
	require.EqualValues(t, 200, exchange.GetMeta().GetCode())
	bearer := "Bearer " + exchange.GetData().GetAccessToken()
	require.NotEqual(t, "Bearer ", bearer)
	records = append(records, evidenceRecord{Name: "real_sts_exchange", HTTP: 200, Trace: exchange.GetMeta().GetRequestId(), Response: json.RawMessage(`{"issuer":"powerx-sts","audience":"powerx:api","token_redacted":true}`)})
	status, result = call("dev_real_sts_directory", "GET", root+"?mode=hybrid", bearer, nil)
	require.Equal(t, 200, status, "%s", result)
	status, result = call("typed_core_internal", "POST", "/api/v1/tenant/invocations", authorization, map[string]any{"capability_id": ks.KnowledgeContentOverviewCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"body": map[string]any{"operation": "list_articles", "query": ks.ContentOverviewQuery{Schema: ks.ContentOverviewQuerySchema, SpaceUUID: space.UUID.String(), Mode: ks.HybridMode}}}})
	require.Equal(t, 200, status, "%s", result)
	status, _ = call("free_form_proxy_rejected", "POST", "/api/v1/tenant/invocations", authorization, map[string]any{"capability_id": ks.KnowledgeContentOverviewCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "GET", "endpoint": "/admin/root", "body": map[string]any{"operation": "list_articles"}}})
	require.Equal(t, 400, status)
	require.NoError(t, creds.DeleteCredentials(context.Background(), tenant, plugin, false))
	status, _ = call("live_sts_grant_revoked", "GET", root+"?mode=hybrid", bearer, nil)
	require.Equal(t, 403, status)
	if origin != "existing_dev_corpus" {
		var binding models.SemanticSpaceBinding
		require.NoError(t, db.Where("space_uuid=?", space.UUID).First(&binding).Error)
		require.NoError(t, db.Model(&binding).Update("configuration_generation", uuid.NewString()).Error)
		status, result = call("legitimate_empty_directory", "GET", root+"?mode=hybrid", authorization, nil)
		require.Equal(t, 200, status)
		var empty ks.ContentOverviewResult
		require.NoError(t, json.Unmarshal(result["data"], &empty))
		require.Empty(t, empty.Items)
		require.Zero(t, empty.Pagination.Total)
		// Restore the frozen configuration from the first response.
		require.NoError(t, db.Model(&binding).Update("configuration_generation", page.Generation.ConfigurationGeneration).Error)
		if firstCursor != nil {
			var document models.TenantDocument
			require.NoError(t, db.Where("space_uuid=? AND queryable=TRUE", space.UUID).First(&document).Error)
			require.NoError(t, db.Model(&document).Update("queryable", false).Error)
			status, _ = call("cursor_visibility_changed", "GET", root+"?mode=hybrid&page_size=2&cursor="+url.QueryEscape(*firstCursor), authorization, nil)
			require.Equal(t, 409, status)
		}
		require.NoError(t, db.Model(&models.HostDocumentChunk{}).Where("space_uuid=?", space.UUID).Update("content", "corrupted source fixture").Error)
		status, _ = call("source_integrity_failure", "GET", root+"?mode=hybrid", authorization, nil)
		require.Equal(t, 503, status)
	}
}

func createContentReadFixture(t *testing.T, db *gorm.DB, tenant string) models.KnowledgeSpace {
	t.Helper()
	identifier := strings.ReplaceAll(uuid.NewString(), "-", "")
	table := "content_read_accept_" + identifier
	space := models.KnowledgeSpace{TenantUUID: tenant, SpaceName: "content-overview-acceptance-" + identifier, Status: models.KnowledgeSpaceStatusActive, ActiveVectorIndexKey: "fixture"}
	require.NoError(t, db.Create(&space).Error)
	profile := models.SemanticEmbeddingProfile{TenantUUID: tenant, Env: "dev", ProfileKey: "fixture/fixed-vectors", Version: 1, Status: "published", Dimensions: 2, ModelRevision: "fixture-v1", ConfigChecksum: sha("fixture")}
	require.NoError(t, db.Create(&profile).Error)
	binding := models.SemanticSpaceBinding{TenantUUID: tenant, SpaceUUID: space.UUID.String(), EmbeddingProfileUUID: profile.UUID.String(), ConfigurationGeneration: uuid.NewString(), CorpusGeneration: uuid.NewString(), VectorIndexKey: "fixture", Modes: datatypes.JSON(`["semantic","hybrid"]`)}
	require.NoError(t, db.Create(&binding).Error)
	index := models.KnowledgeVectorIndex{SpaceUUID: space.UUID, IndexKey: "fixture", VectorTable: table, Dimensions: 2, Status: models.KnowledgeVectorIndexStatusActive}
	require.NoError(t, db.Create(&index).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP TABLE IF EXISTS "public"."`+table+`"`).Error)
		for _, model := range []any{&models.HostDocumentChunk{}, &models.IndexJob{}, &models.TenantDocument{}, &models.SemanticSpaceBinding{}, &models.KnowledgeVectorIndex{}} {
			require.NoError(t, db.Unscoped().Where("space_uuid=?", space.UUID).Delete(model).Error)
		}
		require.NoError(t, db.Unscoped().Delete(&profile).Error)
		require.NoError(t, db.Unscoped().Delete(&space).Error)
	})
	require.NoError(t, db.Exec(`CREATE TABLE "public"."`+table+`" (space_uuid uuid NOT NULL,chunk_uuid uuid NOT NULL,embedding public.vector(2) NOT NULL)`).Error)
	for i := 0; i < 6; i++ {
		documentID, jobID, artifactID := uuid.NewString(), uuid.NewString(), uuid.NewString()
		text := fmt.Sprintf("读取验收文章 %d：中文😀预览须来自已发布的原文片段。", i+1)
		ref := ks.SemanticSourceRef{Kind: "cleaned_document", SourceUUID: uuid.NewString(), Version: "v1", Checksum: sha(text), PositionUnit: "unicode_codepoint", CharEnd: len([]rune(text))}
		source, _ := json.Marshal(map[string]any{"title": fmt.Sprintf("内容导览验收文章 %d", i+1), "uri": "powerx://acceptance/" + documentID, "version": "v1", "artifacts": []ks.SemanticArtifactInput{{UUID: artifactID, Role: "source_chunk", Text: text, Version: "v1", Checksum: sha(text), SourceRef: ref}}})
		job := models.IndexJob{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(jobID)}, TenantUUID: tenant, SpaceUUID: space.UUID.String(), Operation: ks.HostIndexOperationUpsert, Status: ks.HostIndexStatusSucceeded, SourceSnapshot: source, ConfigSnapshot: datatypes.JSON(`{}`)}
		require.NoError(t, db.Create(&job).Error)
		doc := models.TenantDocument{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(documentID)}, TenantUUID: tenant, SpaceUUID: space.UUID.String(), Title: "unpublished draft title", URI: "powerx://acceptance/" + documentID, Content: "unpublished draft text", ContentType: "text/plain", Version: "draft-v2", Checksum: sha("unpublished draft text"), VisibilityEpoch: uuid.NewString(), Queryable: true, IndexStatus: ks.HostDocumentStatusIndexed, ActiveIndexJobUUID: &jobID}
		require.NoError(t, db.Create(&doc).Error)
		if i == 5 {
			require.NoError(t, db.Model(&doc).Update("queryable", false).Error)
		}
		metadata, _ := json.Marshal(map[string]any{"semantic": map[string]any{"artifact_uuid": artifactID, "artifact_role": "source_chunk", "source_ref": ref, "document_version": "v1", "configuration_generation": binding.ConfigurationGeneration, "mode": "hybrid"}})
		chunk := models.HostDocumentChunk{TenantUUID: tenant, SpaceUUID: space.UUID.String(), DocumentUUID: documentID, JobUUID: jobID, Ordinal: 0, Kind: "unit", Content: text, Checksum: sha(text), Metadata: metadata}
		require.NoError(t, db.Create(&chunk).Error)
		require.NoError(t, db.Exec(`INSERT INTO "public"."`+table+`" (space_uuid,chunk_uuid,embedding) VALUES (?,?,'[1,0]')`, space.UUID, chunk.UUID).Error)
	}
	return space
}
