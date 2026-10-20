package system_recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

type configPath struct {
	Logical   string `json:"logical"`
	Directory bool   `json:"directory"`
}

func visitPaths(value reflect.Value, tag string, visit func(reflect.Value, string) error) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return visitPaths(value.Elem(), tag, visit)
	}
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			name := strings.Split(field.Tag.Get("yaml"), ",")[0]
			if e := visitPaths(value.Field(i), name, visit); e != nil {
				return e
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if e := visitPaths(value.Index(i), tag, visit); e != nil {
				return e
			}
		}
	case reflect.String:
		if strings.Contains(tag, "path") || strings.Contains(tag, "file") || strings.Contains(tag, "dir") {
			return visit(value, tag)
		}
	}
	return nil
}
func collectRuntimeFiles(cfg *config.Config, source *snapshots.Config, d Deployment, directory string) error {
	paths := map[string]configPath{}
	// 组件专用目录已有各自合同；这里只收集其余配置、目录及实际存在的辅助状态。
	v := reflect.ValueOf(cfg).Elem()
	for i := 0; i < v.NumField(); i++ {
		switch v.Type().Field(i).Name {
		case "Recovery", "SystemSnapshots", "Plugin", "Storage", "LogConfig", "Audit", "Database":
			continue
		}
		e := visitPaths(v.Field(i), "", func(field reflect.Value, tag string) error {
			original := field.String()
			if original == "" || strings.Contains(original, "://") {
				return nil
			}
			if _, ok := paths[original]; ok {
				return nil
			}
			candidates := []string{original}
			if !filepath.IsAbs(original) {
				candidates = []string{filepath.Join(d.Root, original), filepath.Join(d.Root, "backend", original), filepath.Join(filepath.Dir(d.ConfigFile), original)}
			}
			var absolute string
			var info os.FileInfo
			for _, candidate := range candidates {
				var e error
				info, e = os.Lstat(candidate)
				if e == nil {
					absolute = candidate
					break
				}
				if !errors.Is(e, os.ErrNotExist) {
					return e
				}
			}
			if absolute == "" {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("运行配置引用了软链接，无法确认完整文件边界")
			}
			logical := "core/files/" + digestPath(original)
			paths[original] = configPath{logical, info.IsDir()}
			if !info.IsDir() {
				source.ConfigFiles[logical] = absolute
				return nil
			}
			source.LifecycleRoots = append(source.LifecycleRoots, absolute)
			return filepath.WalkDir(absolute, func(file string, entry os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if entry.IsDir() {
					return nil
				}
				rel, e := filepath.Rel(absolute, file)
				if e != nil {
					return e
				}
				source.ConfigFiles[logical+"/"+filepath.ToSlash(rel)] = file
				return nil
			})
		})
		if e != nil {
			return e
		}
	}
	file := filepath.Join(directory, "runtime-path-map.json")
	if e := snapshots.WriteCoordinatorJSON(file, paths); e != nil {
		return e
	}
	source.ConfigFiles["recovery/path-map.json"] = file
	return nil
}

// Legacy online packages did not include auxiliary files. Local rehearsal relocates
// their references into the sandbox; it never borrows files from the live source.
func relocateLegacyRuntimeFiles(cfg *config.Config, root string) error {
	return visitPaths(reflect.ValueOf(cfg).Elem(), "", func(field reflect.Value, tag string) error {
		original := field.String()
		if !field.CanSet() || original == "" || strings.Contains(original, "://") {
			return nil
		}
		path := filepath.Join(root, "legacy-config", digestPath(original))
		if strings.Contains(tag, "dir") || strings.HasSuffix(tag, "path") {
			if e := os.MkdirAll(path, 0700); e != nil {
				return e
			}
		}
		field.SetString(path)
		return nil
	})
}
func restoreRuntimeFiles(cfg *config.Config, configuration map[string][]byte, root string) error {
	var paths map[string]configPath
	if e := json.Unmarshal(configuration["recovery/path-map.json"], &paths); e != nil {
		return errors.New("运行配置路径映射缺失")
	}
	for name, raw := range configuration {
		if !strings.HasPrefix(name, "core/files/") {
			continue
		}
		file := filepath.Join(root, "runtime-config", filepath.FromSlash(name))
		if !inside(root, file) {
			return errors.New("runtime configuration escaped")
		}
		if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			return e
		}
		if e := os.WriteFile(file, raw, 0600); e != nil {
			return e
		}
	}
	for _, path := range paths {
		if path.Directory {
			if e := os.MkdirAll(filepath.Join(root, "runtime-config", filepath.FromSlash(path.Logical)), 0700); e != nil {
				return e
			}
		}
	}
	return visitPaths(reflect.ValueOf(cfg).Elem(), "", func(field reflect.Value, tag string) error {
		if path, ok := paths[field.String()]; ok && field.CanSet() {
			field.SetString(filepath.Join(root, "runtime-config", filepath.FromSlash(path.Logical)))
		}
		return nil
	})
}

func digestPath(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}
