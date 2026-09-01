package config

import (
	"strings"
	"testing"
)

func TestValidateDeploymentEnv(t *testing.T) {
	t.Parallel()

	for _, env := range []string{DeploymentEnvDev, DeploymentEnvTest, DeploymentEnvStaging, DeploymentEnvProd} {
		env := env
		t.Run(env, func(t *testing.T) {
			t.Parallel()
			if err := ValidateDeploymentEnv(env); err != nil {
				t.Fatalf("ValidateDeploymentEnv(%q) error = %v", env, err)
			}
		})
	}

	for _, env := range []string{"", "development", "production", "DEV", " dev", "dev "} {
		env := env
		t.Run("invalid_"+strings.ReplaceAll(env, " ", "_"), func(t *testing.T) {
			t.Parallel()
			if err := ValidateDeploymentEnv(env); err == nil {
				t.Fatalf("ValidateDeploymentEnv(%q) expected error", env)
			}
		})
	}
}

func TestValidateDeploymentIdentityAllowsUnsetOnlyBeforeInstall(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"uninstalled", "configuring"} {
		cfg := &Config{Install: InstallConfig{Status: status}}
		if err := cfg.ValidateDeploymentIdentity(); err != nil {
			t.Fatalf("status %q should allow setup to choose deployment env: %v", status, err)
		}
	}

	cfg := &Config{Install: InstallConfig{Status: "installed"}}
	if err := cfg.ValidateDeploymentIdentity(); err == nil {
		t.Fatal("installed config without deployment.env should fail")
	}
}
