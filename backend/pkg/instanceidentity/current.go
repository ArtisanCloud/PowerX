// Package instanceidentity locates the existing installation/environment record.
// It does not create another identity or expose a user-configurable namespace.
package instanceidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const CurrentProtocol = "powerx.current-recovery/v1"

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func Valid(id string) bool { return validID.MatchString(id) }

func DefaultStorage(linksRoot string) (string, string) {
	if filepath.IsAbs(linksRoot) {
		return filepath.Join(linksRoot, "storage", "system-snapshots"), "install_root"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".powerx", "storage", "system-snapshots"), "default"
}

func CurrentDirectory(snapshotDirectory, configFile string) string {
	sum := sha256.Sum256([]byte(configFile))
	return filepath.Join(snapshotDirectory, "current-recovery", hex.EncodeToString(sum[:12]))
}

// ReadCurrentID reads the same record used by environment management. Missing
// records are initialized by the normal installation lifecycle, never by a CLI read.
func ReadCurrentID(file, configFile string) (string, error) {
	raw, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("environment identity is unreadable")
	}
	var record struct {
		Protocol   string `json:"protocol"`
		ID         string `json:"id"`
		ConfigFile string `json:"config_file"`
	}
	if json.Unmarshal(raw, &record) != nil || record.Protocol != CurrentProtocol || !Valid(record.ID) || record.ConfigFile != configFile {
		return "", fmt.Errorf("environment identity does not match the current installation")
	}
	return record.ID, nil
}
