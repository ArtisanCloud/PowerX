package system_recovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

func (r *CurrentRuntime) prepareCurrentFiles(ctx context.Context, j *snapshots.ManagedJob, source snapshots.Config, staging string, configuration map[string][]byte, cfg *config.Config) (currentJournal, error) {
	journal := currentJournal{JobID: j.ID}
	preparedPaths := []string{}
	ready := false
	defer func() {
		if !ready {
			for _, path := range preparedPaths {
				_ = os.RemoveAll(path)
			}
		}
	}()
	inputs := map[string]string{}
	media := filepath.Join(staging, "media")
	if err := os.MkdirAll(media, 0700); err != nil {
		return journal, err
	}
	inputs[source.MediaRoot] = media
	core := filepath.Join(staging, "programs/core/powerx")
	raw, err := os.ReadFile(core)
	if err != nil {
		return journal, errors.New("快照缺少 Core 程序")
	}
	localAdapter := !bytes.Contains(raw, []byte(CurrentRecoveryProtocol)) || !bytes.Contains(raw, []byte(CurrentCacheProtocol))
	if !localAdapter {
		inputs[r.spec.Executable] = core
	} else {
		// 旧本地快照仍可验证数据；不执行不认识维护启动协议的旧 Core。
		j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "local_core_adapter", Passed: true, Detail: "旧开发快照不支持当前维护与缓存隔离协议，保留本轮 Core 执行器；此项不作为旧 Core 程序逐字节恢复证明"})
	}
	for _, kind := range []string{"core/web-admin", "core/platform_capabilities"} {
		target := source.Artifacts[kind]
		if target == "" {
			continue
		}
		input := filepath.Join(staging, "programs", filepath.FromSlash(kind))
		if kind == "core/platform_capabilities" {
			_, statErr := os.Lstat(input)
			if errors.Is(statErr, os.ErrNotExist) && localAdapter && r.spec.Environment == "dev" {
				// 早期开发包没有采集正式能力目录。只有保留当前 Core 作为本地
				// 维护适配器时，才允许配套保留当前目录；不得伪称历史程序完整恢复。
				physical, err := currentRestoreTarget(target)
				if err != nil {
					return journal, err
				}
				info, err := os.Lstat(physical)
				if err != nil || !info.IsDir() {
					return journal, fmt.Errorf("旧开发快照未包含正式能力目录，当前 Core 配套目录也不可用：%s；尚未覆盖当前实例", target)
				}
				j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "local_capability_adapter", Passed: true, Detail: "旧开发快照未采集正式能力目录，配套保留当前 Core 的能力目录；此项仅用于本地数据演练，不证明历史能力目录已恢复"})
				continue
			}
			if statErr != nil {
				return journal, fmt.Errorf("快照正式能力目录不可读取（programs/core/platform_capabilities），尚未覆盖当前实例：%w", statErr)
			}
		}
		inputs[target] = input
	}
	plugins := filepath.Join(staging, "programs/plugins")
	if cfg.Plugin.InstalledDir != "" {
		if err = os.MkdirAll(plugins, 0700); err != nil {
			return journal, err
		}
		for logical, data := range configuration {
			if !strings.HasPrefix(logical, "plugins/") || logical == "plugins/registry.json" {
				continue
			}
			file := filepath.Join(plugins, filepath.FromSlash(strings.TrimPrefix(logical, "plugins/")))
			if !inside(plugins, file) {
				return journal, errors.New("插件配置越界")
			}
			if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				return journal, err
			}
			if err = os.WriteFile(file, data, 0600); err != nil {
				return journal, err
			}
		}
		installed, err := filepath.Abs(cfg.Plugin.InstalledDir)
		if err != nil {
			return journal, err
		}
		cfg.Plugin.InstalledDir = installed
		inputs[installed] = plugins
	}
	if data, ok := configuration["plugins/registry.json"]; ok && cfg.Plugin.RegistryFile != "" {
		file := filepath.Join(r.work(j.ID), "registry.json")
		if err = os.WriteFile(file, data, 0600); err != nil {
			return journal, err
		}
		inputs[cfg.Plugin.RegistryFile] = file
	}
	// 辅助文件的写入目标仅来自本机已登记来源，不接受快照提供的任意宿主路径。
	for logical, target := range source.ConfigFiles {
		if !strings.HasPrefix(logical, "core/files/") {
			continue
		}
		if data, ok := configuration[logical]; ok {
			file := filepath.Join(r.work(j.ID), "config-files", filepath.FromSlash(logical))
			if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				return journal, err
			}
			if err = os.WriteFile(file, data, 0600); err != nil {
				return journal, err
			}
			inputs[target] = file
		}
	}
	configFile := filepath.Join(r.work(j.ID), "restored-config.yaml")
	if err = writeInstallationConfig(configFile, cfg); err != nil {
		return journal, err
	}
	loaded, err := config.Load(configFile)
	if err != nil {
		return journal, fmt.Errorf("恢复配置未通过当前版本校验，尚未覆盖当前实例: %w", err)
	}
	if loaded.Auth.JWTSecret != cfg.Auth.JWTSecret || loaded.Auth.Issuer != cfg.Auth.Issuer {
		return journal, errors.New("部署环境变量覆盖了快照登录配置，请先核对；当前实例尚未改变")
	}
	inputs[r.spec.ConfigFile] = configFile
	// 归一全部目标之后再检查；同一物理目录的不同系统别名不能互相绕过。
	normalized := make(map[string]string, len(inputs))
	for target, input := range inputs {
		physical, err := currentRestoreTarget(target)
		if err != nil {
			return journal, err
		}
		if _, exists := normalized[physical]; exists {
			return journal, fmt.Errorf("多个恢复目标指向同一物理路径，拒绝覆盖：%s", physical)
		}
		normalized[physical] = input
	}
	inputs = normalized
	targets := make([]string, 0, len(inputs))
	for target := range inputs {
		targets = append(targets, target)
	}
	runtimeBoundary, err := existingCurrentBoundary(r.spec.Directory)
	if err != nil {
		return journal, err
	}
	snapshotBoundary, err := existingCurrentBoundary(r.spec.SnapshotDirectory)
	if err != nil {
		return journal, err
	}
	sort.Strings(targets)
	for i, target := range targets {
		absolute := target
		if inside(absolute, runtimeBoundary) || inside(absolute, snapshotBoundary) || inside(runtimeBoundary, absolute) || inside(snapshotBoundary, absolute) {
			return journal, fmt.Errorf("业务恢复目录与快照／维护目录重叠，拒绝覆盖：%s", absolute)
		}
		for _, other := range targets[:i] {
			if inside(other, target) || inside(target, other) {
				return journal, fmt.Errorf("恢复文件目标相互包含，拒绝覆盖：%s 与 %s", other, target)
			}
		}
		parent := filepath.Dir(absolute)
		if err = os.MkdirAll(parent, 0700); err != nil {
			return journal, err
		}
		prepared := filepath.Join(parent, ".powerx-new-"+j.ID+"-"+digestPath(absolute))
		backup := filepath.Join(parent, ".powerx-before-"+j.ID+"-"+digestPath(absolute))
		if _, err = os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
			return journal, errors.New("恢复前文件保留目录已存在")
		}
		if _, err = os.Lstat(prepared); !errors.Is(err, os.ErrNotExist) {
			return journal, errors.New("恢复文件准备目录已存在")
		}
		// 复制可能中途失败；账本发布前也必须清理本次创建的相邻临时文件。
		preparedPaths = append(preparedPaths, prepared)
		if err = snapshots.CopyLocalProgram(ctx, inputs[target], prepared); err != nil {
			return journal, fmt.Errorf("准备恢复文件失败（来源：%s；目标：%s）：%w", inputs[target], target, err)
		}
		_, err = os.Lstat(absolute)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return journal, err
		}
		journal.Files = append(journal.Files, currentFile{Target: absolute, Prepared: prepared, Backup: backup, Existed: err == nil})
	}
	ready = true
	return journal, nil
}

func (r *CurrentRuntime) installCurrentFiles(ctx context.Context, journal *currentJournal) error {
	for i := range journal.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &journal.Files[i]
		if err := validatePreparedCurrentTarget(f.Target); err != nil {
			return err
		}
		f.Started = true
		if err := snapshots.WriteCoordinatorJSON(r.journalFile(journal.JobID), journal); err != nil {
			return err
		}
		if f.Existed {
			if err := os.Rename(f.Target, f.Backup); err != nil {
				return err
			}
		}
		if err := os.Rename(f.Prepared, f.Target); err != nil {
			return err
		}
		if err := syncCurrentParent(f.Target); err != nil {
			return err
		}
	}
	return nil
}
func syncCurrentParent(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (r *CurrentRuntime) revertCurrentFiles(journal currentJournal) error {
	for i := len(journal.Files) - 1; i >= 0; i-- {
		f := journal.Files[i]
		if f.Started {
			if _, err := os.Lstat(f.Backup); err == nil {
				if err = os.RemoveAll(f.Target); err != nil {
					return err
				}
				if err = os.Rename(f.Backup, f.Target); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			} else if !f.Existed {
				if err = os.RemoveAll(f.Target); err != nil {
					return err
				}
			}
			if err := syncCurrentParent(f.Target); err != nil {
				return err
			}
		}
		_ = os.RemoveAll(f.Prepared)
	}
	return nil
}
