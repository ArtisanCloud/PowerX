package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func (m *managerImpl) runPluginMigrate(ctx context.Context, desc Descriptor, opts plugin_mgr.InstallOptions) (*plugin_mgr.MigrationRecord, error) {
	spec := desc.Manifest.Migrations
	if spec == nil {
		return nil, nil
	}
	entry := strings.TrimSpace(spec.Entry)
	if entry == "" {
		return nil, nil
	}

	record := &plugin_mgr.MigrationRecord{
		Entry:   spec.Entry,
		WorkDir: spec.WorkDir,
		Once:    spec.Once,
		Timeout: spec.Timeout,
	}
	if len(spec.Args) > 0 {
		record.Args = append([]string(nil), spec.Args...)
	}
	record.Hash = makeMigrationHash(spec.Entry, spec.Args)

	if !m.shouldExecuteMigration(desc, opts) {
		record.LastStatus = plugin_mgr.MigrationStatusSkipped
		return record, nil
	}

	resolvedEntry := desc.Paths.MigrationsEntry
	if resolvedEntry == "" {
		resolvedEntry = ResolvePath(desc.Paths.Root, spec.Entry)
	}
	if resolvedEntry == "" {
		return record, plugin_mgr.Wrap(
			plugin_mgr.CodeMigrationFailed,
			fmt.Errorf("migration entry is empty"),
			plugin_mgr.WithOp("run_plugin_migrate"),
			plugin_mgr.WithPlugin(desc.Manifest.ID),
			plugin_mgr.WithVersion(desc.Manifest.Version),
		)
	}
	if fi, err := os.Stat(resolvedEntry); err != nil {
		return record, plugin_mgr.Wrap(
			plugin_mgr.CodeMigrationFailed,
			fmt.Errorf("migration entry %q not found: %w", resolvedEntry, err),
			plugin_mgr.WithOp("run_plugin_migrate"),
			plugin_mgr.WithPlugin(desc.Manifest.ID),
			plugin_mgr.WithVersion(desc.Manifest.Version),
		)
	} else if fi.IsDir() {
		return record, plugin_mgr.Wrap(
			plugin_mgr.CodeMigrationFailed,
			fmt.Errorf("migration entry %q is a directory", resolvedEntry),
			plugin_mgr.WithOp("run_plugin_migrate"),
			plugin_mgr.WithPlugin(desc.Manifest.ID),
			plugin_mgr.WithVersion(desc.Manifest.Version),
		)
	}

	workDir := desc.Paths.MigrationsWorkDir
	if workDir == "" {
		if spec.WorkDir != "" {
			if filepath.IsAbs(spec.WorkDir) {
				workDir = spec.WorkDir
			} else {
				workDir = filepath.Join(desc.Paths.Root, spec.WorkDir)
			}
		} else {
			workDir = desc.Paths.Root
		}
	}
	if workDir != "" {
		fi, err := os.Stat(workDir)
		if err != nil {
			return record, plugin_mgr.Wrap(
				plugin_mgr.CodeMigrationFailed,
				fmt.Errorf("migration workdir %q invalid: %w", workDir, err),
				plugin_mgr.WithOp("run_plugin_migrate"),
				plugin_mgr.WithPlugin(desc.Manifest.ID),
				plugin_mgr.WithVersion(desc.Manifest.Version),
			)
		}
		if !fi.IsDir() {
			return record, plugin_mgr.Wrap(
				plugin_mgr.CodeMigrationFailed,
				fmt.Errorf("migration workdir %q is not a directory", workDir),
				plugin_mgr.WithOp("run_plugin_migrate"),
				plugin_mgr.WithPlugin(desc.Manifest.ID),
				plugin_mgr.WithVersion(desc.Manifest.Version),
			)
		}
	}

	plug := plugin_mgr.Plugin{
		ID:         desc.Manifest.ID,
		Version:    desc.Manifest.Version,
		Runtime:    desc.Manifest.Runtime,
		HostConfig: desc.HostConfig,
		Paths:      desc.Paths,
	}

	envMap := cloneEnvMap(desc.Manifest.Runtime.Env)
	hostEnv := m.hostEnvForPlugin(plug)
	for k, v := range hostEnv {
		envMap[k] = v
	}
	if desc.Paths.Root != "" {
		envMap["POWERX_PLUGIN_ROOT"] = desc.Paths.Root
	}
	if desc.Paths.ConfigDir != "" {
		envMap["POWERX_PLUGIN_CONFIG_DIR"] = desc.Paths.ConfigDir
	}
	if desc.Paths.HostValuesFile != "" {
		envMap["POWERX_PLUGIN_HOST_VALUES"] = desc.Paths.HostValuesFile
	}
	if desc.Manifest.ID != "" {
		envMap["POWERX_PLUGIN_ID"] = desc.Manifest.ID
	}
	if desc.Manifest.Version != "" {
		envMap["POWERX_PLUGIN_VERSION"] = desc.Manifest.Version
	}

	cmdEnv := mergeEnv(os.Environ(), envMap)

	runCtx := ctx
	var cancel context.CancelFunc
	if strings.TrimSpace(spec.Timeout) != "" {
		if dur, err := time.ParseDuration(spec.Timeout); err == nil && dur > 0 {
			runCtx, cancel = context.WithTimeout(ctx, dur)
			defer cancel()
		}
	}

	cmd := exec.CommandContext(runCtx, resolvedEntry, record.Args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = cmdEnv
	fmt.Printf("[plugin-install] 运行迁移：plugin=%s version=%s entry=%s args=%v workdir=%s\n", desc.Manifest.ID, desc.Manifest.Version, resolvedEntry, record.Args, workDir)
	if dsn := envMap["POWERX_DB_DSN"]; dsn != "" {
		fmt.Printf("[plugin-install] 使用数据库 DSN: %s\n", dsn)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	now := time.Now().UTC()
	record.ExecutedAt = &now
	if err != nil {
		record.LastStatus = plugin_mgr.MigrationStatusFailed
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		record.LastError = msg
		detail := fmt.Sprintf("migration entry %q failed: %v", entry, err)
		if msg != "" {
			detail = fmt.Sprintf("%s: %s", detail, msg)
		}
		return record, plugin_mgr.Wrap(
			plugin_mgr.CodeMigrationFailed,
			fmt.Errorf(detail),
			plugin_mgr.WithOp("run_plugin_migrate"),
			plugin_mgr.WithPlugin(desc.Manifest.ID),
			plugin_mgr.WithVersion(desc.Manifest.Version),
		)
	}

	record.LastStatus = plugin_mgr.MigrationStatusSuccess
	record.LastError = ""
	return record, nil
}

func (m *managerImpl) shouldExecuteMigration(desc Descriptor, opts plugin_mgr.InstallOptions) bool {
	if desc.Manifest.Migrations == nil {
		return false
	}
	if opts.RunMigrations {
		return true
	}
	run := true
	if desc.HostConfig != nil && desc.HostConfig.Spec != nil {
		if runtimeSpec, ok := desc.HostConfig.Spec["runtime"].(map[string]any); ok {
			if raw, ok := runtimeSpec["run_migrate"]; ok {
				if val, ok := toBool(raw); ok {
					run = val
				}
			}
		}
	}
	return run
}

func toBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		s := strings.TrimSpace(strings.ToLower(val))
		switch s {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	case int:
		return val != 0, true
	case int64:
		return val != 0, true
	case float64:
		return val != 0, true
	default:
		return false, false
	}
}

func mergeEnv(base []string, overrides map[string]string) []string {
	envMap := envListToMap(base)
	if envMap == nil {
		envMap = map[string]string{}
	}
	for k, v := range overrides {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		envMap[key] = v
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func makeMigrationHash(entry string, args []string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.TrimSpace(entry)))
	for _, a := range args {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(a))
	}
	return hex.EncodeToString(h.Sum(nil))
}
