package metadata_contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestTenantMetadataHostContractIsUUIDOnlyAndCapabilityBound(t *testing.T) {
	routes, err := os.ReadFile("../../../internal/transport/http/admin/metadata/api.go")
	if err != nil { t.Fatal(err) }
	text := string(routes)
	for _, route := range []string{
		`g.GET("/dictionaries"`, `g.POST("/dictionaries"`, `g.PATCH("/dictionaries/:namespace_uuid"`,
		`g.GET("/taxonomies"`, `g.POST("/taxonomies"`, `g.POST("/tag-bindings"`,
		`g.DELETE("/tag-bindings/:binding_uuid"`, `g.POST("/resource-types"`, `g.PATCH("/resource-types/:resource_type_uuid"`,
	} { if !strings.Contains(text, route) { t.Fatalf("missing tenant host route %s", route) } }
	for _, forbidden := range []string{"tenant_uuid must not be supplied", "tenantHostAuthorize", "RegisterTenantHostRoutes"} { if !strings.Contains(text, forbidden) { t.Fatalf("missing tenant boundary %s", forbidden) } }
	config, err := os.ReadFile("../../../config/platform_capabilities/metadata.yaml"); if err != nil { t.Fatal(err) }
	for _, capability := range []string{"com.corex.metadata.dictionary.read", "com.corex.metadata.dictionary.manage", "com.corex.metadata.taxonomy.read", "com.corex.metadata.taxonomy.manage", "com.corex.metadata.tag.read", "com.corex.metadata.tag.manage", "com.corex.metadata.resource_type.read", "com.corex.metadata.resource_type.manage"} { if !strings.Contains(string(config), capability) { t.Fatalf("missing capability %s", capability) } }
	for _, required := range []string{"actor_context: service_actor", "sts_direct: true", "/api/v1/tenant/metadata/resource-types"} { if !strings.Contains(string(config), required) { t.Fatalf("missing service binding %s", required) } }
}

func TestTagBindingHasStableExternalUUID(t *testing.T) {
	model, err := os.ReadFile("../../../pkg/corex/db/persistence/model/metadata/models.go"); if err != nil { t.Fatal(err) }
	if !strings.Contains(string(model), "BindingUUID") || !strings.Contains(string(model), "uk_metadata_tag_binding_business") { t.Fatal("tag binding must have its own UUID and retain business uniqueness") }
}
