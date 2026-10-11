package integration_gateway

import (
	"strings"
	"testing"
)

func TestPlatformAdapterIDFitsPersistentLimit(t *testing.T) {
	if got := makeAdapterID("com.corex.knowledge.catalog.read", "rest", 0); got != "com-corex-knowledge-catalog-read-rest" {
		t.Fatal("existing adapter ID changed")
	}
	long := "com.corex." + strings.Repeat("a", 118)
	a, b := makeAdapterID(long, "rest", 0), makeAdapterID(long, "rest", 1)
	if len(a) > 128 || len(b) > 128 || a == b || a != makeAdapterID(long, "rest", 0) {
		t.Fatal("bounded adapter IDs must remain deterministic and distinct")
	}
}
