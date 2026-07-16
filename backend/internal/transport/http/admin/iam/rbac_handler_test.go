package iam

import "testing"

func TestParsePermissionCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		module   string
		resource string
		action   string
	}{
		{
			name:     "metadata resource type",
			code:     "metadata.resource_type:read",
			module:   "metadata",
			resource: "resource_type",
			action:   "read",
		},
		{
			name:     "plugin permission",
			code:     "com.powerx.plugins.base.local.template:read",
			module:   "com.powerx.plugins.base.local",
			resource: "template",
			action:   "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, resource, action, err := parsePermissionCode(tt.code)
			if err != nil {
				t.Fatalf("parsePermissionCode() error = %v", err)
			}
			if module != tt.module || resource != tt.resource || action != tt.action {
				t.Fatalf("parsePermissionCode() = (%q,%q,%q), want (%q,%q,%q)", module, resource, action, tt.module, tt.resource, tt.action)
			}
		})
	}
}

func TestParsePermissionCodeRejectsInvalidFormat(t *testing.T) {
	for _, code := range []string{"", "metadata", "metadata.dictionary", "metadata:read", ".dictionary:read", "metadata.:read", "metadata.dictionary:"} {
		if _, _, _, err := parsePermissionCode(code); err == nil {
			t.Fatalf("parsePermissionCode(%q) expected error", code)
		}
	}
}
