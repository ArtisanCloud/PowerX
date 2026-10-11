package main

import (
	"strings"
	"testing"
)

func TestGeneratedCapabilityIDsFitPersistentLimit(t *testing.T) {
	short := "com.corex.rest.admin.read"
	if got := joinID("com.corex", "rest", "admin", "read"); got != short {
		t.Fatalf("existing valid identifier changed: %s", got)
	}
	prefix := strings.Repeat("a", 150)
	a, b := joinID("com.corex", prefix+"read"), joinID("com.corex", prefix+"write")
	if len(a) != 128 || a == b || a != joinID("com.corex", prefix+"read") {
		t.Fatalf("identifier must be bounded, deterministic and distinct")
	}
}
