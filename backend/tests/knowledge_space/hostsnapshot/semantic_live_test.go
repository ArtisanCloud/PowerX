package hostsnapshot_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	aimodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	igw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Opt-in: actual Ollama embeddings, pgvector, asynchronous worker, API Key and
// real STS Exchange. Only prefixed fixtures and one explicit development grant
// are written. Credentials are never recorded in evidence.
func TestLiveSemanticIndexAndRetrieval(t *testing.T) {
	path, base, grpcAddr := os.Getenv("POWERX_KNOWLEDGE_CONFIG"), os.Getenv("POWERX_KNOWLEDGE_HTTP"), os.Getenv("POWERX_KNOWLEDGE_GRPC")
	if path == "" || base == "" || grpcAddr == "" {
		t.Skip("set isolated Core endpoints and private config")
	}
	cfg, err := config.Load(path)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	raw, err := os.ReadFile("../../../cmd/database/seed/seed_dev_api_keys.go")
	require.NoError(t, err)
	keyMatch := regexp.MustCompile(`Key:\s*"(pxk_[^"]+)"`).FindSubmatch(raw)
	require.Len(t, keyMatch, 2)
	apiKey := string(keyMatch[1])
	authorization := "ApiKey " + apiKey
	var key igw.IntegrationGatewayAPIKey
	require.NoError(t, db.Where("key_hash = ? AND status = ?", sha(apiKey), "active").First(&key).Error)
	tenant := key.TenantUUID
	// Materialize exactly the formal explicit permission; preserve existing grants.
	var permissions []iam.Permission
	require.NoError(t, db.Where("module = ? AND allow_api_key = ? AND status = ?", "knowledge_space", true, iam.PermissionStatusActive).Find(&permissions).Error)
	var retrievalGrant igw.IntegrationGatewayAPIKeyPermission
	for _, p := range permissions {
		resolved, ok := apikeypermissions.ResolvePermission(p)
		if ok && resolved.Scope == "_scope.knowledge.retrieval.read" {
			require.True(t, p.AllowAPIKey)
			var metadata map[string]any
			require.NoError(t, json.Unmarshal(p.Meta, &metadata))
			require.Equal(t, true, metadata["api_key_explicit"])
			retrievalGrant = igw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: resolved.Scope, Action: resolved.Action, ResourceType: resolved.ResourceType, ResourcePattern: resolved.ResourcePattern, PluginID: resolved.PluginID, Effect: resolved.Effect}
			break
		}
	}
	require.NotEmpty(t, retrievalGrant.Scope, "capability-seed must materialize the explicit IAM permission")
	var grantCount int64
	require.NoError(t, db.Model(&igw.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid = ? AND scope = ?", key.UUID, retrievalGrant.Scope).Count(&grantCount).Error)
	if grantCount == 0 {
		require.NoError(t, db.Create(&retrievalGrant).Error)
	}
	records := []evidenceRecord{}
	client := &http.Client{Timeout: 90 * time.Second}
	call := func(name, method, path, auth string, body any) (int, map[string]json.RawMessage) {
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)
		request, err := http.NewRequest(method, base+path, bytes.NewReader(bodyBytes))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Trace-ID", uuid.NewString())
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		require.NoError(t, err)
		parsed := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(data, &parsed), "%s: %s", name, data)
		trace := response.Header.Get("X-Trace-ID")
		if trace == "" {
			trace = request.Header.Get("X-Trace-ID")
		}
		records = append(records, evidenceRecord{Name: name, HTTP: response.StatusCode, Trace: trace, Response: data})
		return response.StatusCode, parsed
	}
	if output := os.Getenv("POWERX_KNOWLEDGE_EVIDENCE"); output != "" {
		defer func() {
			encoded, _ := json.MarshalIndent(map[string]any{"schema": "powerx.knowledge.semantic-acceptance/v1", "http_endpoint": base, "model": "ollama/bge-m3", "records": records}, "", "  ")
			require.NoError(t, os.WriteFile(output, encoded, 0600))
		}()
	}
	status, result := call("development_grant_status", "POST", "/api/v1/tenant/capabilities:grant-status", authorization, map[string]any{"capability_ids": []string{ksvc.KnowledgeRetrievalReadCapabilityID, ksvc.KnowledgeDocumentManageCapabilityID}})
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), `"granted"`)
	pluginID := "com.powerx.acceptance.semantic." + strings.ReplaceAll(uuid.NewString(), "-", "")
	creds := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	expires := time.Now().Add(20 * time.Minute).Unix()
	cid, secret, err := creds.EnsureCredentials(context.Background(), tenant, pluginID, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{ksvc.KnowledgeDocumentManageCapabilityID, ksvc.KnowledgeRetrievalReadCapabilityID}, ExpiresAt: &expires})
	require.NoError(t, err)
	defer creds.DeleteCredentials(context.Background(), tenant, pluginID, false)
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	exchange, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{Ctx: &commonv1.RequestContext{RequestId: uuid.NewString()}, ClientId: cid, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 600})
	require.NoError(t, err)
	require.EqualValues(t, 200, exchange.GetMeta().GetCode())
	bearer := "Bearer " + exchange.GetData().GetAccessToken()
	require.NotEqual(t, "Bearer ", bearer)
	records = append(records, evidenceRecord{Name: "real_sts_exchange", HTTP: 200, Trace: exchange.GetMeta().GetRequestId(), Response: json.RawMessage(`{"issuer":"powerx-sts","audience":"powerx:api","token_redacted":true}`)})
	var template models.KnowledgeSpace
	require.NoError(t, db.Where("tenant_uuid = ? AND embedding_profile_key = ? AND active_vector_index_key <> ?", tenant, "ollama/bge-m3", "").First(&template).Error)
	var templateIndex models.KnowledgeVectorIndex
	require.NoError(t, db.Where("space_uuid = ? AND index_key = ?", template.UUID, template.ActiveVectorIndexKey).First(&templateIndex).Error)
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: pluginID, Version: 1, Status: "published", Config: []byte(`{}`)}
	require.NoError(t, db.Create(&profile).Error)
	defer db.Unscoped().Delete(&profile)
	space := models.KnowledgeSpace{TenantUUID: tenant, SpaceName: pluginID, DepartmentCode: "semantic-acceptance", Status: models.KnowledgeSpaceStatusActive, EmbeddingProfileKey: template.EmbeddingProfileKey, ActiveVectorIndexKey: template.ActiveVectorIndexKey, IngestionProfileUUID: &profile.UUID}
	require.NoError(t, db.Create(&space).Error)
	index := templateIndex
	index.PowerModel = coremodel.PowerModel{}
	index.SpaceUUID = space.UUID
	require.NoError(t, db.Create(&index).Error)
	defer func() {
		// The vector table is shared; delete only this fixture's space rows.
		require.Regexp(t, `^[a-zA-Z_][a-zA-Z0-9_]*$`, index.VectorTable)
		db.Exec(`DELETE FROM "public"."`+index.VectorTable+`" WHERE space_uuid = ?`, space.UUID)
		db.Unscoped().Where("space_uuid = ?", space.UUID).Delete(&models.HostDocumentChunk{})
		db.Unscoped().Where("space_uuid = ?", space.UUID).Delete(&models.IndexJob{})
		db.Unscoped().Where("space_uuid = ?", space.UUID).Delete(&models.TenantDocument{})
		db.Unscoped().Where("space_uuid = ?", space.UUID).Delete(&models.SemanticSpaceBinding{})
		db.Unscoped().Delete(&index)
		db.Unscoped().Delete(&space)
	}()
	root := "/api/v1/tenant/knowledge/spaces/" + space.UUID.String()
	status, result = call("semantic_configure", "POST", root+"/semantic-index", authorization, ksvc.SemanticConfigureInput{EmbeddingProfileKey: "ollama/bge-m3"})
	require.Equal(t, 200, status, "%s", result)
	var generation ksvc.SemanticGeneration
	require.NoError(t, json.Unmarshal(result["data"], &generation))
	require.Equal(t, 1024, generation.Dimensions)
	require.NotEmpty(t, generation.ModelRevision)
	defer db.Unscoped().Where("uuid = ?", generation.EmbeddingProfile.UUID).Delete(&models.SemanticEmbeddingProfile{})
	status, result = call("capabilities_before_publish", "GET", root+"/semantic-index", bearer, nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), `"indexed":false`)
	await := func(name, job string) ksvc.HostJob {
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			status, response := call(name, "GET", "/api/v1/tenant/knowledge/index-jobs/"+job, bearer, nil)
			require.Equal(t, 200, status)
			var found ksvc.HostJob
			require.NoError(t, json.Unmarshal(response["data"], &found))
			if found.Status == "succeeded" || found.Status == "failed" {
				return found
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatal("job did not reach terminal state")
		return ksvc.HostJob{}
	}
	text := strings.Repeat("品牌舆情发酵时应建立危机公关工作组，核实负面报道事实，向受影响客户公开解释，并提供后续补救步骤。", 18)
	knowledgeProfile := uuid.NewString()
	external := uuid.NewString()
	tag := uuid.NewString()
	artifact := func(role, text, sourceID string) ksvc.SemanticArtifactInput {
		kind := "cleaned_document"
		var profileUUID *string
		if role == "knowledge_profile" {
			kind = "knowledge_profile"
			profileUUID = &sourceID
		}
		return ksvc.SemanticArtifactInput{UUID: uuid.NewString(), Role: role, Text: text, Checksum: sha(text), Version: "v1", SourceRef: ksvc.SemanticSourceRef{Kind: kind, SourceUUID: sourceID, Version: "v1", Checksum: sha(text), PositionUnit: "unicode_codepoint", CharEnd: len([]rune(text))}, KnowledgeProfileUUID: profileUUID, CategoryCodes: []string{"public_relations"}, TagUUIDs: []string{tag}}
	}
	input := ksvc.HostDocumentInput{Title: "语义验收：品牌声誉与公关", URI: "powerx://" + pluginID + "/document", Content: text, ContentType: "text/plain", Checksum: sha(text), Version: "v1", IdempotencyKey: "publish-v1", ExternalRef: &ksvc.SemanticExternalRef{DocumentUUID: external}, Ingestion: &ksvc.HostIngestionSettings{Schema: ksvc.HostIngestionSnapshotSchema, ChunkSize: ref(640), ChunkOverlap: ref(80)}, Indexing: &ksvc.SemanticIndexingSettings{Mode: "hybrid", EmbeddingProfile: generation.EmbeddingProfile, ArtifactRoles: []string{"source_chunk", "knowledge_profile"}}, Artifacts: []ksvc.SemanticArtifactInput{artifact("source_chunk", text, external), artifact("knowledge_profile", "消费者质疑品牌诚信，材料给出澄清事实、承担责任、补救客户损失和持续沟通的方法。", knowledgeProfile)}}
	status, result = call("publish_real_embeddings", "POST", root+"/documents", authorization, input)
	require.Equal(t, 202, status, "%s", result)
	var accepted ksvc.HostDocumentJob
	require.NoError(t, json.Unmarshal(result["data"], &accepted))
	status, result = call("idempotent_replay", "POST", root+"/documents", bearer, input)
	require.Equal(t, 202, status)
	require.Contains(t, string(result["data"]), accepted.JobUUID)
	job := await("publish_terminal", accepted.JobUUID)
	require.Equal(t, "succeeded", job.Status, "%s", job.ErrorCode)
	require.Equal(t, 2, job.ArtifactCount)
	require.Equal(t, job.ChunkCount, job.VectorCount)
	require.Greater(t, job.VectorCount, 2)
	require.Equal(t, 640, job.EffectiveConfig.ChunkSize)
	require.Equal(t, 80, job.EffectiveConfig.ChunkOverlap)
	require.Equal(t, "hybrid", job.EffectiveConfig.IndexMode)
	var vectorCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM "public"."`+index.VectorTable+`" WHERE space_uuid = ? AND vector_dims(embedding)=1024 AND metadata->>'host_job_uuid' = ?`, space.UUID, accepted.JobUUID).Scan(&vectorCount).Error)
	require.EqualValues(t, job.VectorCount, vectorCount)
	records = append(records, evidenceRecord{Name: "real_pgvector_readback", HTTP: 200, Trace: job.TraceID, Response: json.RawMessage(`{"dimensions":1024,"vector_count":` + stringMustJSON(t, vectorCount) + `,"verified":true}`)})
	status, result = call("actual_chunks", "GET", "/api/v1/tenant/knowledge/index-jobs/"+accepted.JobUUID+"/chunks", bearer, nil)
	require.Equal(t, 200, status)
	// A distractor proves semantic ranking and filtering before LIMIT.
	unrelated := input
	unrelated.Title = "语义验收：厨房维修"
	unrelated.URI += "/unrelated"
	unrelated.Content = "厨房水管漏水时，应关闭水阀，更换密封圈，清理水槽并检测排水管道。"
	unrelated.Checksum = sha(unrelated.Content)
	unrelated.IdempotencyKey = "unrelated-v1"
	unrelated.ExternalRef = &ksvc.SemanticExternalRef{DocumentUUID: uuid.NewString()}
	unrelated.Indexing = &ksvc.SemanticIndexingSettings{Mode: "hybrid", EmbeddingProfile: generation.EmbeddingProfile, ArtifactRoles: []string{"source_chunk", "knowledge_profile"}}
	unrelated.Artifacts = []ksvc.SemanticArtifactInput{artifact("source_chunk", unrelated.Content, unrelated.ExternalRef.DocumentUUID), artifact("knowledge_profile", "本材料讨论厨房管道维修和排水问题。", uuid.NewString())}
	for i := range unrelated.Artifacts {
		unrelated.Artifacts[i].CategoryCodes = []string{"maintenance"}
	}
	status, result = call("publish_distractor", "POST", root+"/documents", authorization, unrelated)
	require.Equal(t, 202, status)
	var distractor ksvc.HostDocumentJob
	require.NoError(t, json.Unmarshal(result["data"], &distractor))
	require.Equal(t, "succeeded", await("distractor_terminal", distractor.JobUUID).Status)
	query := ksvc.SemanticQuery{Schema: ksvc.SemanticQuerySchema, Query: "最近网上出现很多负面讨论，我们希望找到能回应公众质疑并帮助恢复信誉的方法。", SpaceUUIDs: []string{space.UUID.String()}, Mode: "semantic", TopK: 20, Filters: ksvc.SemanticFilters{DocumentUUIDs: []string{accepted.DocumentUUID}, CategoryCodes: []string{"public_relations"}, TagUUIDs: []string{tag}}}
	rankingQuery := query
	rankingQuery.TopK = 1
	rankingQuery.Filters = ksvc.SemanticFilters{}
	status, result = call("semantic_synonym_ranking", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, rankingQuery)
	require.Equal(t, 200, status)
	var ranked ksvc.SemanticResult
	require.NoError(t, json.Unmarshal(result["data"], &ranked))
	require.Len(t, ranked.Items, 1)
	require.Equal(t, accepted.DocumentUUID, ranked.Items[0].DocumentUUID)
	rankingQuery.Filters.DocumentUUIDs = []string{distractor.DocumentUUID}
	status, result = call("document_filter_before_top_k", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, rankingQuery)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &ranked))
	require.Len(t, ranked.Items, 1)
	require.Equal(t, distractor.DocumentUUID, ranked.Items[0].DocumentUUID)
	status, result = call("full_natural_language_semantic_query", "POST", "/api/v1/tenant/knowledge/retrieval/query", authorization, query)
	require.Equal(t, 200, status, "%s", result)
	var semanticResult ksvc.SemanticResult
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Len(t, semanticResult.Items, job.VectorCount)
	for _, hit := range semanticResult.Items {
		require.Equal(t, accepted.DocumentUUID, hit.DocumentUUID)
		require.Equal(t, external, hit.ExternalRef.DocumentUUID)
		require.Equal(t, accepted.JobUUID, hit.BundleGeneration)
		require.Equal(t, "cosine_similarity", hit.ScoreType)
		var original string
		for _, a := range input.Artifacts {
			if a.UUID == hit.ArtifactUUID {
				original = a.Text
			}
		}
		require.Equal(t, hit.Text, string([]rune(original)[hit.SourceRef.CharStart:hit.SourceRef.CharEnd]))
		require.Equal(t, sha(original), hit.SourceRef.Checksum)
	}
	var hydrated models.HostDocumentChunk
	require.NoError(t, db.Where("space_uuid = ? AND document_uuid = ? AND job_uuid = ?", space.UUID, accepted.DocumentUUID, accepted.JobUUID).First(&hydrated).Error)
	require.NoError(t, db.Model(&hydrated).Update("content", "corrupt-private-fixture-content").Error)
	status, _ = call("source_hydration_failure_explicit", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 503, status)
	var originalChunk models.HostDocumentChunk
	// Restore from the immutable source range, not the changed model struct.
	for _, hit := range semanticResult.Items {
		if hit.ChunkUUID == hydrated.UUID.String() {
			originalChunk.Content = hit.Text
		}
	}
	require.NotEmpty(t, originalChunk.Content)
	require.NoError(t, db.Model(&hydrated).Update("content", originalChunk.Content).Error)
	// Admin JWT uses the isolated server's private signing key, never production credentials.
	var rootUser iam.User
	require.NoError(t, db.Where("is_root = ? AND status = ?", true, 1).First(&rootUser).Error)
	signUser := func(user iam.User, member iam.Member, root bool) string {
		claims := reqctx.CoreXClaims{Env: "dev", TenantUUID: tenant, UserID: user.ID, UserUUID: user.UUID.String(), MemberID: member.ID, MemberUUID: member.UUID.String(), IsRoot: root, Scope: "access", Platforms: []string{"admin"}, RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Auth.Issuer, Audience: jwt.ClaimStrings{cfg.Auth.AudienceUser}, Subject: user.UUID.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(3 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now())}}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.Auth.JWTSecret))
		require.NoError(t, err)
		return "Bearer " + token
	}
	adminAuthorization := signUser(rootUser, iam.Member{}, true)
	status, _ = call("admin_user_jwt_query", "POST", "/api/v1/admin/knowledge-spaces/retrieval/query", adminAuthorization, query)
	require.Equal(t, 200, status)
	status, _ = call("sts_cannot_use_admin_plane", "POST", "/api/v1/admin/knowledge-spaces/retrieval/query", bearer, query)
	require.Equal(t, 403, status)
	deniedUser := iam.User{Email: pluginID + "@example.invalid", DisplayName: pluginID, Status: 1}
	require.NoError(t, db.Create(&deniedUser).Error)
	defer db.Unscoped().Delete(&deniedUser)
	deniedMember := iam.Member{TenantUUID: tenant, UserUUID: deniedUser.UUID.String(), UserID: deniedUser.ID, Username: pluginID, Status: 1}
	require.NoError(t, db.Create(&deniedMember).Error)
	defer db.Unscoped().Delete(&deniedMember)
	status, _ = call("admin_missing_rbac_denied", "POST", "/api/v1/admin/knowledge-spaces/retrieval/query", signUser(deniedUser, deniedMember, false), query)
	require.Equal(t, 403, status)
	query.Mode = "hybrid"
	status, result = call("strict_hybrid_query", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.NotEmpty(t, semanticResult.Items)
	require.Equal(t, "rrf", semanticResult.Items[0].ScoreType)
	exactHybrid := query
	exactHybrid.Query = input.Artifacts[1].Text
	status, result = call("hybrid_both_sources", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, exactHybrid)
	require.Equal(t, 200, status)
	var both ksvc.SemanticResult
	require.NoError(t, json.Unmarshal(result["data"], &both))
	hasLexical := false
	for _, hit := range both.Items {
		for _, source := range hit.RetrievalSources {
			if source == "lexical" {
				hasLexical = true
			}
		}
	}
	require.True(t, hasLexical, "hybrid must execute a real lexical channel")
	query.Filters.CategoryCodes = []string{"unrelated_category"}
	query.TopK = 1
	status, result = call("filter_before_top_k", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Empty(t, semanticResult.Items)
	query.Filters.CategoryCodes = []string{"public_relations"}
	query.TopK = 20
	status, _ = call("unknown_filter_rejected", "POST", "/api/v1/tenant/knowledge/retrieval/query", authorization, map[string]any{"schema": query.Schema, "query": query.Query, "space_uuids": query.SpaceUUIDs, "mode": query.Mode, "top_k": 10, "filters": map[string]any{"sql": "1=1"}})
	require.Equal(t, 400, status)
	status, _ = call("tenant_override_rejected", "POST", "/api/v1/tenant/knowledge/retrieval/query?tenant_uuid="+uuid.NewString(), authorization, query)
	require.Equal(t, 400, status)
	bad := input
	bad.IdempotencyKey = "bad-model"
	bad.Indexing = &ksvc.SemanticIndexingSettings{Mode: "hybrid", EmbeddingProfile: ksvc.HostTaskProfileRef{UUID: uuid.NewString(), Version: 1}, ArtifactRoles: input.Indexing.ArtifactRoles}
	status, _ = call("unbound_profile_rejected", "POST", root+"/documents", bearer, bad)
	require.Equal(t, 409, status)
	bad = input
	bad.IdempotencyKey = "publish-v1"
	bad.Title = "different request"
	status, _ = call("idempotency_conflict", "POST", root+"/documents", bearer, bad)
	require.Equal(t, 409, status)
	// Corrupt only a private fixture's accepted snapshot, before the worker can claim.
	var failed ksvc.HostDocumentJob
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		failed, err = ksvc.NewHostContractService(tx).RebuildDocument(context.Background(), tenant, space.UUID.String(), accepted.DocumentUUID, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildReuseSnapshot, IdempotencyKey: "failed-rebuild"})
		if err != nil {
			return err
		}
		return tx.Model(&models.IndexJob{}).Where("uuid = ?", failed.JobUUID).Update("config_snapshot", []byte(`{"corrupt":true}`)).Error
	}))
	failedJob := await("failed_bundle_publication", failed.JobUUID)
	require.Equal(t, "failed", failedJob.Status)
	require.Equal(t, ksvc.KnowledgeReasonSnapshotInvalid, failedJob.ErrorCode)
	status, result = call("failed_bundle_old_version_query", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), accepted.JobUUID)
	status, result = call("single_document_rebuild", "POST", root+"/documents/"+accepted.DocumentUUID+"/indexes:rebuild", bearer, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildReuseSnapshot, IdempotencyKey: "retry"})
	require.Equal(t, 202, status)
	var rebuilt ksvc.HostDocumentJob
	require.NoError(t, json.Unmarshal(result["data"], &rebuilt))
	require.Equal(t, "succeeded", await("single_rebuild_terminal", rebuilt.JobUUID).Status)
	status, result = call("space_rebuild", "POST", root+"/indexes:rebuild", authorization, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildApplyConfig, Indexing: input.Indexing, Ingestion: &ksvc.HostIngestionSettings{Schema: ksvc.HostIngestionSnapshotSchema, ChunkSize: ref(128), ChunkOverlap: ref(16)}, IdempotencyKey: "space-v2"})
	require.Equal(t, 202, status)
	require.NoError(t, json.Unmarshal(result["data"], &rebuilt))
	require.Equal(t, "succeeded", await("space_rebuild_terminal", rebuilt.JobUUID).Status)
	status, _ = call("implicit_lexical_downgrade_rejected", "POST", root+"/documents/"+accepted.DocumentUUID+"/indexes:rebuild", bearer, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildApplyConfig, Ingestion: input.Ingestion})
	require.Equal(t, 422, status)
	status, result = call("query_before_visibility_change", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	query.RequiredGenerations = map[string]string{space.UUID.String(): semanticResult.Generations[0].CorpusGeneration}
	status, _ = call("visibility_revoke", "PATCH", root+"/documents/"+accepted.DocumentUUID+"/visibility", bearer, map[string]any{"queryable": false})
	require.Equal(t, 200, status)
	status, _ = call("stale_generation_rejected", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 409, status)
	query.RequiredGenerations = nil
	status, result = call("revoked_document_invisible", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Empty(t, semanticResult.Items)
	status, _ = call("visibility_missing_bool_rejected", "POST", "/api/v1/tenant/invocations", authorization, map[string]any{"capability_id": ksvc.KnowledgeDocumentManageCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"body": map[string]any{"operation": "set_document_visibility", "space_uuid": space.UUID.String(), "document_uuid": accepted.DocumentUUID, "visibility": map[string]any{}}}})
	require.NotEqual(t, 200, status)
	status, _ = call("visibility_restore", "PATCH", root+"/documents/"+accepted.DocumentUUID+"/visibility", authorization, map[string]any{"queryable": true})
	require.Equal(t, 200, status)
	// Real typed binding must reject a freely selected endpoint.
	status, result = call("typed_core_retrieval", "POST", "/api/v1/tenant/invocations", authorization, map[string]any{"capability_id": ksvc.KnowledgeRetrievalReadCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"body": map[string]any{"operation": "query", "query": query}}})
	require.Equal(t, 200, status, "%s", result)
	status, _ = call("free_form_proxy_rejected", "POST", "/api/v1/tenant/invocations", authorization, map[string]any{"capability_id": ksvc.KnowledgeRetrievalReadCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "GET", "endpoint": "/admin/root", "body": map[string]any{"operation": "capabilities", "space_uuid": space.UUID.String()}}})
	require.Equal(t, 400, status)
	// Read-only key proves least privilege and live revocation.
	readKeyText := "pxk_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	readKey := igw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: key.ProfileID, Name: pluginID, KeyPrefix: readKeyText[:12], KeyHash: sha(readKeyText), Status: "active"}
	require.NoError(t, db.Create(&readKey).Error)
	defer db.Unscoped().Delete(&readKey)
	legacyReadKey := iam.APIKey{TenantUUID: tenant, ProfileID: key.ProfileID, KeyHash: sha(readKeyText)}
	require.NoError(t, db.Create(&legacyReadKey).Error)
	defer db.Unscoped().Delete(&legacyReadKey)
	readGrant := retrievalGrant
	readGrant.PowerUUIDModel = coremodel.PowerUUIDModel{}
	readGrant.APIKeyUUID = readKey.UUID
	require.NoError(t, db.Create(&readGrant).Error)
	defer db.Unscoped().Delete(&readGrant)
	status, _ = call("read_only_key_query", "POST", "/api/v1/tenant/knowledge/retrieval/query", "ApiKey "+readKeyText, query)
	require.Equal(t, 200, status, "%s", records[len(records)-1].Response)
	status, _ = call("read_only_key_write_denied", "POST", root+"/documents", "ApiKey "+readKeyText, input)
	require.Equal(t, 403, status)
	require.NoError(t, db.Delete(&readGrant).Error)
	status, _ = call("revoked_live_api_key_grant", "POST", "/api/v1/tenant/knowledge/retrieval/query", "ApiKey "+readKeyText, query)
	require.Equal(t, 403, status)
	foreignSpace := models.KnowledgeSpace{TenantUUID: uuid.NewString(), SpaceName: pluginID, DepartmentCode: "semantic-acceptance", Status: models.KnowledgeSpaceStatusActive}
	require.NoError(t, db.Create(&foreignSpace).Error)
	defer db.Unscoped().Delete(&foreignSpace)
	query.SpaceUUIDs = []string{foreignSpace.UUID.String()}
	status, _ = call("cross_tenant_query", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 404, status)
	query.SpaceUUIDs = []string{space.UUID.String()}
	// Model/store failure stays explicit; neither path falls back to LIKE.
	require.NoError(t, db.Model(&models.SemanticEmbeddingProfile{}).Where("uuid = ?", generation.EmbeddingProfile.UUID).Update("model_revision", "invalid-fixture-revision").Error)
	status, _ = call("model_revision_conflict", "POST", "/api/v1/tenant/knowledge/retrieval/query", authorization, query)
	require.Equal(t, 409, status)
	require.NoError(t, db.Model(&models.SemanticEmbeddingProfile{}).Where("uuid = ?", generation.EmbeddingProfile.UUID).Update("model_revision", generation.ModelRevision).Error)
	require.NoError(t, db.Model(&index).Update("table_name", "semantic_acceptance_missing_table").Error)
	status, _ = call("vector_channel_failure_no_fallback", "POST", "/api/v1/tenant/knowledge/retrieval/query", authorization, query)
	require.Equal(t, 503, status)
	require.NoError(t, db.Model(&index).Update("table_name", templateIndex.VectorTable).Error)
	// Stage a real second model; old generation stays queryable until an atomic
	// whole-space rebuild publishes every document under the new binding.
	var nextModel aimodel.AIModelProfile
	lookupErr := db.Where("tenant_uuid = ? AND env = ? AND modality = ? AND provider = ? AND model = ?", tenant, "dev", "embedding", "ollama", "bge-large").First(&nextModel).Error
	if lookupErr != nil {
		require.ErrorIs(t, lookupErr, gorm.ErrRecordNotFound)
		require.NoError(t, db.Where("tenant_uuid = ? AND env = ? AND modality = ? AND provider = ? AND model = ?", tenant, "dev", "embedding", "ollama", "bge-m3").First(&nextModel).Error)
		nextModel.PowerModel = coremodel.PowerModel{}
		nextModel.Model = "bge-large"
		require.NoError(t, db.Create(&nextModel).Error)
		defer db.Unscoped().Delete(&nextModel)
	}
	newIndex := templateIndex
	newIndex.PowerModel = coremodel.PowerModel{}
	newIndex.SpaceUUID = space.UUID
	newIndex.IndexKey = "dense_semantic_acceptance_" + uuid.NewString()
	newIndex.EmbeddingModel = "bge-large"
	newIndex.EmbeddingProfileRef = "ollama/bge-large"
	require.NoError(t, db.Create(&newIndex).Error)
	defer db.Unscoped().Delete(&newIndex)
	require.NoError(t, db.Model(&space).Updates(map[string]any{"embedding_profile_key": "ollama/bge-large", "active_vector_index_key": newIndex.IndexKey}).Error)
	status, result = call("stage_new_real_model", "POST", root+"/semantic-index", authorization, ksvc.SemanticConfigureInput{EmbeddingProfileKey: "ollama/bge-large", ExpectedConfigurationGeneration: generation.ConfigurationGeneration})
	require.Equal(t, 200, status, "%s", result)
	var pending ksvc.SemanticGeneration
	require.NoError(t, json.Unmarshal(result["data"], &pending))
	require.NotEqual(t, generation.ConfigurationGeneration, pending.ConfigurationGeneration)
	require.Equal(t, 1024, pending.Dimensions)
	defer db.Unscoped().Where("uuid = ?", pending.EmbeddingProfile.UUID).Delete(&models.SemanticEmbeddingProfile{})
	status, result = call("old_model_query_while_pending", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Equal(t, generation.ConfigurationGeneration, semanticResult.Generations[0].ConfigurationGeneration)
	status, result = call("capabilities_pending_model", "GET", root+"/semantic-index", bearer, nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), pending.ConfigurationGeneration)
	require.Contains(t, string(result["data"]), generation.ConfigurationGeneration)
	newSettings := *input.Indexing
	newSettings.EmbeddingProfile = pending.EmbeddingProfile
	status, result = call("new_model_context_overflow_rejected", "POST", root+"/documents/"+accepted.DocumentUUID+"/indexes:rebuild", bearer, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildApplyConfig, Indexing: &newSettings, Ingestion: input.Ingestion, IdempotencyKey: "model-context-overflow"})
	require.Equal(t, 202, status)
	require.NoError(t, json.Unmarshal(result["data"], &rebuilt))
	overflowJob := await("model_context_overflow_terminal", rebuilt.JobUUID)
	require.Equal(t, "failed", overflowJob.Status)
	require.Equal(t, "KNOWLEDGE_EMBEDDING_FAILED", overflowJob.ErrorCode)
	require.Zero(t, overflowJob.VectorCount)
	boundedModelChunking := &ksvc.HostIngestionSettings{Schema: ksvc.HostIngestionSnapshotSchema, ChunkSize: ref(128), ChunkOverlap: ref(16)}
	status, result = call("single_document_cannot_promote_space_model", "POST", root+"/documents/"+accepted.DocumentUUID+"/indexes:rebuild", bearer, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildApplyConfig, Indexing: &newSettings, Ingestion: boundedModelChunking, IdempotencyKey: "single-model-conflict"})
	require.Equal(t, 202, status)
	require.NoError(t, json.Unmarshal(result["data"], &rebuilt))
	singleModelJob := await("single_model_migration_terminal", rebuilt.JobUUID)
	require.Equal(t, "failed", singleModelJob.Status)
	require.Equal(t, "KNOWLEDGE_MODEL_SPACE_REBUILD_REQUIRED", singleModelJob.ErrorCode)
	status, result = call("failed_model_rebuild_keeps_old_generation", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Equal(t, generation.ConfigurationGeneration, semanticResult.Generations[0].ConfigurationGeneration)
	status, result = call("atomic_space_model_rebuild", "POST", root+"/indexes:rebuild", authorization, ksvc.HostRebuildInput{Mode: ksvc.HostRebuildApplyConfig, Indexing: &newSettings, Ingestion: boundedModelChunking, IdempotencyKey: "space-model-migration"})
	require.Equal(t, 202, status)
	require.NoError(t, json.Unmarshal(result["data"], &rebuilt))
	newModelJob := await("atomic_space_model_terminal", rebuilt.JobUUID)
	require.Equal(t, "succeeded", newModelJob.Status, "%s", newModelJob.ErrorCode)
	require.Equal(t, 2, newModelJob.DocumentCount)
	require.Equal(t, newModelJob.ChunkCount, newModelJob.VectorCount)
	status, result = call("query_after_atomic_model_publish", "POST", "/api/v1/tenant/knowledge/retrieval/query", bearer, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Equal(t, pending.ConfigurationGeneration, semanticResult.Generations[0].ConfigurationGeneration)
	require.Equal(t, "ollama/bge-large", semanticResult.Generations[0].ModelKey)
	require.NotEmpty(t, semanticResult.Items)
	for _, hit := range semanticResult.Items {
		require.Equal(t, rebuilt.JobUUID, hit.BundleGeneration)
	}
	status, result = call("replay_after_model_change", "POST", root+"/documents", authorization, input)
	require.Equal(t, 202, status)
	require.Contains(t, string(result["data"]), accepted.JobUUID)
	status, result = call("caps_after_model_promotion", "GET", root+"/semantic-index", bearer, nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(result["data"]), `"pending_generation":null`)
	status, result = call("delete_document", "DELETE", root+"/documents/"+accepted.DocumentUUID, bearer, nil)
	require.Equal(t, 202, status)
	var deletion ksvc.HostDocumentJob
	require.NoError(t, json.Unmarshal(result["data"], &deletion))
	status, result = call("deleted_document_invisible", "POST", "/api/v1/tenant/knowledge/retrieval/query", authorization, query)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(result["data"], &semanticResult))
	require.Empty(t, semanticResult.Items)
	require.Equal(t, "succeeded", await("delete_terminal", deletion.JobUUID).Status)
	t.Logf("real embedding/pgvector/API Key/STS verified; artifacts=%d vectors=%d chunks=%d evidence_records=%d", job.ArtifactCount, job.VectorCount, job.ChunkCount, len(records))
}
func stringMustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}
