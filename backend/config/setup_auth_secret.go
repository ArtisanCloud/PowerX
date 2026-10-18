package config

import "sync"

var setupJWTSecretMu sync.Mutex

// PrepareSetupJWTSecret persists a deployment-specific signing key before any
// provisioning side effects. Failed setup retries keep the same key. It never
// rotates a key for an installed deployment.
func PrepareSetupJWTSecret(path string) error {
	_, err := prepareInstallationSecrets(path, false)
	return err
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
