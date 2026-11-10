package system_recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

// EnsureEnvironmentIdentity runs before Redis/bootstrap. It reuses the existing
// installation record; first installation creates that same record and ID once.
func EnsureEnvironmentIdentity(cfg *config.Config, configFile string) error {
	if cfg.EnvironmentID != "" {
		_, err := cfg.RedisNamespace()
		return err
	}
	if err := cfg.ReadEnvironmentIdentity(configFile); err != nil {
		return err
	}
	if cfg.EnvironmentID != "" {
		return nil
	}
	file, err := filepath.Abs(configFile)
	if err != nil || configFile == "" || cfg.EnvironmentIdentityFile() == "" {
		return fmt.Errorf("environment identity requires an installation configuration")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(cfg.EnvironmentIdentityFile())))
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	directory := currentDirectory(root, file)
	if err := privateDirectory(directory); err != nil {
		return err
	}
	if err := privateDirectory(filepath.Join(directory, "locks")); err != nil {
		return err
	}
	unlock, err := (&snapshots.CoordinatorStore{Directory: directory}).Lock()
	if err != nil {
		return err
	}
	defer unlock()
	// Re-read after directory preparation to avoid replacing an already-created ID.
	var spec currentSpec
	if err := snapshots.ReadCoordinatorJSON(filepath.Join(directory, "installation.json"), &spec); err == nil {
		if spec.Protocol != CurrentRecoveryProtocol || spec.ConfigFile != file || spec.ID == "" {
			return fmt.Errorf("environment identity does not match the current installation")
		}
		cfg.EnvironmentID = spec.ID
		_, err = cfg.RedisNamespace()
		return err
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	working, err := os.Getwd()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	listen := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if strings.HasPrefix(listen, ":") {
		listen = "127.0.0.1" + listen
	}
	hash := sha256.Sum256([]byte(cfg.Database.DSN))
	spec = currentSpec{Protocol: CurrentRecoveryProtocol, ID: uuid.NewString(), ConfigFile: file, Directory: directory, SnapshotDirectory: root, Environment: cfg.Deployment.Env, Listen: listen, Root: working, Executable: executable, Args: append([]string{}, os.Args...), ConnectionHash: hex.EncodeToString(hash[:])}
	if err := snapshots.WriteCoordinatorJSON(filepath.Join(directory, "installation.json"), spec); err != nil {
		return err
	}
	cfg.EnvironmentID = spec.ID
	return nil
}

// currentSourceConfig reattaches the live environment identity after reading
// restorable YAML. Runtime identity must never be restored from snapshot data.
func (r *CurrentRuntime) currentSourceConfig(source snapshots.Config) (*config.Config, error) {
	cfg, err := sourceConfig(source)
	if err != nil {
		return nil, err
	}
	cfg.EnvironmentID = r.spec.ID
	if _, err := cfg.RedisNamespace(); err != nil {
		return nil, err
	}
	return cfg, nil
}
