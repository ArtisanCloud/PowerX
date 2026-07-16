package contracts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMetadataPatchOperationsHaveGRPCUpdateParity(t *testing.T) {
	root := repoRoot(t)
	openapi := readFile(t, filepath.Join(root, "specs/029-metadata-governance/contracts/http-openapi.yaml"))
	proto := readFile(t, filepath.Join(root, "backend/api/grpc/contracts/powerx/metadata/v1/metadata.proto"))

	re := regexp.MustCompile(`operationId:\s+(update[A-Za-z0-9]+)`)
	matches := re.FindAllStringSubmatch(openapi, -1)
	if len(matches) == 0 {
		t.Fatalf("expected metadata OpenAPI to declare update operations")
	}
	for _, match := range matches {
		rpc := strings.ToUpper(match[1][:1]) + match[1][1:]
		if !strings.Contains(proto, "rpc "+rpc+"(") {
			t.Fatalf("missing gRPC parity for REST operationId %s", match[1])
		}
	}
}

func TestMetadataGRPCUpdateScalarsPreservePresence(t *testing.T) {
	root := repoRoot(t)
	proto := readFile(t, filepath.Join(root, "backend/api/grpc/contracts/powerx/metadata/v1/metadata.proto"))
	required := []string{
		"optional int32 sort_order",
		"optional int32 max_depth",
		"optional MetadataStatus status",
		"optional bool binding_enabled",
		"optional string validator_key",
		"optional string color",
	}
	for _, snippet := range required {
		if !strings.Contains(proto, snippet) {
			t.Fatalf("metadata proto must preserve update field presence with %q", snippet)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Dir(dir)
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = next
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
