package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func setupSecretTestPath(t *testing.T, state, secret string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "deployment:\n  env: test\ninstall:\n  status: " + state + "\n  allow_without_db: true\nauth:\n  jwt_secret: " + secret + "\ncustom_marker: preserved\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func readSetupSecret(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err = yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Auth.JWTSecret
}
func TestSetupOnlyLoadsWithoutSigningKeyButInstalledFailsClosed(t *testing.T) {
	t.Setenv("POWERX_ENV", "test")
	t.Setenv("POWERX_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_AUTH_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"uninstalled", "configuring"} {
		path := setupSecretTestPath(t, state, "\"\"")
		if _, err := Load(path); err != nil {
			t.Fatalf("setup-only load failed: %v", err)
		}
		if readSetupSecret(t, path) != "" {
			t.Fatal("loading configuration mutated its key")
		}
	}
	path := setupSecretTestPath(t, "installed", "\"\"")
	if _, err := Load(path); err == nil {
		t.Fatal("installed deployment accepted a missing signing key")
	}
	path = setupSecretTestPath(t, "uninstalled", "\"\"")
	raw, _ := os.ReadFile(path)
	raw = []byte(strings.Replace(string(raw), "allow_without_db: true", "allow_without_db: false", 1))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("full runtime accepted a missing signing key")
	}
}
func TestSetupSigningKeyPersistedOnceAndUniquePerDeployment(t *testing.T) {
	t.Setenv("POWERX_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_AUTH_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	previous := ""
	for _, old := range []string{"\"\"", legacyDefaultJWTSecret, "CHANGE_ME_WITH_A_RANDOM_SECRET_AT_LEAST_32_CHARS"} {
		path := setupSecretTestPath(t, "uninstalled", old)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- PrepareSetupJWTSecret(path) }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		secret := readSetupSecret(t, path)
		if len(secret) != 64 || validateJWTSecret(secret) != nil || secret == previous {
			t.Fatal("deployment key is invalid or shared")
		}
		previous = secret
		if err := PrepareSetupJWTSecret(path); err != nil {
			t.Fatal(err)
		}
		if readSetupSecret(t, path) != secret {
			t.Fatal("retry rotated the signing key")
		}
		raw, _ := os.ReadFile(path)
		if !strings.Contains(string(raw), "custom_marker: preserved") {
			t.Fatal("unrelated configuration was lost")
		}
	}
}
func TestSetupPreservesExistingKeysAndRejectsInvalidExternalOverride(t *testing.T) {
	t.Setenv("POWERX_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_AUTH_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	valid := randomTestJWTSecret(t)
	path := setupSecretTestPath(t, "installed", valid)
	before, _ := os.ReadFile(path)
	if err := PrepareSetupJWTSecret(path); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing valid key was rewritten")
	}
	path = setupSecretTestPath(t, "installed", legacyDefaultJWTSecret)
	if err := PrepareSetupJWTSecret(path); err == nil {
		t.Fatal("installed deployment key was silently rotated")
	}
	if readSetupSecret(t, path) != legacyDefaultJWTSecret {
		t.Fatal("installed key changed")
	}
	path = setupSecretTestPath(t, "uninstalled", "\"\"")
	t.Setenv("POWERX_AUTH_JWT_SECRET", valid)
	if err := PrepareSetupJWTSecret(path); err != nil {
		t.Fatal(err)
	}
	if readSetupSecret(t, path) != "" {
		t.Fatal("external key management was overwritten")
	}
	t.Setenv("POWERX_AUTH_JWT_SECRET", legacyDefaultJWTSecret)
	if err := PrepareSetupJWTSecret(path); err == nil {
		t.Fatal("invalid operator override was silently accepted")
	}
}
