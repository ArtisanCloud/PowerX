package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gopkg.in/yaml.v3"
)

func randomTestJWTSecret(t *testing.T) string {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(secret)
}

func TestJWTSecretRejectsMissingPublishedAndPlaceholderValues(t *testing.T) {
	for _, secret := range []string{"", " ", "too-short", legacyDefaultJWTSecret,
		" " + legacyDefaultJWTSecret + " ",
		"CHANGE_ME_WITH_A_RANDOM_SECRET_AT_LEAST_32_CHARS",
		"REPLACE_WITH_A_RANDOM_JWT_SECRET_AT_LEAST_32_CHARS"} {
		if err := validateJWTSecret(secret); err == nil {
			t.Fatal("unsafe JWT secret accepted")
		} else if len(secret) >= 32 && strings.Contains(err.Error(), secret) {
			t.Fatal("validation error exposes the secret")
		}
	}
	if err := validateJWTSecret(randomTestJWTSecret(t)); err != nil {
		t.Fatalf("explicit random secret rejected: %v", err)
	}
}

func TestLoadRequiresExplicitJWTSecretAndPreservesOverrides(t *testing.T) {
	t.Setenv("POWERX_ENV", "test") // Keep operator .env files out of this test.
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfig := func(secret string) {
		t.Helper()
		body := "deployment:\n  env: test\nauth:\n  jwt_secret: " + secret + "\n"
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, unsafe := range []string{"\"\"", legacyDefaultJWTSecret} {
		writeConfig(unsafe)
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "auth.jwt_secret") {
			t.Fatalf("expected JWT configuration failure, got %v", err)
		}
	}
	secret := randomTestJWTSecret(t)
	writeConfig(secret)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.JWTSecret != secret {
		t.Fatal("YAML signing secret changed during loading")
	}
	writeConfig(legacyDefaultJWTSecret)
	t.Setenv("CORE_X_AUTH_JWT_SECRET", secret)
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.JWTSecret != secret {
		t.Fatal("environment override was not preserved")
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", legacyDefaultJWTSecret)
	if _, err := Load(path); err == nil {
		t.Fatal("published secret accepted through environment override")
	}
	writeConfig("\"\"")
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", secret)
	cfg, err = Load(path)
	if err != nil || cfg.Auth.JWTSecret != secret {
		t.Fatalf("legacy environment variable compatibility changed: %v", err)
	}
}

func TestConfiguredJWTSecretRejectsLegacySignedUserAndSTSTokens(t *testing.T) {
	secret := []byte(randomTestJWTSecret(t))
	for _, identity := range []struct{ issuer, audience string }{
		{"powerx-auth", "user"}, {"powerx-sts", "powerx:api"},
	} {
		claims := reqctx.CoreXClaims{TenantUUID: "11111111-1111-1111-1111-111111111111"}
		current, err := auth.GenerateAccessJWT(claims, identity.issuer, []string{identity.audience}, time.Minute, secret)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := auth.ParseAndValidate(current, secret, identity.issuer, identity.audience); err != nil {
			t.Fatalf("valid token rejected: %v", err)
		}
		legacy, err := auth.GenerateAccessJWT(claims, identity.issuer, []string{identity.audience}, time.Minute, []byte(legacyDefaultJWTSecret))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := auth.ParseAndValidate(legacy, secret, identity.issuer, identity.audience); err == nil {
			t.Fatal("legacy signed token accepted with deployment-specific secret")
		}
	}
}

func TestShippedDefaultsAndExamplesContainNoJWTSigningSecret(t *testing.T) {
	if GetDefaults().Auth.JWTSecret != "" {
		t.Fatal("defaults include a shared JWT signing secret")
	}
	for _, path := range []string{"../etc/config_example.yaml", "../etc/config_example.prod.yaml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Auth.JWTSecret != "" || strings.Contains(string(data), legacyDefaultJWTSecret) {
			t.Fatalf("%s includes a signing secret", path)
		}
	}
}
