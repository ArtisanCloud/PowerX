package metadata

import (
	"testing"

	"gorm.io/datatypes"
)

func TestMetadataModelsExposeUUIDAndJSONBFields(t *testing.T) {
	ns := DictionaryNamespace{NameI18n: datatypes.JSON([]byte(`{"zh-CN":"客户等级"}`))}
	if ns.UUID.String() == "" {
		t.Fatalf("uuid field must be present")
	}
	if len(ns.NameI18n) == 0 {
		t.Fatalf("name_i18n jsonb field must be present")
	}
}

func TestMetadataRelationshipTablesUseObjectUUIDs(t *testing.T) {
	binding := TagBinding{TenantUUID: "tenant", TagUUID: "tag", ResourceUUID: "resource"}
	if binding.TagUUID == "" || binding.ResourceUUID == "" {
		t.Fatalf("tag binding must reference object UUIDs")
	}
	ref := Reference{TenantUUID: "tenant", MetadataUUID: "metadata", ResourceUUID: "resource"}
	if ref.MetadataUUID == "" || ref.ResourceUUID == "" {
		t.Fatalf("metadata reference must reference object UUIDs")
	}
}
