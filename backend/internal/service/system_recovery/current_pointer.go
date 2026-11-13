package system_recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

// The systemd installation root is available before configuration/DB loading.
// Keep this pointer outside restorable configuration, media and release trees.
// It only locates the existing installation; it never creates a recovery epoch.
func currentPointerFile(configFile string) (string, error) {
	root := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT"))
	if root == "" {
		return configFile + ".recovery.json", nil
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", errors.New("恢复启动指针需要绝对安装根目录")
	}
	sum := sha256.Sum256([]byte(configFile))
	return filepath.Join(root, "storage", "recovery-bootstrap", hex.EncodeToString(sum[:])[:24]+".json"), nil
}

func readCurrentPointer(file, configFile string) (map[string]string, error) {
	var pointer map[string]string
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("恢复启动指针必须是私有普通文件")
	}
	if err = snapshots.ReadCoordinatorJSON(file, &pointer); err != nil {
		return nil, err
	}
	if pointer["protocol"] != CurrentRecoveryProtocol || pointer["config_file"] != configFile || !filepath.IsAbs(pointer["directory"]) {
		return nil, errors.New("当前恢复启动指针无效")
	}
	return pointer, nil
}

func saveCurrentPointer(configFile, directory string) error {
	file, err := currentPointerFile(configFile)
	if err != nil {
		return err
	}
	// Keep old installations readable, but never overwrite a conflicting pointer
	// or mask a damaged new pointer by falling back to the old one.
	for _, candidate := range []string{file, configFile + ".recovery.json"} {
		old, e := readCurrentPointer(candidate, configFile)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if old["directory"] != directory {
			return errors.New("恢复启动指针与当前安装目录不一致")
		}
	}
	if file != configFile+".recovery.json" {
		if err = privateDirectory(filepath.Dir(file)); err != nil {
			return err
		}
	}
	return snapshots.WriteCoordinatorJSON(file, map[string]string{"protocol": CurrentRecoveryProtocol, "config_file": configFile, "directory": directory})
}

func loadCurrentPointer(configFile string) (map[string]string, error) {
	file, err := currentPointerFile(configFile)
	if err != nil {
		return nil, err
	}
	if file != configFile+".recovery.json" {
		info, e := os.Lstat(filepath.Dir(file))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0) {
			return nil, errors.New("恢复启动指针目录必须是私有普通目录")
		}
	}
	pointer, err := readCurrentPointer(file, configFile)
	if file != configFile+".recovery.json" && errors.Is(err, os.ErrNotExist) {
		return readCurrentPointer(configFile+".recovery.json", configFile)
	}
	if err != nil {
		return nil, err
	}
	if file != configFile+".recovery.json" {
		old, e := readCurrentPointer(configFile+".recovery.json", configFile)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return nil, e
		}
		if e == nil && old["directory"] != pointer["directory"] {
			return nil, errors.New("恢复启动指针与当前安装目录不一致")
		}
	}
	return pointer, nil
}
