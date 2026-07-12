package plugin_dev

import (
	"testing"
)

func TestValidateLocalDebugHostRequestRequiresLocalPluginID(t *testing.T) {
	if err := validateLocalDebugHostRequest("com.powerx.plugins.base.local", 8078); err != nil {
		t.Fatalf("expected local plugin id to pass, got %v", err)
	}
	if err := validateLocalDebugHostRequest("com.powerx.plugins.base", 8078); err == nil {
		t.Fatal("expected non-local plugin id to fail")
	}
	if err := validateLocalDebugHostRequest("com.powerx.plugins.base.local", 0); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}
