package plugincredential

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// PrepareInstall 处理安装产物已丢失、数据库仍有凭证的场景。
// 仅准备当前插件的持久 STS 凭证；不伪造安装版本、host-values 或已启用状态。
func (s *RuntimeCredentialRepairService) PrepareInstall(ctx context.Context, opts RuntimeCredentialRepairOptions) (*RuntimeCredentialRepairResult, error) {
	if s.db == nil || s.cfg == nil {
		return nil, fmt.Errorf("runtime credential preparation requires database and config")
	}
	if err := config.ValidateDeploymentEnv(s.cfg.Deployment.Env); err != nil {
		return nil, err
	}
	opts.PluginID = strings.TrimSpace(opts.PluginID)
	if strings.TrimSpace(s.cfg.Plugin.InstalledDir) == "" || strings.TrimSpace(s.cfg.Plugin.RegistryFile) == "" {
		return nil, fmt.Errorf("plugin installed root and registry path required")
	}
	root, err := filepath.Abs(s.cfg.Plugin.InstalledDir)
	if err != nil {
		return nil, err
	}
	durablePath, err := runtimecredential.Path(root, opts.PluginID)
	if err != nil {
		return nil, err
	}
	pluginRoot := filepath.Join(root, opts.PluginID)
	if info, err := os.Lstat(pluginRoot); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, fmt.Errorf("credential preparation requires a regular plugin directory: %s", pluginRoot)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if _, _, err := resolveRepairDirectories(root, opts.PluginID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	hosts, err := filepath.Glob(filepath.Join(pluginRoot, "*", "config", "host-values.yaml"))
	if err != nil {
		return nil, err
	}
	if len(hosts) > 0 {
		return nil, fmt.Errorf("installed host-values exist; use repair-plugin-runtime-credentials")
	}
	db := s.db.Session(&gorm.Session{Logger: gormlog.Default.LogMode(gormlog.Silent)}).WithContext(ctx)
	tenantUUID, record, cc, recordErr := readRepairRuntimeRecord(ctx, db, opts.PluginID)
	if recordErr != nil && (tenantUUID == "" || !errors.Is(recordErr, gorm.ErrRecordNotFound)) {
		return nil, recordErr
	}
	result := &RuntimeCredentialRepairResult{
		DeploymentEnv: s.cfg.Deployment.Env, Database: s.cfg.Database.Database, TenantUUID: tenantUUID,
		PluginID: opts.PluginID, ClientID: cc.ClientID, SecretVersion: cc.SecretVersion,
		CurrentConfigAvailable: false, NextStep: "install_plugin_package", HostValuesFiles: []string{},
	}
	if db.Dialector.Name() == "postgres" {
		if err := db.Raw("SELECT current_database()").Scan(&result.Database).Error; err != nil {
			return nil, err
		}
	}
	if record == nil {
		// 没有旧凭证时，正常安装会自行创建；这个工具不代替首次安装签发。
		result.Action = "install_required"
		return result, nil
	}
	registryPath, err := filepath.Abs(s.cfg.Plugin.RegistryFile)
	if err != nil {
		return nil, err
	}
	registryFile, err := readCredentialRepairFile(registryPath, true)
	if err != nil {
		return nil, err
	}
	durableFile, err := readCredentialRepairFile(durablePath, true)
	if err != nil {
		return nil, err
	}
	files := []*credentialRepairFile{durableFile}
	var registry map[string]any
	var versionConfigs []map[string]any
	var candidates []runtimecredential.Credential
	if saved, err := runtimecredential.Read(durablePath); err == nil {
		candidates = append(candidates, *saved)
	}
	if registryFile.Exists {
		if err := json.Unmarshal(registryFile.Raw, &registry); err != nil || registry == nil {
			return nil, fmt.Errorf("plugin registry is invalid JSON")
		}
		plugins, _ := registry["plugins"].(map[string]any)
		p, _ := plugins[opts.PluginID].(map[string]any)
		versions, _ := p["versions"].(map[string]any)
		result.CurrentVersion, _ = p["current"].(string)
		for version, rawVersion := range versions {
			v, _ := rawVersion.(map[string]any)
			manifest, _ := v["manifest"].(map[string]any)
			if manifest["id"] != opts.PluginID || manifest["version"] != version {
				return nil, fmt.Errorf("registry manifest identity mismatch for version=%s", version)
			}
			hc, _ := v["host_config"].(map[string]any)
			if hc == nil {
				hc = map[string]any{}
				v["host_config"] = hc
			}
			values, _ := hc["values"].(map[string]any)
			candidates = append(candidates, credentialFromRepairEnv(values, tenantUUID))
			versionConfigs = append(versionConfigs, hc)
		}
		// 只有已有插件版本记录时才更新注册表，不创建不存在的安装记录。
		if len(versionConfigs) > 0 {
			files = append(files, registryFile)
		}
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
		result.Action, result.SecretVersion = "rotate", cc.SecretVersion
	} else {
		result.Action = "restore"
	}
	cred := runtimecredential.Credential{TenantUUID: tenantUUID, ClientID: cc.ClientID, ClientSecret: secret}
	durableFile.Next, _ = json.MarshalIndent(cred, "", "  ")
	healthy := false
	if saved, err := runtimecredential.Read(durablePath); err == nil && *saved == cred {
		healthy = true
	}
	for _, hc := range versionConfigs {
		values, _ := hc["values"].(map[string]any)
		if credentialFromRepairEnv(values, "") != cred {
			healthy = false
		}
		if values == nil {
			values = map[string]any{}
			hc["values"] = values
		}
		setRepairCredentialEnv(values, cred)
	}
	if len(versionConfigs) > 0 {
		registryFile.Next, err = json.MarshalIndent(registry, "", "  ")
		if err != nil {
			return result, err
		}
	}
	if result.Action == "restore" && healthy {
		result.Action = "healthy"
		return result, nil
	}
	if !opts.Confirm {
		return result, nil
	}
	if err := s.requireStoppedCore(ctx); err != nil {
		return result, err
	}
	_, rootStatErr := os.Lstat(root)
	_, pluginStatErr := os.Lstat(pluginRoot)
	if err := os.MkdirAll(pluginRoot, 0o750); err != nil {
		return result, err
	}
	completed := false
	defer func() {
		if completed || result.RecoveryNeeded {
			return
		}
		if os.IsNotExist(pluginStatErr) {
			_ = os.Remove(pluginRoot)
		}
		if os.IsNotExist(rootStatErr) {
			_ = os.Remove(root)
		}
	}()
	backupParent := filepath.Dir(registryPath)
	if err := os.MkdirAll(backupParent, 0o750); err != nil {
		return result, err
	}
	if err := s.apply(ctx, db, record, cc, files, result, backupParent); err != nil {
		return result, err
	}
	completed = true
	result.Applied = true
	return result, nil
}

func readRepairRuntimeRecord(ctx context.Context, db *gorm.DB, pluginID string) (string, *settingmodel.PluginInstanceConfig, setting.ClientCredential, error) {
	var cc setting.ClientCredential
	var tenant tenantmodel.Tenant
	if err := db.Where("key = ?", tenantmodel.SystemTenantKey).First(&tenant).Error; err != nil {
		return "", nil, cc, fmt.Errorf("read existing system tenant: %w", err)
	}
	if tenant.UUID == uuid.Nil || tenant.Status != tenantmodel.TenantStatusActive {
		return "", nil, cc, fmt.Errorf("active system tenant required")
	}
	tenantUUID := tenant.UUID.String()
	record, err := settingrepo.NewPluginInstanceConfigRepository(db).Get(ctx, tenantUUID, pluginID, setting.KeyClientCredentials)
	if err != nil {
		return tenantUUID, nil, cc, err
	}
	if record == nil {
		return tenantUUID, nil, cc, gorm.ErrRecordNotFound
	}
	if err := json.Unmarshal(record.ValueJSON, &cc); err != nil || cc.ClientID != pluginID+"."+tenantUUID || cc.ClientSecretHash == "" {
		return tenantUUID, nil, cc, fmt.Errorf("runtime credential record is malformed or bound to another identity")
	}
	if !record.Enabled || record.Status != settingmodel.PluginInstanceStatusEnabled {
		return tenantUUID, nil, cc, fmt.Errorf("runtime credential is disabled or draining; preparation does not change authorization")
	}
	if cc.ExpiresAt != nil && time.Now().Unix() > *cc.ExpiresAt {
		return tenantUUID, nil, cc, fmt.Errorf("runtime credential is expired; preparation does not change expiry")
	}
	if !credentialAllows(cc.AllowedAudiences, "powerx:api") || !credentialAllows(cc.AllowedScopes, "access") {
		return tenantUUID, nil, cc, fmt.Errorf("runtime credential audience or scope is not allowed")
	}
	return tenantUUID, record, cc, nil
}
