package runtime_identity

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

const CapabilityID = "com.corex.runtime.identity.read"
const APIKeyScope = "_scope.runtime.identity.read"

var pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type Identity struct {
	RuntimeMode         string `json:"runtime_mode"`
	CoreVersion         string `json:"core_version"`
	DeploymentEnv       string `json:"deployment_env"`
	PluginID            string `json:"plugin_id"`
	RuntimePluginID     string `json:"runtime_plugin_id"`
	PluginVersion       string `json:"plugin_version"`
	PluginVersionSource string `json:"plugin_version_source"`
	PluginState         string `json:"plugin_state"`
}
type Service struct {
	manager func() (plugin_mgr.Manager, error)
	info    CoreInfo
}

type CoreInfo struct {
	Version       string
	DeploymentEnv string
	Ready         bool
	Manager       func() (plugin_mgr.Manager, error)
}

func NewService(info CoreInfo) *Service { return &Service{manager: info.Manager, info: info} }
func Error(status int, code string) error {
	return dto.NewErrorWithCode(status, code, dto.RuntimeIdentityErrorMessage("", code), nil)
}
func ValidPluginID(id string) bool { return pluginIDPattern.MatchString(id) }

// Lookup is used only after service authorization or the Admin route guard.
// Version describes the selected registry entry, never the running binary.
func (s *Service) Lookup(ctx context.Context, id string) (*Identity, error) {
	if !ValidPluginID(id) {
		return nil, Error(http.StatusBadRequest, "RUNTIME_IDENTITY_INVALID_ARGUMENT")
	}
	info := s.info
	if !info.Ready || info.Version == "" {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE")
	}
	if s.manager == nil {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE")
	}
	mgr, err := s.manager()
	if err != nil || mgr == nil {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE")
	}
	p, err := mgr.Get(ctx, id)
	if err != nil {
		var typed *plugin_mgr.ManagerError
		if errors.As(err, &typed) && typed.Code == plugin_mgr.CodeNotFound {
			return nil, Error(http.StatusNotFound, "RUNTIME_IDENTITY_PLUGIN_NOT_FOUND")
		}
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE")
	}
	if strings.TrimSpace(p.Version) == "" {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_VERSION_UNAVAILABLE")
	}
	runtime, ok := mgr.(interface {
		RuntimeStatus(string) (supervisor.ProcInfo, bool)
	})
	if !ok {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_STATE_UNAVAILABLE")
	}
	proc, ok := runtime.RuntimeStatus(id)
	if !ok || strings.TrimSpace(string(proc.State)) == "" {
		return nil, Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_STATE_UNAVAILABLE")
	}
	return &Identity{RuntimeMode: "powerx", CoreVersion: info.Version, DeploymentEnv: info.DeploymentEnv, PluginID: id, RuntimePluginID: id, PluginVersion: p.Version, PluginVersionSource: "registry", PluginState: string(proc.State)}, nil
}
