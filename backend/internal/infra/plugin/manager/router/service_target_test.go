package router

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestPluginServiceTargetKeepsActualMountedBasePathAndUsageGuards(t *testing.T) {
	for _, base := range []string{"/api/v1", "/custom/api/v7"} {
		t.Run(base, func(t *testing.T) {
			target, err := url.Parse("http://127.0.0.1:49152")
			require.NoError(t, err)
			var tenantErr, usageErr error
			router := &DynamicRouter{apis: map[string]apiUpstream{"com.powerx.plugins.scrm": {target: target, basePath: base}}, tenantGuard: func(context.Context, string, string) error { return tenantErr }, usageGuard: func(context.Context, string) error { return usageErr }}
			path := base + "/service/messaging/executions.reply"
			actual, err := router.ResolvePluginServiceTarget(context.Background(), "tenant", "com.powerx.plugins.scrm", "1.0.0", path)
			require.NoError(t, err)
			require.Equal(t, target.String()+path, actual)
			for _, wrong := range []string{"/service/messaging/executions.reply", base + "-other/service/messaging/executions.reply", "/_p/com.powerx.plugins.scrm" + path} {
				_, err = router.ResolvePluginServiceTarget(context.Background(), "tenant", "com.powerx.plugins.scrm", "1.0.0", wrong)
				require.Equal(t, "CAPABILITY_SERVICE_BINDING_INVALID", dto.CodeOf(err))
			}
			tenantErr = errors.New("tenant disabled")
			_, err = router.ResolvePluginServiceTarget(context.Background(), "tenant", "com.powerx.plugins.scrm", "1.0.0", path)
			require.ErrorIs(t, err, tenantErr)
			tenantErr = nil
			usageErr = errors.New("plugin draining")
			_, err = router.ResolvePluginServiceTarget(context.Background(), "tenant", "com.powerx.plugins.scrm", "1.0.0", path)
			require.ErrorIs(t, err, usageErr)
		})
	}
}
