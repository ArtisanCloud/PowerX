package workflow

import (
	"context"
	"errors"
	"testing"
)

type testNodeAdapter struct {
	spec        NodeAdapterSpec
	validateErr error
}

func (a testNodeAdapter) Spec() NodeAdapterSpec {
	return a.spec
}

func (a testNodeAdapter) Validate(StepDefinition) error {
	return a.validateErr
}

func (a testNodeAdapter) Execute(context.Context, NodeExecutionContext) (NodeResult, error) {
	return NodeResult{Status: NodeResultStatusSucceeded}, nil
}

func TestNodeAdapterRegistryRegisterRejectsInvalidAdapter(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := registry.Register(nil); !errors.Is(err, ErrNodeAdapterNil) {
		t.Fatalf("expected ErrNodeAdapterNil, got %v", err)
	}
	if err := registry.Register(testNodeAdapter{}); !errors.Is(err, ErrNodeKindRequired) {
		t.Fatalf("expected ErrNodeKindRequired, got %v", err)
	}
}

func TestNodeAdapterRegistryRejectsDuplicateNodeKind(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	adapter := testNodeAdapter{spec: NodeAdapterSpec{NodeKind: "skill.invoke"}}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register first adapter: %v", err)
	}
	if err := registry.Register(testNodeAdapter{spec: NodeAdapterSpec{NodeKind: " SKILL.INVOKE "}}); !errors.Is(err, ErrNodeAdapterDuplicated) {
		t.Fatalf("expected ErrNodeAdapterDuplicated, got %v", err)
	}
}

func TestNodeAdapterRegistryAdapterRequiresKnownNodeKind(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if _, err := registry.Adapter(""); !errors.Is(err, ErrNodeKindRequired) {
		t.Fatalf("expected ErrNodeKindRequired, got %v", err)
	}
	if _, err := registry.Adapter("capability.invoke"); !errors.Is(err, ErrNodeAdapterNotFound) {
		t.Fatalf("expected ErrNodeAdapterNotFound, got %v", err)
	}
}

func TestNodeAdapterRegistryListIsSortedAndNormalized(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := registry.Register(testNodeAdapter{spec: NodeAdapterSpec{NodeKind: "metadata.write", Category: "metadata"}}); err != nil {
		t.Fatalf("register metadata adapter: %v", err)
	}
	if err := registry.Register(testNodeAdapter{spec: NodeAdapterSpec{NodeKind: " Skill.Invoke ", Category: "skill"}}); err != nil {
		t.Fatalf("register skill adapter: %v", err)
	}

	specs := registry.List()
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].NodeKind != "metadata.write" || specs[1].NodeKind != "skill.invoke" {
		t.Fatalf("expected sorted normalized node kinds, got %#v", specs)
	}
}

func TestNodeAdapterRegistryValidateDefinitionDelegatesToAdapter(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	expected := errors.New("workflow.test_validation_failed")
	if err := registry.Register(testNodeAdapter{spec: NodeAdapterSpec{NodeKind: "knowledge.stage"}, validateErr: expected}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	if err := registry.ValidateDefinition(StepDefinition{NodeKind: "knowledge.stage"}); !errors.Is(err, expected) {
		t.Fatalf("expected adapter validation error, got %v", err)
	}
}
