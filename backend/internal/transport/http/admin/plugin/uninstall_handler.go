package plugin

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// --- Uninstall ---
// POST /api/admin/plugins/:id/uninstall
// body: {"version":"0.1.1","purge":true}
type uninstallReq struct {
	Version string `json:"version"` // 为空则卸载 current
	Purge   bool   `json:"purge"`   // 是否连同磁盘产物清理
}

func PluginUninstallHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req uninstallReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
			return
		}

		mgr, err := tryGetPluginManager()
		ctx := c.Request.Context()
		if deps != nil && deps.DB != nil {
			drainSvc := pluginservice.NewPluginDrainJobService(deps.DB)
			if _, drainErr := drainSvc.RequireNoActiveTenantInstances(ctx, id, req.Version); drainErr != nil {
				dtoRequest.RespondErrorFrom(c, drainErr)
				return
			}
		}
		if err != nil {
			if fallbackErr := uninstallFromRegistry(ctx, id, req.Version, req.Purge); fallbackErr != nil {
				respondPluginRuntimeUnavailable(c, fallbackErr)
				return
			}
			dtoRequest.ResponseSuccess(c, gin.H{
				"id":       id,
				"version":  req.Version,
				"purge":    req.Purge,
				"fallback": true,
			})
			return
		}

		if req.Purge {
			if req.Version != "" {
				err = mgr.UninstallAndPurge(ctx, id, req.Version)
			} else {
				err = mgr.UninstallAndPurge(ctx, id)
			}
		} else {
			if req.Version != "" {
				err = mgr.Uninstall(ctx, id, req.Version)
			} else {
				err = mgr.Uninstall(ctx, id)
			}
		}
		if err != nil {
			status := plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err))
			errCode := "PLUGIN_UNINSTALL_FAILED"
			if plugin_mgr.CodeOf(err) == plugin_mgr.CodeConflict {
				status = http.StatusConflict
				errCode = pluginservice.ErrCodePluginDrainRequired
			}
			dtoRequest.ResponseErrorWithDetails(c, status, "卸载失败", err, gin.H{
				"requires_tenant_instance_cleanup": plugin_mgr.CodeOf(err) == plugin_mgr.CodeConflict,
				"requires_drain":                   plugin_mgr.CodeOf(err) == plugin_mgr.CodeConflict,
				"error_code":                       errCode,
			})
			return
		}
		if deps != nil && deps.DB != nil {
			drainSvc := pluginservice.NewPluginDrainJobService(deps.DB)
			if err := drainSvc.CompleteFinalUninstall(ctx, id); err != nil {
				dtoRequest.RespondErrorFrom(c, err)
				return
			}
		}
		pluginservice.PublishPluginUninstallStatus(ctx, id, req.Version, req.Purge)

		dtoRequest.ResponseSuccess(c, gin.H{
			"id":      id,
			"version": req.Version, // 为空=卸载当时的 current
			"purge":   req.Purge,
		})
	}
}
