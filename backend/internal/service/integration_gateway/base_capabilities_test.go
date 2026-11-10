package integration_gateway

import (
	"strings"
	"testing"
)

func TestFormalPlatformCapabilitiesDeclareMediaUploadREST(t *testing.T) {
	const (
		wantCapability = "com.corex.media.assets.transfer"
		wantEndpoint   = "/api/v1/media/transfers/{asset_uuid}/upload"
		wantMethod     = "PUT"
	)

	defs, err := parsePlatformCapabilityFile("../../../config/platform_capabilities/media.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range defs {
		if def.CapabilityID != wantCapability {
			continue
		}
		for _, binding := range def.Protocols {
			if strings.EqualFold(strings.TrimSpace(binding.Channel), "rest") &&
				strings.EqualFold(strings.TrimSpace(binding.Method), wantMethod) &&
				strings.TrimSpace(binding.Endpoint) == wantEndpoint &&
				strings.EqualFold(strings.TrimSpace(binding.AuthType), "signed_ticket") && binding.ResourceScope == "ticket_scoped_asset" && !binding.STSDirect {
				return
			}
		}
		t.Fatalf("%s missing REST %s %s signed_ticket binding", wantCapability, wantMethod, wantEndpoint)
	}

	t.Fatalf("platform capability %s not found", wantCapability)
}

func TestFormalPlatformCapabilitiesDeclareMediaVariantREST(t *testing.T) {
	want := map[string]string{
		"POST /api/v1/tenant/media/assets/{asset_uuid}/variants/{variant_uuid}/presign-upload":   "api_key_or_bearer",
		"POST /api/v1/tenant/media/assets/{asset_uuid}/variants/{variant_uuid}/complete-upload":  "api_key_or_bearer",
		"POST /api/v1/tenant/media/assets/{asset_uuid}/variants/{variant_uuid}/presign-download": "api_key_or_bearer",
		"PUT /api/v1/media/transfers/{asset_uuid}/variants/{variant_uuid}/upload":                "signed_ticket",
		"GET /api/v1/media/transfers/{asset_uuid}/variants/{variant_uuid}/download":              "signed_ticket",
	}
	defs, err := parsePlatformCapabilityFile("../../../config/platform_capabilities/media.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range defs {
		if def.CapabilityID != "com.corex.media.assets.transfer" {
			continue
		}
		for _, binding := range def.Protocols {
			key := strings.ToUpper(strings.TrimSpace(binding.Method)) + " " + strings.TrimSpace(binding.Endpoint)
			if auth, ok := want[key]; ok && strings.EqualFold(strings.TrimSpace(binding.AuthType), auth) && binding.ActorContext == "service_actor" {
				delete(want, key)
			}
		}
		if len(want) > 0 {
			t.Fatalf("media variant REST bindings missing: %#v", want)
		}
		return
	}
	t.Fatal("platform capability com.corex.media.assets.transfer not found")
}
