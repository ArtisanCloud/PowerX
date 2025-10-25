package capability

import (
	"context"
	"testing"
)

type stubScopeVerifier struct {
	result bool
	err    error
}

func (s stubScopeVerifier) ScopeExists(_ context.Context, scope string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.result && scope != "", nil
}

func TestValidateContractDraft_Success(t *testing.T) {
	v := NewValidator(ValidatorOptions{})
	issues, err := v.ValidateContractDraft(context.Background(), &CapabilityContractDraft{
		CapabilityKey:  "crm.lead.create",
		Version:        "1.0.0",
		DisplayName:    "创建线索",
		ProviderID:     "provider.crm",
		LifecycleState: "draft",
		SecurityScope:  "crm.lead.write",
		IOSchemas: []IOSchemaDescriptor{
			{Direction: "input", Format: "json_schema", SchemaURI: "s3://schemas/input.json"},
			{Direction: "output", Format: "json_schema", SchemaURI: "s3://schemas/output.json"},
		},
		TransportPreferences: []TransportPreference{
			{Transport: "grpc", Mode: "prefer"},
			{Transport: "http", Mode: "fallback"},
		},
		TransportProfiles: []TransportProfile{
			{Transport: "grpc", Mode: "prefer", TimeoutMillis: 8000},
			{Transport: "http", Mode: "fallback", TimeoutMillis: 12000},
		},
		ErrorTaxonomy: []ErrorTaxonomyEntry{
			{Namespace: "capability", Category: "transport", Code: "timeout", Severity: "ERROR", Stage: "invoke"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %d: %#v", len(issues), issues)
	}
}

func TestValidateContractDraft_InvalidKeyAndVersion(t *testing.T) {
	v := NewValidator(ValidatorOptions{})
	issues, err := v.ValidateContractDraft(context.Background(), &CapabilityContractDraft{
		CapabilityKey: "Invalid Key",
		Version:       "1.0",
		DisplayName:   "",
		SecurityScope: "",
		IOSchemas: []IOSchemaDescriptor{
			{Direction: "input", Format: "json_schema"},
		},
		TransportPreferences: []TransportPreference{
			{Transport: "grpc", Mode: "prefer"},
			{Transport: "grpc", Mode: "fallback"},
		},
		TransportProfiles: []TransportProfile{
			{Transport: "grpc", Mode: "prefer", TimeoutMillis: 0},
		},
		ErrorTaxonomy: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) == 0 {
		t.Fatalf("expected issues but got none")
	}
	var hasKeyIssue, hasVersionIssue bool
	for _, issue := range issues {
		switch issue.Code {
		case "contract.capability_key.format":
			hasKeyIssue = true
		case "contract.version.format":
			hasVersionIssue = true
		}
	}
	if !hasKeyIssue || !hasVersionIssue {
		t.Fatalf("expected key & version issues, got %#v", issues)
	}
}

func TestValidateContractDraft_ScopeVerification(t *testing.T) {
	v := NewValidator(ValidatorOptions{
		ScopeVerifier: stubScopeVerifier{result: false},
	})
	issues, err := v.ValidateContractDraft(context.Background(), &CapabilityContractDraft{
		CapabilityKey: "crm.lead.create",
		Version:       "1.0.0",
		DisplayName:   "创建线索",
		SecurityScope: "crm.lead.write",
		IOSchemas: []IOSchemaDescriptor{
			{Direction: "input", Format: "json_schema", SchemaURI: "a"},
			{Direction: "output", Format: "json_schema", SchemaURI: "b"},
		},
		TransportPreferences: []TransportPreference{
			{Transport: "grpc", Mode: "prefer"},
			{Transport: "http", Mode: "fallback"},
		},
		TransportProfiles: []TransportProfile{
			{Transport: "grpc", Mode: "prefer", TimeoutMillis: 1000},
			{Transport: "http", Mode: "fallback", TimeoutMillis: 1000},
		},
		ErrorTaxonomy: []ErrorTaxonomyEntry{
			{Namespace: "capability", Category: "transport", Code: "timeout", Severity: "ERROR"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var scopeMissing bool
	for _, issue := range issues {
		if issue.Code == "contract.scope.missing" {
			scopeMissing = true
			break
		}
	}
	if !scopeMissing {
		t.Fatalf("expected scope missing issue, got %#v", issues)
	}
}
