package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/autoseed"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

// GET /api/.../admin/plugins/:id/tenant_config
func PluginTenantConfigHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		svc := setting.NewPluginInstanceConfigService(deps)
		cfg, err := svc.Get(c, tenantUUID, id, setting.KeyClientCredentials)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "查询失败", err)
			return
		}
		exists := cfg != nil && len(cfg.ValueJSON) > 0
		enabled := false
		clientID := ""
		if exists {
			enabled = cfg.Enabled
			var cc struct {
				ClientID string `json:"client_id"`
			}
			_ = json.Unmarshal(cfg.ValueJSON, &cc)
			clientID = cc.ClientID
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"tenant_uuid": tenantUUID,
			"plugin_id":   id,
			"exists":      exists,
			"enabled":     enabled,
			"client_id":   clientID,
		})
	}
}

type tenantEnableReq struct {
	Enabled bool           `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func TenantPluginInstanceListHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		manager := pmimpl.GetPluginManager()
		if manager == nil {
			dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "插件管理器未初始化", nil)
			return
		}
		plugins, err := manager.List(c.Request.Context())
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "加载插件失败", err)
			return
		}
		svc := pluginservice.NewTenantPluginInstanceService(deps.DB)
		items, err := svc.List(c.Request.Context(), tenantUUID, plugins)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, items)
	}
}

func TenantPluginInstanceEnableHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("plugin_id")
		if id == "" {
			id = c.Param("id")
		}
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		var req tenantEnableReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}

		manager := pmimpl.GetPluginManager()
		if manager == nil {
			dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "插件管理器未初始化", nil)
			return
		}
		p, err := manager.Get(c.Request.Context(), id)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusNotFound, "插件不存在", err)
			return
		}
		if string(p.State) != "enabled" {
			dtoRequest.ResponseError(c, http.StatusConflict, "全局插件包未启用", fmt.Errorf("plugin %s state=%s", p.ID, p.State))
			return
		}

		svc := pluginservice.NewTenantPluginInstanceService(deps.DB)
		instance, clientID, clientSecret, err := svc.Enable(c.Request.Context(), tenantUUID, p, req.Config)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		if err := ensureTenantEventFabricTopics(c, deps, tenantUUID, id); err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "启用失败：Topic 注册失败", err)
			return
		}
		if clientSecret != "" {
			_ = pmimplnotify.PushTenantCredentials(c, id, tenantUUID, clientID, clientSecret)
		}
		proc, _ := pmimpl.TryRuntimeStatus(manager, id)
		out := gin.H{"instance": instance, "enabled": true, "client_id": clientID, "just_issued": clientSecret != ""}
		out["runtime_scope"] = gin.H{
			"scope":             "global_plugin_process",
			"tenant_isolated":   false,
			"shared_by_tenants": true,
			"process_id":        id,
			"pid":               proc.PID,
		}
		if clientSecret != "" {
			out["client_secret"] = clientSecret
		}
		dtoRequest.ResponseSuccess(c, out)
	}
}

func TenantPluginInstanceDisableHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("plugin_id")
		if id == "" {
			id = c.Param("id")
		}
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		svc := pluginservice.NewTenantPluginInstanceService(deps.DB)
		instance, err := svc.Disable(c.Request.Context(), tenantUUID, id)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		manager := pmimpl.GetPluginManager()
		proc, _ := pmimpl.TryRuntimeStatus(manager, id)
		dtoRequest.ResponseSuccess(c, gin.H{
			"instance": instance,
			"enabled":  false,
			"runtime_scope": gin.H{
				"scope":             "global_plugin_process",
				"tenant_isolated":   false,
				"shared_by_tenants": true,
				"process_id":        id,
				"pid":               proc.PID,
			},
		})
	}
}

// POST /api/.../admin/plugins/:id/tenant_enable
// body: {"enabled": true}
func PluginTenantEnableHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}
		if _, ok := tenantUUIDFromGin(c); !ok {
			return
		}
		var req tenantEnableReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		if req.Enabled {
			TenantPluginInstanceEnableHandler(deps)(c)
			return
		}
		TenantPluginInstanceDisableHandler(deps)(c)
	}
}

// GET /api/.../admin/plugins/:id/credentials
// 返回只读信息：client_id、是否存在、是否启用；不返回明文 secret
func PluginCredentialGetHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		svc := setting.NewPluginInstanceConfigService(deps)
		cfg, err := svc.Get(c, tenantUUID, id, setting.KeyClientCredentials)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "查询失败", err)
			return
		}
		resp := gin.H{"tenant_uuid": tenantUUID, "plugin_id": id, "exists": false, "enabled": false, "client_id": ""}
		if cfg != nil && len(cfg.ValueJSON) > 0 {
			resp["exists"] = true
			resp["enabled"] = cfg.Enabled
			var cc struct {
				ClientID string `json:"client_id"`
			}
			_ = json.Unmarshal(cfg.ValueJSON, &cc)
			resp["client_id"] = cc.ClientID
		}
		dtoRequest.ResponseSuccess(c, resp)
	}
}

// POST /api/.../admin/plugins/:id/credentials/rotate
// 轮换并一次性返回新的明文 secret
func PluginCredentialRotateHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		svc := setting.NewPluginInstanceConfigService(deps)
		// 先读取 client_id 以便响应
		cfg, err := svc.Get(c, tenantUUID, id, setting.KeyClientCredentials)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "查询失败", err)
			return
		}
		if cfg == nil || len(cfg.ValueJSON) == 0 {
			dtoRequest.ResponseError(c, 404, "未找到凭证记录", nil)
			return
		}
		var cc struct {
			ClientID string `json:"client_id"`
		}
		_ = json.Unmarshal(cfg.ValueJSON, &cc)

		secret, err := svc.RotateSecret(c, tenantUUID, id)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "轮换失败", err)
			return
		}
		// 可选：保持 enabled 状态不变
		_ = tryKeepEnabled(c, svc, cfg)

		// 尝试通过 gRPC 下发到插件（最佳努力）
		_ = pmimplnotify.PushTenantCredentials(c, id, tenantUUID, cc.ClientID, secret)

		dtoRequest.ResponseSuccess(c, gin.H{
			"client_id":     cc.ClientID,
			"client_secret": secret, // 明文只此一次
		})
	}
}

// 辅助：轮换后保留 enabled 状态（若需要）
func tryKeepEnabled(c *gin.Context, svc *setting.PluginInstanceConfigService, old *dbsetting.PluginInstanceConfig) error {
	if old == nil {
		return nil
	}
	if old.Enabled {
		return svc.SetEnabled(c, old.TenantUUID, old.PluginID, true)
	}
	return nil
}

func tenantUUIDFromGin(c *gin.Context) (string, bool) {
	value, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return "", false
	}
	canonical, err := reqctx.CanonicalTenantUUID(value)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "tenant_uuid 格式错误", err)
		return "", false
	}
	return canonical, true
}

func ensureTenantEventFabricTopics(c *gin.Context, deps *shared.Deps, tenantUUID, pluginID string) error {
	if deps == nil || deps.EventFabric == nil || deps.EventFabric.Seeder == nil {
		return nil
	}
	manager := pmimpl.GetPluginManager()
	if manager == nil {
		return fmt.Errorf("plugin manager is not initialized")
	}
	plugin, err := manager.Get(c.Request.Context(), pluginID)
	if err != nil {
		return fmt.Errorf("load plugin %s failed: %w", pluginID, err)
	}

	manifestPath, err := autoseed.ResolveManifestPath(plugin)
	if err != nil {
		return fmt.Errorf("resolve event manifest failed: %w", err)
	}
	if manifestPath == "" {
		logger.DebugF(c.Request.Context(), "[plugin-tenant-enable] no event_fabric manifest found plugin=%s tenant=%s", pluginID, tenantUUID)
		return nil
	}

	file, err := os.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open manifest %s failed: %w", manifestPath, err)
	}
	defer file.Close()

	doc, err := manifest.Parse(file)
	if err != nil {
		return fmt.Errorf("parse manifest %s failed: %w", manifestPath, err)
	}

	seedCtx := manifest.SeedContext{
		TenantUUID:    tenantUUID,
		PluginID:      plugin.ID,
		PluginVersion: plugin.Version,
		Operator:      "plugin-tenant-enable",
		Variables:     autoseed.BuildSeedVariables(plugin),
	}
	if _, err := deps.EventFabric.Seeder.ApplyManifest(c.Request.Context(), doc, seedCtx); err != nil {
		return fmt.Errorf("seed event_fabric manifest failed: %w", err)
	}
	logger.InfoF(c.Request.Context(), "[plugin-tenant-enable] event_fabric seeded plugin=%s tenant=%s manifest=%s", pluginID, tenantUUID, manifestPath)
	return nil
}

// DELETE /api/.../admin/plugins/:id/tenant_config
// 彻底删除当前租户下该插件的配置（包含凭证），默认硬删
func PluginTenantDeleteHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		svc := setting.NewPluginInstanceConfigService(deps)
		// 确认是否存在
		cfg, err := svc.Get(c, tenantUUID, id, setting.KeyClientCredentials)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "查询失败", err)
			return
		}
		if cfg == nil {
			dtoRequest.ResponseError(c, 404, "记录不存在", nil)
			return
		}
		if err := svc.DeleteCredentials(c, tenantUUID, id, false); err != nil { // 硬删除
			dtoRequest.ResponseError(c, 500, "删除失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"ok":          true,
			"deleted":     true,
			"tenant_uuid": tenantUUID,
			"plugin_id":   id,
		})
	}
}
