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

func TestBuiltinPlatformCapabilitiesDeclareMediaVariantREST(t *testing.T) {
	want := map[string]string{
		"POST /api/v1/media/assets/{uuid}/variants/{variant}":         "",
		"PUT /api/v1/media/assets/{uuid}/variants/{variant}":          "",
		"POST /api/v1/media/assets/{uuid}/variants/{variant}/presign": "",
		"GET /api/v1/media/assets/{uuid}/variants/{variant}/resource": "",
	}
	for _, def := range builtinPlatformCapabilityDefinitions() {
		if def.CapabilityID != "com.corex.media.assets.manage" {
			continue
		}
		for _, binding := range def.Protocols {
			key := strings.ToUpper(strings.TrimSpace(binding.Method)) + " " + strings.TrimSpace(binding.Endpoint)
			if _, ok := want[key]; ok && strings.EqualFold(strings.TrimSpace(binding.AuthType), "tenant_jwt") {
				delete(want, key)
			}
		}
		if len(want) > 0 {
			t.Fatalf("media variant REST bindings missing: %#v", want)
		}
		return
	}
	t.Fatal("platform capability com.corex.media.assets.manage not found")
}
