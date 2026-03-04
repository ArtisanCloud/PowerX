package plugin

import (
	"fmt"

	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

func tryGetPluginManager() (mgr plugin_mgr.Manager, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin manager unavailable: %v", r)
			mgr = nil
		}
	}()
	mgr = mgrimpl.GetPluginManager()
	if mgr == nil {
		return nil, fmt.Errorf("plugin manager unavailable: nil manager")
	}
	return mgr, nil
}

func respondPluginRuntimeUnavailable(c *gin.Context, err error) {
	dtoRequest.ResponseError(c, 503, "插件运行时不可用", err)
}
