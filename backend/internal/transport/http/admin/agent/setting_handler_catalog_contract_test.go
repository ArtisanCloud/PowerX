package agent

import (
	"encoding/json"
	"testing"
)

func TestProviderCatalogRESTContractUsesLowerCamelCaseIdentityFields(t *testing.T) {
	raw, err := json.Marshal(providerView{ID: "ollama", Name: "Ollama (Local)"})
	if err != nil {
		t.Fatal(err)
	}
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if item["id"] != "ollama" || item["name"] != "Ollama (Local)" {
		t.Fatalf("provider identity contract=%s", raw)
	}
	if _, exists := item["ID"]; exists {
		t.Fatalf("provider contract must not expose Go field name: %s", raw)
	}
	if _, exists := item["Name"]; exists {
		t.Fatalf("provider contract must not expose Go field name: %s", raw)
	}
}
