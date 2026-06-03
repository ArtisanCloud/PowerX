package integration_gateway

import (
	"strings"
	"testing"
)

func TestBuiltinPlatformCapabilitiesDeclareMediaUploadREST(t *testing.T) {
	const (
		wantCapability = "com.corex.media.assets.manage"
		wantEndpoint   = "/api/v1/media/assets/{uuid}"
		wantMethod     = "PUT"
	)

	for _, def := range builtinPlatformCapabilityDefinitions() {
		if def.CapabilityID != wantCapability {
			continue
		}
		for _, binding := range def.Protocols {
			if strings.EqualFold(strings.TrimSpace(binding.Channel), "rest") &&
				strings.EqualFold(strings.TrimSpace(binding.Method), wantMethod) &&
				strings.TrimSpace(binding.Endpoint) == wantEndpoint &&
				strings.EqualFold(strings.TrimSpace(binding.AuthType), "tenant_jwt") {
				return
			}
		}
		t.Fatalf("%s missing REST %s %s tenant_jwt binding", wantCapability, wantMethod, wantEndpoint)
	}

	t.Fatalf("platform capability %s not found", wantCapability)
}
