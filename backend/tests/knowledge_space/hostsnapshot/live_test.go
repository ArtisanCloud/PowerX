package hostsnapshot_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
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

func sha(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }
func ref[T any](value T) *T  { return &value }

type evidenceRecord struct {
	Name     string          `json:"name"`
	HTTP     int             `json:"http_status"`
	Trace    string          `json:"trace_id"`
	Response json.RawMessage `json:"response"`
}

// 真实 HTTP/API Key 和真实 gRPC STS Exchange 验收，凭据不写入证据。
// 测试创建有明确前缀的临时业务夹具与限权服务凭据，结束后删除夹具及凭据。
func TestLiveDocumentConfigurationSnapshot(t *testing.T) {
	path, base, grpcAddr := os.Getenv("POWERX_KNOWLEDGE_CONFIG"), os.Getenv("POWERX_KNOWLEDGE_HTTP"), os.Getenv("POWERX_KNOWLEDGE_GRPC")
	if path == "" || base == "" || grpcAddr == "" {
		t.Skip("set private config, isolated HTTP and STS gRPC endpoints")
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
	match := regexp.MustCompile(`Key:\s*"(pxk_[^"]+)"`).FindSubmatch(raw)
	require.Len(t, match, 2)
	apiKey := string(match[1])
	var key igw.IntegrationGatewayAPIKey
	require.NoError(t, db.Where("key_hash = ? AND status = ?", sha(apiKey), "active").First(&key).Error)
	tenant := key.TenantUUID
	var records []evidenceRecord
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(name, method, path, authorization string, body any) (int, map[string]json.RawMessage) {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
		require.NoError(t, err)
		trace := uuid.NewString()
		req.Header.Set("X-Trace-ID", trace)
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		response, err := client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		result, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		require.NoError(t, err)
		var parsed map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(result, &parsed))
		records = append(records, evidenceRecord{name, response.StatusCode, trace, result})
		return response.StatusCode, parsed
	}
	if output := os.Getenv("POWERX_KNOWLEDGE_EVIDENCE"); output != "" {
		defer func() {
			raw, _ := json.MarshalIndent(map[string]any{"schema": "powerx.knowledge.snapshot-acceptance/v1", "http_endpoint": base, "records": records}, "", "  ")
			_ = os.WriteFile(output, raw, 0o600)
		}()
	}
	code, response := call("api_key_grant_status", "POST", "/api/v1/tenant/capabilities:grant-status", "ApiKey "+apiKey, map[string]any{"capability_ids": []string{ksvc.KnowledgeDocumentManageCapabilityID}})
	require.Equal(t, 200, code)
	require.Contains(t, string(response["data"]), `"granted"`)
	pluginID := "com.powerx.acceptance.snapshot." + strings.ReplaceAll(uuid.NewString(), "-", "")
	credentials := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	expiry := time.Now().Add(15 * time.Minute).Unix()
	cid, secret, err := credentials.EnsureCredentials(context.Background(), tenant, pluginID, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{ksvc.KnowledgeDocumentManageCapabilityID, ksvc.KnowledgeSearchReadCapabilityID}, ExpiresAt: &expiry})
	require.NoError(t, err)
	defer credentials.DeleteCredentials(context.Background(), tenant, pluginID, false)
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	exchange, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{Ctx: &commonv1.RequestContext{RequestId: uuid.NewString()}, ClientId: cid, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 600})
	require.NoError(t, err)
	require.EqualValues(t, 200, exchange.GetMeta().GetCode())
	bearer := "Bearer " + exchange.GetData().GetAccessToken()
	require.NotEqual(t, "Bearer ", bearer)
	records = append(records, evidenceRecord{Name: "real_sts_exchange", HTTP: int(exchange.GetMeta().GetCode()), Trace: exchange.GetMeta().GetRequestId(), Response: json.RawMessage(`{"issuer":"powerx-sts","audience":"powerx:api","token_redacted":true}`)})
	profile := models.IngestionProfileVersion{TenantUUID: tenant, ProfileKey: pluginID, Version: 7, Status: models.ProfileStatusPublished, DisplayName: "snapshot acceptance", Config: datatypes.JSON(`{"chunking":{"chunk_size":256,"chunk_overlap":16}}`)}
	require.NoError(t, db.Create(&profile).Error)
	defer db.Unscoped().Delete(&profile)
	space := models.KnowledgeSpace{TenantUUID: tenant, SpaceName: pluginID, DepartmentCode: "snapshot-acceptance", Status: models.KnowledgeSpaceStatusActive, IngestionProfileUUID: &profile.UUID}
	require.NoError(t, db.Create(&space).Error)
	defer func() {
		db.Unscoped().Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Delete(&models.HostDocumentChunk{})
		db.Unscoped().Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Delete(&models.IndexJob{})
		db.Unscoped().Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Delete(&models.TenantDocument{})
		db.Unscoped().Delete(&space)
	}()
	content := strings.Repeat("甲乙丙丁戊己庚辛壬癸", 200)
	settings := ksvc.HostIngestionSettings{Schema: ksvc.HostIngestionSnapshotSchema, IngestionProfile: &ksvc.HostTaskProfileRef{UUID: profile.UUID.String(), Version: 7}, ChunkSize: ref(640), ChunkOverlap: ref(80), PagePriority: ref(false), Separators: &[]string{}}
	input := ksvc.HostDocumentInput{Title: "640/80 acceptance", Content: content, ContentType: "text/plain", Checksum: sha(content), Version: "v1", Ingestion: &settings}
	var acceptedJobs []string
	for _, mode := range []struct{ name, auth string }{{"api_key", "ApiKey " + apiKey}, {"sts", bearer}} {
		input.URI = "powerx://acceptance/" + pluginID + "/" + mode.name
		code, response = call(mode.name+"_submit", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", mode.auth, input)
		require.Equal(t, 202, code)
		var accepted ksvc.HostDocumentJob
		require.NoError(t, json.Unmarshal(response["data"], &accepted))
		acceptedJobs = append(acceptedJobs, accepted.JobUUID)
		var job ksvc.HostJob
		require.Eventually(t, func() bool {
			status, data := call(mode.name+"_job", "GET", "/api/v1/tenant/knowledge/index-jobs/"+accepted.JobUUID, mode.auth, nil)
			if status != 200 {
				return false
			}
			_ = json.Unmarshal(data["data"], &job)
			return job.Status == "succeeded" || job.Status == "failed"
		}, 10*time.Second, 100*time.Millisecond)
		require.Equal(t, "succeeded", job.Status)
		require.Equal(t, 640, job.EffectiveConfig.ChunkSize)
		require.Equal(t, 80, job.EffectiveConfig.ChunkOverlap)
		require.Equal(t, 7, job.EffectiveConfig.IngestionProfile.Version)
		require.Equal(t, profile.UUID.String(), job.EffectiveConfig.IngestionProfile.UUID)
		code, response = call(mode.name+"_chunks", "GET", "/api/v1/tenant/knowledge/index-jobs/"+accepted.JobUUID+"/chunks", mode.auth, nil)
		require.Equal(t, 200, code)
		var chunks struct {
			Items []ksvc.HostChunk `json:"items"`
		}
		require.NoError(t, json.Unmarshal(response["data"], &chunks))
		require.Len(t, chunks.Items, 4)
		for i, ch := range chunks.Items {
			require.LessOrEqual(t, utf8.RuneCountInString(ch.Content), 640)
			require.Equal(t, sha(ch.Content), ch.Checksum)
			if i > 0 {
				a, b := []rune(chunks.Items[i-1].Content), []rune(ch.Content)
				require.Equal(t, string(a[len(a)-80:]), string(b[:80]))
			}
		}
	}
	// 固定 typed service plane 不允许被网关注入原始 HTTP header。
	for _, mode := range []struct{ name, auth string }{{"api_key", "ApiKey " + apiKey}, {"sts", bearer}} {
		status, data := call(mode.name+"_typed_job", "POST", "/api/v1/tenant/invocations", mode.auth, map[string]any{"capability_id": ksvc.KnowledgeDocumentManageCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "INVOKE", "endpoint": "core://knowledge/documents", "body": map[string]any{"operation": "get_job", "job_uuid": acceptedJobs[0]}}})
		require.Equal(t, 200, status)
		require.Contains(t, string(data["data"]), acceptedJobs[0])
	}
	deniedPlugin := pluginID + ".denied"
	deniedID, deniedSecret, err := credentials.EnsureCredentials(context.Background(), tenant, deniedPlugin, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{ksvc.KnowledgeCatalogReadCapabilityID}, ExpiresAt: &expiry})
	require.NoError(t, err)
	defer credentials.DeleteCredentials(context.Background(), tenant, deniedPlugin, false)
	deniedExchange, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{ClientId: deniedID, ClientSecret: deniedSecret, Audience: "powerx:api", Scope: "access", TtlSeconds: 600})
	require.NoError(t, err)
	require.EqualValues(t, 200, deniedExchange.GetMeta().GetCode())
	code, _ = call("sts_missing_document_grant", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", "Bearer "+deniedExchange.GetData().GetAccessToken(), input)
	require.Equal(t, 403, code)
	code, _ = call("unknown_top_level_setting", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", "ApiKey "+apiKey, map[string]any{"title": "x", "uri": "powerx://unknown", "content": "x", "content_type": "text/plain", "checksum": sha("x"), "version": "v1", "chunk_size": 640})
	require.Equal(t, 400, code)
	for _, tc := range []struct {
		name   string
		change func(*ksvc.HostIngestionSettings)
		status int
		reason string
	}{
		{"bad_overlap", func(v *ksvc.HostIngestionSettings) { v.ChunkOverlap = ref(640) }, 400, ksvc.KnowledgeReasonInvalidArgument},
		{"bad_order", func(v *ksvc.HostIngestionSettings) { v.SegmentOrder = &[]string{"size", "size", "segment", "unknown"} }, 400, ksvc.KnowledgeReasonInvalidArgument},
		{"unsupported_mode", func(v *ksvc.HostIngestionSettings) { v.SegmentMode = ref("semantic") }, 422, ksvc.KnowledgeReasonUnsupportedSetting},
		{"expired_version", func(v *ksvc.HostIngestionSettings) {
			v.IngestionProfile = &ksvc.HostTaskProfileRef{UUID: profile.UUID.String(), Version: 99}
		}, 422, ksvc.KnowledgeReasonProfileUnavailable},
	} {
		copy := settings
		tc.change(&copy)
		input.Ingestion = &copy
		input.URI = "powerx://acceptance/" + pluginID + "/" + tc.name
		code, response = call(tc.name, "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", "ApiKey "+apiKey, input)
		require.Equal(t, tc.status, code)
		whole, _ := json.Marshal(response)
		require.Contains(t, string(whole), tc.reason)
	}
	input.Ingestion = &settings
	foreignTenant := uuid.NewString()
	foreignProfile := models.IngestionProfileVersion{TenantUUID: foreignTenant, ProfileKey: pluginID, Version: 7, Status: models.ProfileStatusPublished, Config: datatypes.JSON(`{}`)}
	require.NoError(t, db.Create(&foreignProfile).Error)
	defer db.Unscoped().Delete(&foreignProfile)
	foreignSpace := models.KnowledgeSpace{TenantUUID: foreignTenant, SpaceName: pluginID, DepartmentCode: "snapshot-acceptance", Status: models.KnowledgeSpaceStatusActive}
	require.NoError(t, db.Create(&foreignSpace).Error)
	defer db.Unscoped().Delete(&foreignSpace)
	code, _ = call("cross_tenant_space", "POST", "/api/v1/tenant/knowledge/spaces/"+foreignSpace.UUID.String()+"/documents", "ApiKey "+apiKey, input)
	require.Equal(t, 404, code)
	copy := settings
	copy.IngestionProfile = &ksvc.HostTaskProfileRef{UUID: foreignProfile.UUID.String(), Version: 7}
	input.Ingestion = &copy
	code, response = call("cross_tenant_profile", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", bearer, input)
	require.Equal(t, 422, code)
	body, _ := json.Marshal(response)
	require.Contains(t, string(body), ksvc.KnowledgeReasonProfileUnavailable)
	input.Ingestion = &settings
	code, _ = call("no_auth", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", "", input)
	require.Equal(t, 401, code)
	// 验证不可变性只修改本测试创建的空间和 Profile，不修改用户资料。
	code, response = call("snapshot_before_change", "GET", "/api/v1/tenant/knowledge/index-jobs/"+acceptedJobs[0], bearer, nil)
	require.Equal(t, 200, code)
	before := response["data"]
	require.NoError(t, db.Model(&profile).Updates(map[string]any{"config": datatypes.JSON(`{"chunking":{"chunk_size":128}}`), "status": models.ProfileStatusArchived}).Error)
	require.NoError(t, db.Model(&space).Update("ingestion_profile_uuid", nil).Error)
	code, response = call("snapshot_after_change", "GET", "/api/v1/tenant/knowledge/index-jobs/"+acceptedJobs[0], bearer, nil)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(before), string(response["data"]))
	input.Ingestion = &settings
	code, _ = call("archived_profile", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/documents", bearer, input)
	require.Equal(t, 422, code)
	var docs, jobs int64
	require.NoError(t, db.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Count(&docs).Error)
	require.NoError(t, db.Model(&models.IndexJob{}).Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Count(&jobs).Error)
	require.EqualValues(t, 2, docs)
	require.EqualValues(t, 2, jobs)
	if os.Getenv("POWERX_KNOWLEDGE_REBUILD_CHECK") == "1" {
		require.NoError(t, db.Model(&profile).Updates(map[string]any{"status": models.ProfileStatusPublished, "config": datatypes.JSON(`{}`)}).Error)
		require.NoError(t, db.Model(&space).Update("ingestion_profile_uuid", profile.UUID).Error)
		var docs []models.TenantDocument
		require.NoError(t, db.Where("tenant_uuid = ? AND space_uuid = ?", tenant, space.UUID).Order("uri ASC").Find(&docs).Error)
		require.Len(t, docs, 2)
		await := func(name, id, auth string) ksvc.HostJob {
			var result ksvc.HostJob
			require.Eventually(t, func() bool {
				status, body := call(name, "GET", "/api/v1/tenant/knowledge/index-jobs/"+id, auth, nil)
				if status != 200 {
					return false
				}
				_ = json.Unmarshal(body["data"], &result)
				return result.Status == "succeeded" || result.Status == "failed"
			}, 10*time.Second, 100*time.Millisecond)
			return result
		}
		oldSecond := *docs[1].ActiveIndexJobUUID
		endpoint := "/api/v1/tenant/knowledge/spaces/" + space.UUID.String() + "/documents/" + docs[0].UUID.String() + "/indexes:rebuild"
		request := map[string]any{"mode": "reuse_snapshot", "idempotency_key": "single-reuse"}
		status, body := call("single_rebuild_submit", "POST", endpoint, "ApiKey "+apiKey, request)
		require.Equal(t, 202, status)
		var accepted ksvc.HostDocumentJob
		require.NoError(t, json.Unmarshal(body["data"], &accepted))
		require.Equal(t, docs[0].UUID.String(), accepted.DocumentUUID)
		result := await("single_rebuild_job", accepted.JobUUID, "ApiKey "+apiKey)
		require.Equal(t, "succeeded", result.Status)
		require.Equal(t, 1, result.DocumentCount)
		require.Equal(t, 640, result.EffectiveConfig.ChunkSize)
		status, body = call("single_rebuild_same_key", "POST", endpoint, "ApiKey "+apiKey, request)
		require.Equal(t, 202, status)
		require.Contains(t, string(body["data"]), accepted.JobUUID)
		require.NoError(t, db.Where("uuid = ?", docs[1].UUID).First(&docs[1]).Error)
		require.Equal(t, oldSecond, *docs[1].ActiveIndexJobUUID)
		applied := map[string]any{"mode": "apply_config", "idempotency_key": "single-new-config", "ingestion": map[string]any{"schema": ksvc.HostIngestionSnapshotSchema, "ingestion_profile": map[string]any{"uuid": profile.UUID.String(), "version": 7}, "chunk_size": 320, "chunk_overlap": 40}}
		status, body = call("single_new_config_submit", "POST", endpoint, bearer, applied)
		require.Equal(t, 202, status)
		require.NoError(t, json.Unmarshal(body["data"], &accepted))
		result = await("single_new_config_job", accepted.JobUUID, bearer)
		require.Equal(t, "succeeded", result.Status)
		require.Equal(t, 320, result.EffectiveConfig.ChunkSize)
		require.Greater(t, result.ChunkCount, 4)
		applied["idempotency_key"] = "whole-space"
		status, body = call("space_reprocess_submit", "POST", "/api/v1/tenant/knowledge/spaces/"+space.UUID.String()+"/indexes:rebuild", bearer, applied)
		require.Equal(t, 202, status)
		require.NoError(t, json.Unmarshal(body["data"], &accepted))
		result = await("space_reprocess_job", accepted.JobUUID, bearer)
		require.Equal(t, "succeeded", result.Status)
		require.Equal(t, 2, result.DocumentCount)
		require.Equal(t, 2, result.ProcessedDocuments)
		status, _ = call("single_rebuild_cross_tenant", "POST", "/api/v1/tenant/knowledge/spaces/"+foreignSpace.UUID.String()+"/documents/"+docs[0].UUID.String()+"/indexes:rebuild", "ApiKey "+apiKey, request)
		require.Equal(t, 404, status)
		status, _ = call("single_rebuild_invalid_config", "POST", endpoint, bearer, map[string]any{"mode": "apply_config", "idempotency_key": "invalid-rebuild", "ingestion": map[string]any{"schema": ksvc.HostIngestionSnapshotSchema, "chunk_size": 320, "chunk_overlap": 320}})
		require.Equal(t, 400, status)
		// 与接收位于同一外层数据库事务，先注入本测试任务故障再提交；
		// 真实 Worker 不会在故障注入之前抢领，不修改其他任务或服务配置。
		var failed ksvc.HostDocumentJob
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			service := ksvc.NewHostContractService(tx)
			var err error
			failed, err = service.RebuildDocument(context.Background(), tenant, space.UUID.String(), docs[0].UUID.String(), ksvc.HostRebuildInput{Mode: ksvc.HostRebuildReuseSnapshot, IdempotencyKey: "failure-then-retry"})
			if err != nil {
				return err
			}
			return tx.Model(&models.IndexJob{}).Where("uuid = ? AND status = ?", failed.JobUUID, "queued").Update("config_snapshot", datatypes.JSON(`{"corrupt":true}`)).Error
		}))
		{
			result = await("rebuild_failed_job", failed.JobUUID, bearer)
			require.Equal(t, "failed", result.Status)
			require.NoError(t, db.Where("uuid = ?", docs[0].UUID).First(&docs[0]).Error)
			require.Equal(t, accepted.JobUUID, *docs[0].ActiveIndexJobUUID)
			status, body = call("failed_rebuild_old_index_search", "POST", "/api/v1/tenant/knowledge/search", bearer, map[string]any{"query": "甲乙", "space_uuids": []string{space.UUID.String()}, "limit": 10})
			require.Equal(t, 200, status)
			require.Contains(t, string(body["data"]), docs[0].UUID.String())
			status, body = call("failed_rebuild_retry", "POST", endpoint, bearer, map[string]any{"mode": "reuse_snapshot", "idempotency_key": "retry-after-failure"})
			require.Equal(t, 202, status)
			require.NoError(t, json.Unmarshal(body["data"], &accepted))
			result = await("failed_rebuild_retry_job", accepted.JobUUID, bearer)
			require.Equal(t, "succeeded", result.Status)
		}
	}
	t.Logf("real API Key + issued STS passed; jobs=%d documents=%d records=%d; immutable 640/80 chunks verified", jobs, docs, len(records))
}
