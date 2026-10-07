package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearDeprecatedJWTEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET"} {
		t.Setenv(name, "") // 清理时恢复测试开始前的环境。
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
func TestJWTSecretAcceptsOnlyCanonicalEnvironmentName(t *testing.T) {
	t.Setenv("POWERX_ENV", "test")
	yamlSecret := randomTestJWTSecret(t)
	secret := randomTestJWTSecret(t)
	empty := ""
	for _, tc := range []struct {
		name, canonical, yaml, want string
		oldAuth, oldJWT             *string
		reject                      bool
	}{
		{name: "canonical", canonical: secret, yaml: yamlSecret, want: secret},
		{name: "YAML with no override", yaml: yamlSecret, want: yamlSecret},
		{name: "old auth", oldAuth: &secret, yaml: yamlSecret, reject: true},
		{name: "old JWT", oldJWT: &secret, reject: true},
		{name: "old JWT with valid YAML", oldJWT: &secret, yaml: yamlSecret, reject: true},
		{name: "equal old and new auth", canonical: secret, oldAuth: &secret, yaml: yamlSecret, reject: true},
		{name: "equal old and new JWT", canonical: secret, oldJWT: &secret, yaml: yamlSecret, reject: true},
		{name: "empty deprecated auth", canonical: secret, oldAuth: &empty, reject: true},
		{name: "empty deprecated JWT", canonical: secret, oldJWT: &empty, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearDeprecatedJWTEnvironment(t)
			t.Setenv("POWERX_AUTH_JWT_SECRET", tc.canonical)
			if tc.oldAuth != nil {
				t.Setenv("CORE_X_AUTH_JWT_SECRET", *tc.oldAuth)
			}
			if tc.oldJWT != nil {
				t.Setenv("CORE_X_JWT_SECRET", *tc.oldJWT)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			body := "deployment:\n  env: test\nauth:\n  jwt_secret: \"" + tc.yaml + "\"\n"
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "已不再支持") {
					t.Fatal("deprecated variable was not rejected")
				}
				if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), yamlSecret) {
					t.Fatal("error exposed a signing key")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Auth.JWTSecret != tc.want {
				t.Fatal("canonical key precedence changed")
			}
		})
	}
}
func TestSetupRejectsDeprecatedJWTVariablesWithoutWritingConfig(t *testing.T) {
	t.Setenv("POWERX_ENV", "test")
	clearDeprecatedJWTEnvironment(t)
	secret := randomTestJWTSecret(t)
	t.Setenv("POWERX_AUTH_JWT_SECRET", secret)
	path := setupSecretTestPath(t, "uninstalled", "\"\"")
	if err := PrepareSetupJWTSecret(path); err != nil {
		t.Fatal(err)
	}
	if readSetupSecret(t, path) != "" {
		t.Fatal("setup replaced externally managed identity")
	}
	loaded, err := Load(path)
	if err != nil || loaded.Auth.JWTSecret != secret {
		t.Fatal("setup and loader disagree")
	}
	for _, name := range []string{"CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET"} {
		t.Run(name, func(t *testing.T) {
			clearDeprecatedJWTEnvironment(t)
			t.Setenv(name, secret)
			before, _ := os.ReadFile(path)
			if err := PrepareSetupJWTSecret(path); err == nil {
				t.Fatal("setup accepted a deprecated variable")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("rejected setup modified configuration")
			}
		})
	}
}

func TestShippedJWTEnvironmentTemplateUsesOnlyCanonicalName(t *testing.T) {
	raw, err := os.ReadFile("../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET"} {
		if strings.Contains(string(raw), name) {
			t.Fatal("environment template contains a rejected JWT name")
		}
	}
	if !strings.Contains(string(raw), "POWERX_AUTH_JWT_SECRET=") {
		t.Fatal("canonical JWT variable missing from template")
	}
}
