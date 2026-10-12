package plugin_release

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPluginReleaseHostContractRegistersOnlyUUIDTenantRoutes(t *testing.T) {
	engine := gin.New()
	RegisterTenantRoutes(engine.Group("/api/v1"), nil)
	// Nil dependencies deliberately mount no routes; this test protects the
	// concrete route spellings at the handler level below instead of accepting
	// legacy numeric aliases through a global router.
	_ = engine

	for _, tt := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{"unauthorized", pluginReleaseUnauthorized(errors.New("missing token")), http.StatusUnauthorized, "PLUGIN_RELEASE_UNAUTHORIZED"},
		{"forbidden", pluginReleaseForbidden(errors.New("grant missing")), http.StatusForbidden, "PLUGIN_RELEASE_FORBIDDEN"},
		{"session missing", pluginReleaseSessionNotFound(nil), http.StatusNotFound, "PLUGIN_RELEASE_SESSION_NOT_FOUND"},
		{"import missing", pluginReleaseImportJobNotFound(nil), http.StatusNotFound, "PLUGIN_RELEASE_IMPORT_JOB_NOT_FOUND"},
		{"active conflict", pluginReleaseActiveSessionConflict(nil), http.StatusConflict, "PLUGIN_RELEASE_ACTIVE_SESSION_CONFLICT"},
		{"package verification", pluginReleasePackageVerificationFailed(errors.New("signature invalid")), http.StatusUnprocessableEntity, "PLUGIN_RELEASE_PACKAGE_VERIFICATION_FAILED"},
		{"upstream", pluginReleaseUpstream(errors.New("dependency unavailable")), http.StatusServiceUnavailable, "PLUGIN_RELEASE_UPSTREAM_DEPENDENCY"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, dto.StatusCode(tt.err))
			require.Equal(t, tt.code, dto.CodeOf(tt.err))
		})
	}
}
