package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, p))
}

func BootstrapPlugin(ctx context.Context, cfg *config.Config, r *gin.Engine) (pm.Manager, error) {
	dr := pmimpl.NewDynamicRouter(cfg.Plugin.BasePrefix, r)
	sup := supervisor.New()

	installedRoot := abs(cfg.Plugin.InstalledDir)
	registryFile := abs(cfg.Plugin.RegistryFile)

	mgr := pmimpl.New(pmimpl.Options{
		Enabled:       cfg.Plugin.Enabled,
		BasePrefix:    cfg.Plugin.BasePrefix,
		InstalledRoot: installedRoot,
		RegistryFile:  registryFile,
		Loader:        pmimpl.NewFSLoader(),
		Registry:      pmimpl.NewJSONRegistry(registryFile),
		HTTP:          dr,
		Supervisor:    sup,
	})
	if err := mgr.Bootstrap(ctx); err != nil {
		return nil, err
	}

	// ★ 自动恢复：把上次 state=enabled 的插件重新启用
	if list, err := mgr.List(ctx); err != nil {
		logger.WarnF(ctx, "auto-restore: list failed: %v", err) // ← 看看是不是这里错了
	} else {
		enabled := 0
		for _, p := range list {
			logger.InfoF(ctx, "boot state: id=%s ver=%s state=%s", p.ID, p.Version, p.State)
			if p.State == pm.StateEnabled {
				enabled++
				if err := mgr.Enable(ctx, p.ID); err != nil {
					logger.WarnF(ctx, "auto-restore failed: id=%s err=%v", p.ID, err)
				} else {
					logger.InfoF(ctx, "auto-restore ok: id=%s", p.ID)
				}
			}
		}
		logger.InfoF(ctx, "auto-restore scanned=%d enabled=%d", len(list), enabled)
	}

	pmimpl.InitGlobal(mgr)

	return mgr, nil
}
