package plugincredential

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlog "gorm.io/gorm/logger"
)

type RuntimeCredentialRepairOptions struct {
	PluginID string
	Confirm  bool
	Rotate   bool // 仅在无法恢复有效 secret 时允许轮换。
}

// RuntimeCredentialRepairResult 只返回身份、状态和路径，不返回 secret 或 hash。
type RuntimeCredentialRepairResult struct {
	DeploymentEnv          string   `json:"deployment_env"`
	Database               string   `json:"database"`
	TenantUUID             string   `json:"tenant_uuid"`
	PluginID               string   `json:"plugin_id"`
	CurrentVersion         string   `json:"current_version"`
	CurrentConfigAvailable bool     `json:"current_config_available"`
	NextStep               string   `json:"next_step"`
	ClientID               string   `json:"client_id"`
	SecretVersion          int      `json:"secret_version"`
	Action                 string   `json:"action"`
	Applied                bool     `json:"applied"`
	BackupDir              string   `json:"backup_dir,omitempty"`
	RecoveryNeeded         bool     `json:"recovery_needed,omitempty"`
	HostValuesFiles        []string `json:"host_values_files"`
}

type RuntimeCredentialRepairService struct {
	db    *gorm.DB
	cfg   *config.Config
	write func(string, []byte, os.FileMode) error
}

func NewRuntimeCredentialRepairService(db *gorm.DB, cfg *config.Config) *RuntimeCredentialRepairService {
	return &RuntimeCredentialRepairService{db: db, cfg: cfg, write: runtimecredential.AtomicWrite}
}

type credentialRepairFile struct {
	Path   string      `json:"path"`
	Raw    []byte      `json:"raw"`
	Mode   os.FileMode `json:"mode"`
	Exists bool        `json:"exists"`
	Next   []byte      `json:"-"`
}

type credentialRepairHost struct {
	file   *credentialRepairFile
	doc    map[string]any
	record map[string]any
}

// Repair 是显式离线运维操作；默认只预览，执行前必须停止对应 Core 服务。
// DB 事务失败会补偿已写文件；进程中断时保留 0700 备份目录用于恢复。
func (s *RuntimeCredentialRepairService) Repair(ctx context.Context, opts RuntimeCredentialRepairOptions) (*RuntimeCredentialRepairResult, error) {
	if s.db == nil || s.cfg == nil {
		return nil, fmt.Errorf("runtime credential repair requires database and config")
	}
	opts.PluginID = strings.TrimSpace(opts.PluginID)
	root, err := filepath.Abs(s.cfg.Plugin.InstalledDir)
	if err != nil || strings.TrimSpace(s.cfg.Plugin.InstalledDir) == "" {
		return nil, fmt.Errorf("plugin installed directory required")
	}
	durablePath, err := runtimecredential.Path(root, opts.PluginID)
	if err != nil {
		return nil, err
	}
	env := s.cfg.Deployment.Env
	if err := config.ValidateDeploymentEnv(env); err != nil {
		return nil, err
	}
	if _, _, err := resolveRepairDirectories(root, opts.PluginID); err != nil {
		return nil, err
	}
	// 凭证写入 SQL 不进入日志，避免错误路径泄漏 hash。
	db := s.db.Session(&gorm.Session{Logger: gormlog.Default.LogMode(gormlog.Silent)}).WithContext(ctx)
	tenantUUID, record, cc, err := readRepairRuntimeRecord(ctx, db, opts.PluginID)
	if err != nil {
		return nil, err
	}
	registryPath, err := filepath.Abs(s.cfg.Plugin.RegistryFile)
	if err != nil || strings.TrimSpace(s.cfg.Plugin.RegistryFile) == "" {
		return nil, fmt.Errorf("plugin registry file required")
	}
	registryFile, err := readCredentialRepairFile(registryPath, false)
	if err != nil {
		return nil, err
	}
	var registry map[string]any
	if err := json.Unmarshal(registryFile.Raw, &registry); err != nil {
		return nil, fmt.Errorf("plugin registry is invalid JSON")
	}
	plugins, _ := registry["plugins"].(map[string]any)
	pluginRecord, _ := plugins[opts.PluginID].(map[string]any)
	versions, _ := pluginRecord["versions"].(map[string]any)
	current, _ := pluginRecord["current"].(string)
	if len(versions) == 0 || versions[current] == nil {
		return nil, fmt.Errorf("installed plugin and current version required in registry")
	}
	result := &RuntimeCredentialRepairResult{
		DeploymentEnv: env, Database: s.cfg.Database.Database, TenantUUID: tenantUUID,
		PluginID: opts.PluginID, CurrentVersion: current, ClientID: cc.ClientID, SecretVersion: cc.SecretVersion,
		CurrentConfigAvailable: true, NextStep: "enable_plugin",
	}
	if db.Dialector.Name() == "postgres" {
		if err := db.Raw("SELECT current_database()").Scan(&result.Database).Error; err != nil {
			return nil, fmt.Errorf("read actual database: %w", err)
		}
	}
	durableFile, err := readCredentialRepairFile(durablePath, true)
	if err != nil {
		return nil, err
	}
	files := []*credentialRepairFile{registryFile, durableFile}
	hosts := make([]credentialRepairHost, 0, len(versions))
	var candidates []runtimecredential.Credential
	if durableFile.Exists {
		var saved runtimecredential.Credential
		if json.Unmarshal(durableFile.Raw, &saved) == nil {
			candidates = append(candidates, saved)
		}
	}
	versionNames := make([]string, 0, len(versions))
	for version := range versions {
		versionNames = append(versionNames, version)
	}
	sort.Strings(versionNames)
	for _, version := range versionNames {
		if filepath.Base(version) != version || version == "." || version == ".." {
			return nil, fmt.Errorf("invalid installed version")
		}
		v, _ := versions[version].(map[string]any)
		manifest, _ := v["manifest"].(map[string]any)
		if manifest["id"] != opts.PluginID || manifest["version"] != version {
			return nil, fmt.Errorf("registry manifest identity mismatch for version=%s", version)
		}
		path := filepath.Join(root, opts.PluginID, version, "config", "host-values.yaml")
		// 不接受注册表指定的任意路径，也不跟随逃出当前插件目录的软链接。
		actualDir, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return nil, fmt.Errorf("PLUGIN_RUNTIME_CONFIG_DIR_UNAVAILABLE: version=%s path=%s: %w", version, filepath.Dir(path), err)
		}
		_, pluginRoot, err := resolveRepairDirectories(root, opts.PluginID)
		if err != nil {
			return nil, err
		}
		if !repairDirectoryContains(pluginRoot, actualDir) {
			return nil, fmt.Errorf("PLUGIN_RUNTIME_CONFIG_DIR_OUTSIDE_PLUGIN: version=%s path=%s resolved=%s plugin_root=%s", version, filepath.Dir(path), actualDir, pluginRoot)
		}
		paths, _ := v["paths"].(map[string]any)
		if declared, ok := paths["host_values_file"].(string); ok && declared != "" && filepath.Clean(declared) != path {
			return nil, fmt.Errorf("registry host-values path mismatch for version=%s", version)
		}
		file, err := readCredentialRepairFile(path, false)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := yaml.Unmarshal(file.Raw, &doc); err != nil || doc == nil {
			return nil, fmt.Errorf("host-values invalid YAML for version=%s", version)
		}
		envDoc, _ := doc["env"].(map[string]any)
		candidates = append(candidates, credentialFromRepairEnv(envDoc, tenantUUID))
		hc, _ := v["host_config"].(map[string]any)
		if hc == nil {
			hc = map[string]any{}
			v["host_config"] = hc
		}
		values, _ := hc["values"].(map[string]any)
		candidates = append(candidates, credentialFromRepairEnv(values, tenantUUID))
		hosts = append(hosts, credentialRepairHost{file: file, doc: doc, record: hc})
		files = append(files, file)
		result.HostValuesFiles = append(result.HostValuesFiles, path)
	}
	var secret string
	for _, candidate := range candidates {
		if candidate.TenantUUID == tenantUUID && candidate.ClientID == cc.ClientID && candidate.ClientSecret != "" &&
			bcrypt.CompareHashAndPassword([]byte(cc.ClientSecretHash), []byte(candidate.ClientSecret)) == nil {
			secret = candidate.ClientSecret
			break
		}
	}
	if secret == "" {
		result.Action = "rotation_required"
		if !opts.Confirm {
			return result, nil
		}
		if !opts.Rotate {
			return result, fmt.Errorf("no recoverable credential; explicit -rotate is required with -confirm")
		}
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return result, err
		}
		secret = base64.RawURLEncoding.EncodeToString(random)
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return result, err
		}
		cc.ClientSecretHash = string(hash)
		cc.SecretVersion++
		now := time.Now().Unix()
		cc.RotatedAt = &now
		result.SecretVersion = cc.SecretVersion
		result.Action = "rotate"
	} else {
		result.Action = "restore"
	}
	cred := runtimecredential.Credential{TenantUUID: tenantUUID, ClientID: cc.ClientID, ClientSecret: secret}
	durableFile.Next, _ = json.MarshalIndent(cred, "", "  ")
	for _, host := range hosts {
		envDoc, _ := host.doc["env"].(map[string]any)
		if envDoc == nil {
			envDoc = map[string]any{}
			host.doc["env"] = envDoc
		}
		setRepairCredentialEnv(envDoc, cred)
		host.file.Next, err = yaml.Marshal(host.doc)
		if err != nil {
			return result, fmt.Errorf("encode repaired host-values")
		}
		// 从文件合并完整 env，保留注册表与宿主配置的其他字段。
		values, _ := host.record["values"].(map[string]any)
		if values == nil {
			values = map[string]any{}
		}
		for key, value := range envDoc {
			values[key] = value
		}
		setRepairCredentialEnv(values, cred)
		host.record["values"] = values
		host.record["values_file"] = host.file.Path
	}
	registryFile.Next, err = json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return result, fmt.Errorf("encode repaired registry")
	}
	// 比较语义而非 YAML/JSON 排版，重复执行不写文件、不轮换。
	if result.Action == "restore" && repairFilesHaveCredential(registryFile.Raw, durableFile.Raw, hosts, cred) {
		result.Action = "healthy"
		return result, nil
	}
	if !opts.Confirm {
		return result, nil
	}
	if err := s.requireStoppedCore(ctx); err != nil {
		return result, err
	}
	if err := s.apply(ctx, db, record, cc, files, result, filepath.Dir(registryPath)); err != nil {
		return result, err
	}
	result.Applied = true
	return result, nil
}

func resolveRepairDirectories(root, pluginID string) (string, string, error) {
	actualRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("PLUGIN_RUNTIME_INSTALLED_ROOT_UNAVAILABLE: path=%s: %w", root, err)
	}
	pluginPath := filepath.Join(root, pluginID)
	actualPluginRoot, err := filepath.EvalSymlinks(pluginPath)
	if err != nil {
		return "", "", fmt.Errorf("PLUGIN_RUNTIME_PLUGIN_DIR_UNAVAILABLE: plugin=%s path=%s: %w", pluginID, pluginPath, err)
	}
	if !repairDirectoryContains(actualRoot, actualPluginRoot) {
		return "", "", fmt.Errorf("PLUGIN_RUNTIME_PLUGIN_DIR_OUTSIDE_INSTALLED_ROOT: plugin=%s path=%s resolved=%s installed_root=%s", pluginID, pluginPath, actualPluginRoot, actualRoot)
	}
	return actualRoot, actualPluginRoot, nil
}

func repairDirectoryContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func (s *RuntimeCredentialRepairService) requireStoppedCore(ctx context.Context) error {
	port := config.ResolveEffectivePorts(s.cfg).BackendPort
	if port <= 0 {
		return fmt.Errorf("configured backend port required for offline repair")
	}
	host := s.cfg.Server.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	conn, err := (&net.Dialer{Timeout: 500 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("Core backend port is listening; stop the matching Core service before credential repair")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (s *RuntimeCredentialRepairService) apply(ctx context.Context, db *gorm.DB, record *settingmodel.PluginInstanceConfig, cc setting.ClientCredential, files []*credentialRepairFile, result *RuntimeCredentialRepairResult, backupParent string) error {
	nextJSON, _ := json.Marshal(cc)
	written := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var locked settingmodel.PluginInstanceConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, record.ID).Error; err != nil {
			return err
		}
		if !credentialJSONEqual(locked.ValueJSON, record.ValueJSON) || locked.Enabled != record.Enabled || locked.Status != record.Status {
			return fmt.Errorf("credential changed concurrently; preview again")
		}
		for _, file := range files {
			current, err := readCredentialRepairFile(file.Path, !file.Exists)
			if err != nil || current.Exists != file.Exists || !bytes.Equal(current.Raw, file.Raw) {
				return fmt.Errorf("runtime file changed concurrently: %s", file.Path)
			}
		}
		backupDir, err := os.MkdirTemp(backupParent, ".runtime-credential-repair-*")
		if err != nil {
			return err
		}
		result.BackupDir = backupDir
		backup, _ := json.MarshalIndent(struct {
			PluginID   string                  `json:"plugin_id"`
			TenantUUID string                  `json:"tenant_uuid"`
			Credential json.RawMessage         `json:"credential_record"`
			Files      []*credentialRepairFile `json:"files"`
		}{result.PluginID, result.TenantUUID, json.RawMessage(record.ValueJSON), files}, "", "  ")
		if err := runtimecredential.AtomicWrite(filepath.Join(backupDir, "backup.json"), backup, 0o600); err != nil {
			return err
		}
		if result.Action == "rotate" {
			if err := tx.Model(&locked).Update("value_json", datatypes.JSON(nextJSON)).Error; err != nil {
				return err
			}
		}
		meta, _ := json.Marshal(map[string]any{"action": result.Action, "deployment_env": result.DeploymentEnv, "secret_version": result.SecretVersion, "backup_dir": backupDir, "actor_os_uid": os.Getuid()})
		if err := tx.Create(&auditmodel.AuditEvent{
			OccurredAt: time.Now(), TenantUUID: result.TenantUUID, Source: "database_cli",
			Operation: "PLUGIN_RUNTIME_CREDENTIAL_REPAIR", ResourceType: "plugin", ResourceID: result.PluginID,
			Outcome: "SUCCESS", Severity: "WARN", ChangesRedacted: true, Meta: datatypes.JSON(meta),
		}).Error; err != nil {
			return fmt.Errorf("record credential repair audit: %w", err)
		}
		written = true
		for _, file := range files {
			if err := s.write(file.Path, file.Next, 0o600); err != nil {
				return fmt.Errorf("write repaired runtime file %s: %w", file.Path, err)
			}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	if written {
		// Commit 的结果若不明确，先重读；不盲目把已提交的新凭证文件回滚成旧 secret。
		var stored settingmodel.PluginInstanceConfig
		if checkErr := db.WithContext(context.WithoutCancel(ctx)).First(&stored, record.ID).Error; checkErr != nil {
			result.RecoveryNeeded = true
			return fmt.Errorf("repair result uncertain; keep service stopped and inspect backup=%s: %w", result.BackupDir, err)
		}
		if !credentialJSONEqual(stored.ValueJSON, record.ValueJSON) {
			result.RecoveryNeeded = true
			return fmt.Errorf("database may have committed or changed; keep files and inspect backup=%s: %w", result.BackupDir, err)
		}
		for _, file := range files {
			var restoreErr error
			if file.Exists {
				restoreErr = runtimecredential.AtomicWrite(file.Path, file.Raw, file.Mode)
			} else {
				restoreErr = os.Remove(file.Path)
				if os.IsNotExist(restoreErr) {
					restoreErr = nil
				}
			}
			if restoreErr != nil {
				result.RecoveryNeeded = true
				err = errors.Join(err, fmt.Errorf("file rollback failed for %s: %w", file.Path, restoreErr))
			}
		}
	}
	return err
}

func readCredentialRepairFile(path string, allowMissing bool) (*credentialRepairFile, error) {
	file := &credentialRepairFile{Path: path, Mode: 0o600}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && allowMissing {
		return file, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("runtime repair requires regular file: %s", path)
	}
	file.Exists, file.Mode = true, info.Mode().Perm()
	file.Raw, err = os.ReadFile(path)
	return file, err
}

func credentialFromRepairEnv(env map[string]any, tenantUUID string) runtimecredential.Credential {
	id, _ := env["POWERX_STS_CLIENT_ID"].(string)
	secret, _ := env["POWERX_STS_CLIENT_SECRET"].(string)
	if declared, _ := env["POWERX_GRPC_UPSTREAM_TENANT_UUID"].(string); declared != "" {
		tenantUUID = declared
	}
	return runtimecredential.Credential{TenantUUID: tenantUUID, ClientID: id, ClientSecret: secret}
}

func setRepairCredentialEnv(env map[string]any, cred runtimecredential.Credential) {
	env["POWERX_STS_CLIENT_ID"] = cred.ClientID
	env["POWERX_STS_CLIENT_SECRET"] = cred.ClientSecret
	env["POWERX_GRPC_UPSTREAM_TENANT_UUID"] = cred.TenantUUID
}

func credentialAllows(allowed []string, want string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if item == want {
			return true
		}
	}
	return false
}

func credentialJSONEqual(first, second []byte) bool {
	var a, b any
	return json.Unmarshal(first, &a) == nil && json.Unmarshal(second, &b) == nil && reflect.DeepEqual(a, b)
}

func repairFilesHaveCredential(registryRaw, durableRaw []byte, hosts []credentialRepairHost, cred runtimecredential.Credential) bool {
	var saved runtimecredential.Credential
	if json.Unmarshal(durableRaw, &saved) != nil || saved != cred {
		return false
	}
	var registry map[string]any
	if json.Unmarshal(registryRaw, &registry) != nil {
		return false
	}
	plugins, _ := registry["plugins"].(map[string]any)
	pluginID := strings.TrimSuffix(cred.ClientID, "."+cred.TenantUUID)
	p, _ := plugins[pluginID].(map[string]any)
	versions, _ := p["versions"].(map[string]any)
	for _, host := range hosts {
		var original map[string]any
		if yaml.Unmarshal(host.file.Raw, &original) != nil {
			return false
		}
		env, _ := original["env"].(map[string]any)
		version := filepath.Base(filepath.Dir(filepath.Dir(host.file.Path)))
		v, _ := versions[version].(map[string]any)
		hc, _ := v["host_config"].(map[string]any)
		values, _ := hc["values"].(map[string]any)
		if credentialFromRepairEnv(env, "") != cred || credentialFromRepairEnv(values, "") != cred {
			return false
		}
	}
	return true
}
