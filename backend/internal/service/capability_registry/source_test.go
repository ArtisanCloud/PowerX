package capability_registry

import "testing"

func TestNormalizeCapabilitySource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty means all", input: "", want: ""},
		{name: "all means all", input: "all", want: ""},
		{name: "any means all", input: "any", want: ""},
		{name: "corex canonical", input: "corex", want: CapabilitySourceCoreX},
		{name: "platform alias", input: "platform", want: CapabilitySourceCoreX},
		{name: "plugin canonical", input: "plugin", want: CapabilitySourcePlugin},
		{name: "invalid source", input: "foobar", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeCapabilitySource(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
