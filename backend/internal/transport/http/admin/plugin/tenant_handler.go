package plugin

import (
	"encoding/json"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
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
	Enabled bool `json:"enabled"`
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
		tenantUUID, ok := tenantUUIDFromGin(c)
		if !ok {
			return
		}
		var req tenantEnableReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		svc := setting.NewPluginInstanceConfigService(deps)
		if req.Enabled {
			clientID, clientSecret, err := svc.EnsureCredentials(c, tenantUUID, id, nil)
			if err != nil {
				dtoRequest.ResponseError(c, 500, "启用失败", err)
				return
			}
			if err := svc.SetEnabled(c, tenantUUID, id, true); err != nil {
				dtoRequest.ResponseError(c, 500, "启用失败", err)
				return
			}
			// 首次创建会返回一次性明文 secret，此时尝试通过 gRPC 下发到插件（best-effort）
			if clientSecret != "" {
				_ = pmimplnotify.PushTenantCredentials(c, id, tenantUUID, clientID, clientSecret)
			}
			// 仅当首次创建时返回一次性 secret（非空）
			out := gin.H{"ok": true, "enabled": true, "client_id": clientID}
			if clientSecret != "" {
				out["just_issued"] = true
				out["client_secret"] = clientSecret
			} else {
				out["just_issued"] = false
			}
			dtoRequest.ResponseSuccess(c, out)
			return
		}
		// 停用（仅影响本租户的启用态，不影响进程与其他租户）
		if err := svc.SetEnabled(c, tenantUUID, id, false); err != nil {
			dtoRequest.ResponseError(c, 500, "停用失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "enabled": false})
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
