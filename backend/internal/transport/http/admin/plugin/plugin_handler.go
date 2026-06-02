package plugin

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	pluginDto "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"
	pluginMgr "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// 辅助：从全局配置拿 BasePrefix（/_p）
func basePrefix() string {
	if cfg := config.GetGlobalConfig(); cfg != nil && cfg.Plugin.BasePrefix != "" {
		return cfg.Plugin.BasePrefix
	}
	return "/_p"
}

// GET /api/.../admin/plugins
func PluginListHandler(c *gin.Context) {
	ctx := c.Request.Context()
	mgr, err := tryGetPluginManager()
	var list []pluginMgr.Plugin
	if err == nil {
		list, err = mgr.List(ctx)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "获取插件列表失败", err)
			return
		}
	} else {
		list, err = listPluginsFromRegistry(ctx)
		if err != nil {
			dtoRequest.ResponseSuccess(c, gin.H{"plugins": []pluginDto.PluginItem{}})
			return
		}
	}

	prefix := basePrefix()
	out := make([]pluginDto.PluginItem, 0, len(list))

	for _, p := range list {
		adminURL := ""
		hasAdmin := p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != ""
		if hasAdmin {
			adminURL = prefix + "/" + p.ID + "/admin/"
		}

		menus := make([]pluginDto.PluginMenuItem, 0, len(p.Frontend.Admin.Menus))
		for _, m := range p.Frontend.Admin.Menus {
			title := m.Title
			if m.TitleI18n != nil {
				if def := m.TitleI18n.Default; def != "" {
					title = def
				} else if title == "" {
					title = m.TitleI18n.Key
				}
			}
			menus = append(menus, pluginDto.PluginMenuItem{
				ID:    p.ID,
				Title: title,
				Icon:  m.Icon,
				Order: m.Order,
				URL:   adminURL, // 简化：统一指向根；细分路由可在此扩展
			})
		}
		sort.Slice(menus, func(i, j int) bool { return menus[i].Order < menus[j].Order })

		out = append(out, pluginDto.PluginItem{
			ID:       p.ID,
			Name:     p.ID, // 或使用 p.Manifest.Name（若你对外暴露了）
			Version:  p.Version,
			State:    string(p.State),
			AdminURL: adminURL,
			APIBase:  prefix + "/" + p.ID + "/api",
			HasAdmin: hasAdmin,
			Menus:    menus,
		})
	}

	// 启用优先、再按 ID
	sort.Slice(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State == string(pluginMgr.StateEnabled)
		}
		return out[i].ID < out[j].ID
	})

	dtoRequest.ResponseSuccess(c, gin.H{"plugins": out})
}

// POST /api/.../admin/plugins/:id/enable
func PluginEnableHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		ctx := c.Request.Context()
		tenantUUID := optionalTenantContext(c)
		if tenantUUID != "" {
			ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
		}
		if err := mgr.Enable(ctx, id); err != nil {
			dtoRequest.ResponseError(c, statusFromManagerErr(err), "启用插件失败", err)
			return
		}
		if err := completeReadyDrainJobsForPlugin(c, deps, id); err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "启用插件失败：Drain 状态恢复失败", err)
			return
		}
		if err := enablePluginForCurrentTenantIfPresent(c, deps, id); err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "启用插件失败：租户插件启用失败", err)
			return
		}
		seeded, err := ensureEnabledTenantEventFabricTopics(c, deps, id)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "启用插件失败：Topic 注册失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "event_fabric_seeded_tenants": seeded})
	}
}

func completeReadyDrainJobsForPlugin(c *gin.Context, deps *shared.Deps, pluginID string) error {
	if deps == nil || deps.DB == nil {
		return nil
	}
	_, err := reposetting.NewPluginDrainJobRepository(deps.DB).MarkReadyJobsCompletedByPlugin(c.Request.Context(), pluginID, time.Now().UTC())
	return err
}

func enablePluginForCurrentTenantIfPresent(c *gin.Context, deps *shared.Deps, pluginID string) error {
	if deps == nil || deps.DB == nil {
		return nil
	}
	tenantUUID := strings.TrimSpace(reqctx.TenantUUIDFromGin(c))
	if tenantUUID == "" {
		return nil
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return err
	}
	mgr, err := tryGetPluginManager()
	if err != nil {
		return err
	}
	p, err := mgr.Get(c.Request.Context(), pluginID)
	if err != nil {
		return err
	}
	svc := pluginservice.NewTenantPluginInstanceService(deps.DB)
	_, clientID, clientSecret, err := svc.Enable(c.Request.Context(), tenantUUID, p, nil)
	if err != nil {
		return err
	}
	if clientSecret != "" {
		_ = pmimplnotify.PushTenantCredentials(c, pluginID, tenantUUID, clientID, clientSecret)
	}
	return nil
}

func ensureEnabledTenantEventFabricTopics(c *gin.Context, deps *shared.Deps, pluginID string) (int, error) {
	if deps == nil || deps.DB == nil {
		return 0, nil
	}
	repo := reposetting.NewPluginInstanceConfigRepository(deps.DB)
	bindings, err := repo.ListTenantPluginBindings(c.Request.Context(), reposetting.ListTenantPluginOptions{
		PluginIDs:   []string{pluginID},
		Key:         reposetting.KeyClientCredentials,
		OnlyEnabled: true,
	})
	if err != nil {
		return 0, err
	}
	seeded := 0
	for _, binding := range bindings {
		tenantUUID := strings.TrimSpace(binding.TenantUUID)
		if tenantUUID == "" {
			return seeded, fmt.Errorf("empty tenant uuid for plugin %s", pluginID)
		}
		if err := ensureTenantEventFabricTopics(c, deps, tenantUUID, pluginID); err != nil {
			return seeded, err
		}
		seeded++
	}
	logger.InfoF(c.Request.Context(), "[plugin-enable] event_fabric seeded existing tenants plugin=%s count=%d", pluginID, seeded)
	return seeded, nil
}

// POST /api/.../admin/plugins/:id/event_fabric/resync
func PluginEventFabricResyncHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		manager, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		if _, err := manager.Get(c.Request.Context(), id); err != nil {
			dtoRequest.ResponseError(c, http.StatusNotFound, "插件不存在", err)
			return
		}
		seeded, err := ensureEnabledTenantEventFabricTopics(c, deps, id)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "Topic 重新同步失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"ok":                          true,
			"plugin_id":                   id,
			"event_fabric_seeded_tenants": seeded,
		})
	}
}

// POST /api/.../admin/plugins/:id/disable
func PluginDisableHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		ctx := c.Request.Context()
		if deps != nil && deps.DB != nil {
			drainSvc := pluginservice.NewPluginDrainJobService(deps.DB)
			if _, drainErr := drainSvc.RequireNoActiveTenantInstancesForDisable(ctx, id); drainErr != nil {
				dtoRequest.RespondErrorFrom(c, drainErr)
				return
			}
		}
		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		if err := mgr.Disable(ctx, id); err != nil {
			dtoRequest.ResponseError(c, statusFromManagerErr(err), "停用插件失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	}
}

func optionalTenantContext(c *gin.Context) string {
	return strings.TrimSpace(reqctx.TenantUUIDFromGin(c))
}

// GET /api/.../admin/plugins/menus
func PluginMenusHandler(c *gin.Context) {
	ctx := c.Request.Context()
	mgr, err := tryGetPluginManager()
	var list []pluginMgr.Plugin
	if err == nil {
		list, err = mgr.List(ctx)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusInternalServerError, "获取插件菜单失败", err)
			return
		}
	} else {
		list, err = listPluginsFromRegistry(ctx)
		if err != nil {
			dtoRequest.ResponseSuccess(c, gin.H{"menus": []pluginDto.PluginMenuItem{}})
			return
		}
	}

	prefix := basePrefix()
	agg := make([]pluginDto.PluginMenuItem, 0, 16)

	for _, p := range list {
		if p.State != pluginMgr.StateEnabled {
			continue
		}
		if !(p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "") {
			continue
		}
		base := prefix + "/" + p.ID + "/admin/"
		for _, m := range p.Frontend.Admin.Menus {
			title := m.Title
			if m.TitleI18n != nil {
				if def := m.TitleI18n.Default; def != "" {
					title = def
				} else if title == "" {
					title = m.TitleI18n.Key
				}
			}
			agg = append(agg, pluginDto.PluginMenuItem{
				ID:    p.ID,
				Title: title,
				Icon:  m.Icon,
				Order: m.Order,
				URL:   base,
			})
		}
	}
	sort.Slice(agg, func(i, j int) bool { return agg[i].Order < agg[j].Order })

	dtoRequest.ResponseSuccess(c, gin.H{"menus": agg})
}

// GET /api/.../admin/plugins/:id
// 返回插件详情（合并注册表信息与运行态）
func PluginGetHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	ctx := c.Request.Context()
	mgr, err := tryGetPluginManager()
	var (
		p    pluginMgr.Plugin
		proc = supervisor.ProcInfo{}
	)
	if err == nil {
		p, err = mgr.Get(ctx, id)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusNotFound, "插件不存在", err)
			return
		}
		proc, _ = manager.TryRuntimeStatus(mgr, id)
	} else {
		p, err = getPluginFromRegistry(ctx, id)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusNotFound, "插件不存在", err)
			return
		}
	}

	prefix := basePrefix()
	hasAdmin := p.Frontend.Admin.Kind == pluginMgr.FrontendKindStatic && p.Paths.FrontendAdminDir != ""
	adminURL := ""
	if hasAdmin {
		adminURL = prefix + "/" + p.ID + "/admin/"
	}

	// 统一输出结构（贴近列表项 + 运行状态）
	out := gin.H{
		"id":          p.ID,
		"name":        p.Name,
		"version":     p.Version,
		"state":       string(p.State),
		"adminURL":    adminURL,
		"apiBase":     prefix + "/" + p.ID + "/api",
		"hasAdmin":    hasAdmin,
		"description": p.Description,
		"author":      p.Metadata.Author,
		"category":    p.Metadata.Category,
		"tags":        append([]string(nil), p.Metadata.Tags...),
		"runtime": gin.H{
			"pid":           proc.PID,
			"port":          proc.Port,
			"state":         proc.State,
			"healthy":       proc.Healthy,
			"restarts":      proc.Restarts,
			"started_at":    proc.StartedAt,
			"stopped_at":    proc.StoppedAt,
			"last_exit_err": proc.LastExitErr,
			"health_ok":     proc.HealthOKCount,
			"health_fails":  proc.HealthFails,
		},
	}

	dtoRequest.ResponseSuccess(c, out)
}

// POST /api/.../admin/plugins/:id/restart
func PluginRestartHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
		return
	}
	mgr, err := tryGetPluginManager()
	if err != nil {
		respondPluginRuntimeUnavailable(c, err)
		return
	}

	ctx := c.Request.Context()
	if err := mgr.Disable(ctx, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "重启插件失败（停用阶段）", err)
		return
	}
	if err := mgr.Enable(ctx, id); err != nil {
		dtoRequest.ResponseError(c, statusFromManagerErr(err), "重启插件失败（启用阶段）", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// 你可以在 pluginMgr 里提供 Code→HTTP 的映射；这里先最小化映射为 400
func statusFromManagerErr(error) int { return http.StatusBadRequest }
