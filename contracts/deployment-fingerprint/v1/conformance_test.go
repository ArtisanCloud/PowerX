package deploymentfingerprint

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := Bundle.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func TestSharedJSONSchemas(t *testing.T) {
	var data struct {
		Cases []struct {
			Name, Definition string
			Data             json.RawMessage
			Valid            bool
		}
	}
	fixture(t, "schema-cases.json", &data)
	for _, row := range data.Cases {
		t.Run(row.Name, func(t *testing.T) {
			_, err := Validate(row.Definition, row.Data)
			if (err == nil) != row.Valid {
				t.Fatalf("valid=%v, error=%v", row.Valid, err)
			}
		})
	}
}

func TestCrossLanguageGoldenDigests(t *testing.T) {
	var data struct {
		Vectors []struct {
			Name      string
			Manifest  json.RawMessage
			Canonical string `json:"canonical_payload"`
			ID        string `json:"build_id"`
		}
	}
	fixture(t, "golden-manifests.json", &data)
	for _, row := range data.Vectors {
		t.Run(row.Name, func(t *testing.T) {
			canonical, id, err := ManifestDigest(row.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			if canonical != row.Canonical || id != row.ID {
				t.Fatalf("digest mismatch: canonical=%s id=%s", canonical, id)
			}
			var changed map[string]any
			_ = json.Unmarshal(row.Manifest, &changed)
			changed["built_at"] = "2030-01-01T00:00:00.000Z"
			raw, _ := json.Marshal(changed)
			_, again, err := ManifestDigest(raw)
			if err != nil || again != id {
				t.Fatal("built_at must not change build_id", err)
			}
			changed["artifacts"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			raw, _ = json.Marshal(changed)
			_, tampered, err := ManifestDigest(raw)
			if err != nil || tampered == id {
				t.Fatal("artifact change must change build_id", err)
			}
		})
	}
}

func TestSemanticBoundariesAndMonotonicTime(t *testing.T) {
	var data struct {
		Paths         []string `json:"manifest_invalid_paths"`
		Sites         []struct{ Input, Output string }
		IdentifySites []struct{ Input, Output string } `json:"identify_sites"`
		InvalidSites  []string                         `json:"invalid_sites"`
		ServerTime    string                           `json:"server_time"`
		Challenge     struct {
			ExpiresAt string `json:"expires_at"`
		}
		TimeCases []struct {
			Name      string
			RTT       int64  `json:"rtt_ms"`
			ExpiresIn int64  `json:"expires_in_ms"`
			Existing  *int64 `json:"existing_remaining_ms"`
			Want      int64  `json:"expected_remaining_ms"`
			Invalid   bool   `json:"expected_error"`
		} `json:"time_cases"`
	}
	fixture(t, "semantic-cases.json", &data)
	for _, p := range data.Paths {
		if validArtifactPath(p) {
			t.Errorf("unsafe artifact path accepted: %s", p)
		}
	}
	for _, row := range data.Sites {
		got, err := NormalizeSite(row.Input)
		if err != nil || got != row.Output {
			t.Errorf("site %s: %q, %v", row.Input, got, err)
		}
	}
	for _, site := range data.InvalidSites {
		if _, err := NormalizeSite(site); err == nil {
			t.Errorf("invalid site accepted: %s", site)
		}
	}
	for _, row := range data.IdentifySites {
		got, err := NormalizeWebsite(row.Input)
		if err != nil || got != row.Output {
			t.Errorf("identify URL: %q %v", got, err)
		}
	}
	for _, row := range data.TimeCases {
		t.Run(row.Name, func(t *testing.T) {
			got, err := RemainingMillis(data.ServerTime, data.Challenge.ExpiresAt, row.ExpiresIn, row.RTT, row.Existing)
			if row.Invalid {
				if err == nil {
					t.Fatal("invalid challenge time accepted")
				}
				return
			}
			if err != nil || got != row.Want {
				t.Fatalf("remaining=%d want=%d err=%v", got, row.Want, err)
			}
		})
	}
}

func TestStrictJSONAndManifestSemantics(t *testing.T) {
	for _, raw := range []string{`{"status":"recorded","status":"recorded"}`, `{"status":"\ud800"}`, `{"status":"recorded"} {}`} {
		if _, err := Validate("HeartbeatResponseLegacy", []byte(raw)); err == nil {
			t.Fatalf("invalid strict JSON accepted")
		}
	}
	var data struct {
		Vectors []struct{ Manifest map[string]any }
	}
	fixture(t, "golden-manifests.json", &data)
	m := data.Vectors[0].Manifest
	artifacts := m["artifacts"].([]any)
	m["artifacts"] = append(artifacts, artifacts[0])
	raw, _ := json.Marshal(m)
	if _, _, err := ManifestDigest(raw); err == nil {
		t.Fatal("duplicate artifact path accepted")
	}
}
