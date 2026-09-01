package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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
		if errors.Is(err, os.ErrNotExist) {
			record.LastStatus = plugin_mgr.MigrationStatusFailed
			record.LastError = fmt.Sprintf("migration entry %q not found", resolvedEntry)
		}
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
		injectPluginSkillsDir(envMap, desc.Paths.Root)
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

	migrationEnv := sanitizePluginMigrationEnv(envMap)
	if err := m.validatePluginMigrationEnv(desc, migrationEnv); err != nil {
		record.LastStatus = plugin_mgr.MigrationStatusFailed
		record.LastError = err.Error()
		return record, plugin_mgr.Wrap(
			plugin_mgr.CodeMigrationFailed,
			err,
			plugin_mgr.WithOp("run_plugin_migrate"),
			plugin_mgr.WithPlugin(desc.Manifest.ID),
			plugin_mgr.WithVersion(desc.Manifest.Version),
		)
	}
	cmdEnv := mergeEnv(pluginMigrationBaseEnv(os.Environ()), migrationEnv)

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
	logger.InfoF(ctx, "[plugin-install] 运行迁移：plugin=%s version=%s entry=%s args=%v workdir=%s", desc.Manifest.ID, desc.Manifest.Version, resolvedEntry, record.Args, workDir)
	if envMap["POWERX_DB_DSN"] != "" {
		logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{
			"plugin_id":            desc.Manifest.ID,
			"plugin_version":       desc.Manifest.Version,
			"database_dsn_present": true,
		}), "[plugin-install] migration database binding ready")
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
			errors.New(detail),
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

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func injectPluginSkillsDir(env map[string]string, pluginRoot string) {
	if env == nil {
		return
	}
	if skillsDir := filepath.Join(strings.TrimSpace(pluginRoot), "skills"); dirExists(skillsDir) {
		env["PLUGIN_SKILLS_DIR"] = skillsDir
	}
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

func pluginMigrationBaseEnv(base []string) []string {
	allowedPrefixes := []string{
		"PATH=",
		"HOME=",
		"TMPDIR=",
		"TEMP=",
		"TMP=",
		"LANG=",
		"LC_",
		"TZ=",
		"SSL_CERT_FILE=",
		"SSL_CERT_DIR=",
		"NO_PROXY=",
		"no_proxy=",
		"HTTP_PROXY=",
		"http_proxy=",
		"HTTPS_PROXY=",
		"https_proxy=",
	}
	filtered := make([]string, 0, len(base))
	for _, item := range base {
		if pluginMigrationEnvAllowed(item, allowedPrefixes) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func pluginMigrationEnvAllowed(item string, allowedPrefixes []string) bool {
	key, _, ok := strings.Cut(item, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return false
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasSuffix(prefix, "=") {
			if strings.HasPrefix(item, prefix) {
				return true
			}
			continue
		}
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func sanitizePluginMigrationEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		key := strings.TrimSpace(k)
		if key == "" || pluginMigrationEnvDenied(key) {
			continue
		}
		out[key] = v
	}
	return out
}

func pluginMigrationEnvDenied(key string) bool {
	switch key {
	case
		"POWERX_CONFIG",
		"POWERX_RUNTIME_ROOT",
		"POWERX_SETUP_RUNTIME_CONFIG_PATH",
		"POWERX_SETUP_ALLOW_REENTRY",
		"POWERX_RELEASES_ROOT",
		"POWERX_LINKS_ROOT",
		"POWERX_STORAGE_ROOT",
		"POWERX_PLUGIN_RUNTIME_ROOT":
		return true
	default:
		return false
	}
}

func (m *managerImpl) validatePluginMigrationEnv(desc Descriptor, env map[string]string) error {
	if !pluginMigrationDelegatedMode(env) {
		return nil
	}

	pluginSchema := strings.TrimSpace(env["POWERX_PLUGIN_DB_SCHEMA"])
	dbSchema := strings.TrimSpace(env["POWERX_DB_SCHEMA"])
	if pluginSchema == "" {
		return fmt.Errorf("delegated plugin migration requires POWERX_PLUGIN_DB_SCHEMA for isolated plugin seed: plugin=%s", desc.Manifest.ID)
	}
	if dbSchema == "" {
		return fmt.Errorf("delegated plugin migration requires POWERX_DB_SCHEMA for isolated plugin seed: plugin=%s", desc.Manifest.ID)
	}
	if pluginSchema != dbSchema {
		return fmt.Errorf("delegated plugin migration schema mismatch: POWERX_PLUGIN_DB_SCHEMA=%s POWERX_DB_SCHEMA=%s plugin=%s", pluginSchema, dbSchema, desc.Manifest.ID)
	}
	if err := m.assertPluginSchemaSafeToDrop(pluginSchema); err != nil {
		return fmt.Errorf("delegated plugin migration refuses unsafe schema %q: %w", pluginSchema, err)
	}
	return nil
}

func pluginMigrationDelegatedMode(env map[string]string) bool {
	if env == nil {
		return false
	}
	providerMode := strings.ToLower(strings.TrimSpace(env["POWERX_PROVIDER_MODE"]))
	proxy := strings.TrimSpace(env["POWERX_PROXY"])
	return providerMode == "delegated" || proxy == "1" || strings.EqualFold(proxy, "true")
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
