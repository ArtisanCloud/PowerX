package seed

import (
	"encoding/json"
	"testing"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

func TestPermFromPathAndMethodCreatesDeprecatedSwaggerCandidate(t *testing.T) {
	perm := permFromPathAndMethod("/api/v1/admin/example/records/{uuid}/approve", "POST")

	if perm.Status != dbm.PermissionStatusDeprecated {
		t.Fatalf("Status = %q, want deprecated", perm.Status)
	}
	if perm.Source != "swagger" {
		t.Fatalf("Source = %q, want swagger", perm.Source)
	}
	if perm.DeprecatedAt == nil {
		t.Fatal("DeprecatedAt is nil")
	}
	if perm.AllowAPIKey {
		t.Fatal("AllowAPIKey = true, want false for swagger candidate")
	}

	var meta map[string]any
	if err := json.Unmarshal(perm.Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta["type"] != "api_candidate" {
		t.Fatalf("meta.type = %#v, want api_candidate", meta["type"])
	}
	if _, ok := meta["label"]; ok {
		t.Fatalf("meta.label should not be set for swagger candidates: %#v", meta["label"])
	}
	if meta["registration_status"] != "invalid" {
		t.Fatalf("meta.registration_status = %#v, want invalid", meta["registration_status"])
	}
	if meta["http_method"] != "POST" || meta["api_endpoint"] != "/api/v1/admin/example/records/{uuid}/approve" {
		t.Fatalf("unexpected binding meta: %#v", meta)
	}
}
