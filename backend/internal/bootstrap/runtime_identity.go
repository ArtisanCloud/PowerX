package bootstrap

import (
	"errors"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func runtimeIdentityManager() (mgr plugin_mgr.Manager, err error) {
	defer func() {
		if recover() != nil {
			mgr = nil
			err = errors.New("plugin manager unavailable")
		}
	}()
	mgr = manager.GetPluginManager()
	if mgr == nil {
		return nil, errors.New("plugin manager unavailable")
	}
	return mgr, nil
}
