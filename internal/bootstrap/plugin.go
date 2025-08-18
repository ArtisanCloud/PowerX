package bootstrap

import (
	"context"
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

	pmimpl.InitGlobal(mgr)

	return mgr, nil
}
