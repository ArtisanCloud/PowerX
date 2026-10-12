package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var setupJWTSecretMu sync.Mutex

// PrepareSetupJWTSecret persists a deployment-specific signing key before any
// provisioning side effects. Failed setup retries keep the same key. It never
// rotates a key for an installed deployment.
func PrepareSetupJWTSecret(path string) error {
	setupJWTSecretMu.Lock()
	defer setupJWTSecretMu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err = yaml.Unmarshal(raw, &root); err != nil {
		return err
	}
	if root == nil {
		root = map[string]any{}
	}
	auth, _ := root["auth"].(map[string]any)
	if auth == nil {
		auth = map[string]any{}
	}
	secret, _ := auth["jwt_secret"].(string)
	// Explicit external configuration keeps its existing precedence and must be
	// valid; setup must not quietly replace a key supplied by the operator.
	override, err := jwtSecretEnvironment()
	if err != nil {
		return err
	}
	if override != "" {
		if err = validateJWTSecret(override); err != nil {
			return fmt.Errorf("setup JWT environment override is invalid: %w", err)
		}
		return nil
	}
	if validateJWTSecret(secret) == nil {
		return nil
	}
	var state struct {
		Install InstallConfig `yaml:"install"`
	}
	if err = yaml.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Install.EffectiveStatus() == "installed" {
		return fmt.Errorf("installed deployment requires an explicitly configured JWT secret")
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return fmt.Errorf("generate setup signing key: %w", err)
	}
	auth["jwt_secret"] = hex.EncodeToString(key)
	root["auth"] = auth
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	// Preserve the runtime file's owner, mode and ACL: systemd and setup can have
	// separate OS identities. Do not print or include the key in setup responses.
	return os.WriteFile(path, data, 0600)
}

func (c *Config) setupOnlyWithoutJWT() bool {
	if c == nil || !c.Install.AllowWithoutDB {
		return false
	}
	switch c.Install.EffectiveStatus() {
	case "uninstalled", "configuring":
		return true
	}
	return false
}
