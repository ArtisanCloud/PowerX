package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const setupCompletedKey = "platform.setup.completed"

var setupCompletedCompatKeys = []string{"platform.installed", "system.installed"}

type setupDomainConfig struct {
	Domain       string `json:"domain"`
	APISubdomain string `json:"api_subdomain,omitempty"`
	EnableCDN    bool   `json:"enable_cdn"`
	CDNDomain    string `json:"cdn_domain,omitempty"`
}

type setupHTTPSConfig struct {
	Mode        string `json:"mode"` // auto/manual/disable
	CertEmail   string `json:"cert_email,omitempty"`
	CertContent string `json:"cert_content,omitempty"`
	KeyContent  string `json:"key_content,omitempty"`
}

type setupStorageConfig struct {
	Type      string `json:"type"` // local/s3/minio/oss/cos
	LocalPath string `json:"local_path,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Region    string `json:"region,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

type setupCacheConfig struct {
	Type      string `json:"type"` // redis/memcached/file
	RedisHost string `json:"redis_host,omitempty"`
	RedisPort int    `json:"redis_port,omitempty"`
	RedisDB   int    `json:"redis_db,omitempty"`
}

type setupEmailConfig struct {
	Enabled     bool   `json:"enabled"`
	SMTPHost    string `json:"smtp_host,omitempty"`
	SMTPPort    int    `json:"smtp_port,omitempty"`
	FromName    string `json:"from_name,omitempty"`
	FromAddress string `json:"from_address,omitempty"`
}

type setupPortsConfig struct {
	BackendPort  int `json:"backend_port,omitempty"`
	WebAdminPort int `json:"web_admin_port,omitempty"`
}

type setupConfigPayload struct {
	Domain  setupDomainConfig  `json:"domain"`
	HTTPS   setupHTTPSConfig   `json:"https"`
	Storage setupStorageConfig `json:"storage"`
	Cache   setupCacheConfig   `json:"cache"`
	Email   setupEmailConfig   `json:"email"`
	Ports   setupPortsConfig   `json:"ports"`
}

type SetupHandler struct {
	db *gorm.DB
	s  *settingsvc.SettingService
}

func NewSetupHandler(db *gorm.DB) *SetupHandler {
	h := &SetupHandler{db: db}
	if db != nil {
		h.s = settingsvc.NewSettingService(db)
	}
	return h
}

func (h *SetupHandler) Status(c *gin.Context) {
	installStatus := "installed"
	guardMode := "strict"
	version := ""
	if cfg := config.GetGlobalConfig(); cfg != nil {
		installStatus = cfg.Install.EffectiveStatus()
		guardMode = cfg.Install.EffectiveLockMode()
		version = cfg.EffectiveSystemVersion()
	}

	userCount, tenantCount, aiProfileCount := int64(0), int64(0), int64(0)
	if h.db != nil {
		userCount = h.safeCount(c.Request.Context(), coremodel.TableIAMUser)
		tenantCount = h.safeCount(c.Request.Context(), coremodel.TableIAMTenant)
		aiProfileCount = h.safeCount(c.Request.Context(), coremodel.TableAgentProviderProfiles)
	}

	completedBySetting := h.readSetupCompletedFlag(c.Request.Context())
	configured := installStatus == "installed"
	requiresLogin := configured

	dto.ResponseSuccess(c, gin.H{
		"configured":      configured,
		"requires_login":  requiresLogin,
		"completed_by_kv": completedBySetting,
		"install_status":  installStatus,
		"guard_mode":      guardMode,
		"version":         version,
		"checks": gin.H{
			"users":       userCount,
			"tenants":     tenantCount,
			"ai_profiles": aiProfileCount,
		},
	})
}

func (h *SetupHandler) GetConfig(c *gin.Context) {
	cfg := defaultSetupConfig()
	if draft, ok := h.loadDraftConfig(); ok {
		cfg = draft
	}
	applyRuntimePortOverrides(&cfg)
	dto.ResponseSuccess(c, gin.H{"config": cfg})
}

func (h *SetupHandler) SaveConfig(c *gin.Context) {
	var req setupConfigPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	normalizeSetupPorts(&req)
	if err := validateSetupConfig(req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	if err := h.storeDraftConfig(req); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "保存初始化配置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true, "config": req})
}

func (h *SetupHandler) Complete(c *gin.Context) {
	status := "installed"
	if cfg := config.GetGlobalConfig(); cfg != nil {
		status = cfg.Install.EffectiveStatus()
	}
	if status == "installed" {
		dto.ResponseSuccess(c, gin.H{"ok": true, "configured": true, "install_status": "installed"})
		return
	}

	draft, ok := h.loadDraftConfig()
	if !ok {
		dto.ResponseError(c, http.StatusBadRequest, "尚未保存安装向导配置", nil)
		return
	}
	if err := validateSetupConfig(draft); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	runtimePath := resolveRuntimeConfigPath()
	if runtimePath == "" {
		dto.ResponseError(c, http.StatusInternalServerError, "未找到运行配置文件路径", nil)
		return
	}
	original, err := os.ReadFile(runtimePath)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "读取运行配置失败", err)
		return
	}

	if err := writeRuntimeConfig(runtimePath, draft, "configuring"); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "写入安装中配置失败", err)
		return
	}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWERX_SETUP_SIMULATE_PHASE2_FAIL")), "true") {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "安装初始化失败，请修复后重试", errors.New("setup phase2 simulated failure"))
		return
	}
	if err := runSetupProvisionSteps(); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "安装初始化失败，请修复后重试", err)
		return
	}

	if err := writeRuntimeConfig(runtimePath, draft, "installed"); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "安装完成写入失败", err)
		return
	}

	if cfg := config.GetGlobalConfig(); cfg != nil {
		cfg.Install.Status = "installed"
	}

	if h.s != nil {
		group := "setup"
		desc := "首次安装向导完成标记"
		editable := true
		_ = h.s.UpsertSystem(c.Request.Context(), setupCompletedKey, datatypes.JSON([]byte("true")), &group, &desc, &editable)
	}

	dto.ResponseSuccess(c, gin.H{
		"ok":             true,
		"configured":     true,
		"install_status": "installed",
	})
}

func (h *SetupHandler) safeCount(ctx context.Context, table string) int64 {
	if h.db == nil || strings.TrimSpace(table) == "" {
		return 0
	}
	var total int64
	if err := h.db.WithContext(ctx).Table(table).Count(&total).Error; err != nil {
		return 0
	}
	return total
}

func (h *SetupHandler) readSetupCompletedFlag(ctx context.Context) bool {
	if h == nil || h.s == nil {
		return false
	}
	if v := h.readBoolSystemSetting(ctx, setupCompletedKey); v {
		return true
	}
	for _, key := range setupCompletedCompatKeys {
		if v := h.readBoolSystemSetting(ctx, key); v {
			return true
		}
	}
	return false
}

func (h *SetupHandler) readBoolSystemSetting(ctx context.Context, key string) bool {
	if h.s == nil {
		return false
	}
	item, err := h.s.GetSystem(ctx, key)
	if err != nil || item == nil || len(item.ValueJSON) == 0 {
		return false
	}
	raw := strings.TrimSpace(string(item.ValueJSON))
	switch strings.ToLower(raw) {
	case "true", "\"true\"", "1", "\"1\"":
		return true
	default:
		return false
	}
}

func (h *SetupHandler) loadDraftConfig() (setupConfigPayload, bool) {
	cfg := defaultSetupConfig()
	path := resolveSetupDraftPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultSetupConfig(), false
	}
	normalizeSetupPorts(&cfg)
	return cfg, true
}

func (h *SetupHandler) storeDraftConfig(payload setupConfigPayload) error {
	path := resolveSetupDraftPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

func resolveSetupDraftPath() string {
	if p := strings.TrimSpace(os.Getenv("POWERX_SETUP_DRAFT_PATH")); p != "" {
		return p
	}
	if runtimePath := resolveRuntimeConfigPath(); runtimePath != "" {
		return filepath.Join(filepath.Dir(runtimePath), "setup.wizard.config.json")
	}
	return filepath.Join(os.TempDir(), "powerx.setup.wizard.config.json")
}

func resolveRuntimeConfigPath() string {
	if p := strings.TrimSpace(os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH")); p != "" {
		return p
	}
	if p := strings.TrimSpace(config.GetGlobalConfigPath()); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "backend", "etc", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, "etc", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return ""
}

func writeRuntimeConfig(path string, payload setupConfigPayload, installStatus string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return err
	}
	if root == nil {
		root = make(map[string]any)
	}

	server := asMap(root["server"])
	if payload.Ports.BackendPort > 0 {
		server["port"] = payload.Ports.BackendPort
	}
	root["server"] = server

	install := asMap(root["install"])
	install["status"] = installStatus
	if _, ok := install["lock_mode"]; !ok {
		install["lock_mode"] = "strict"
	}
	if _, ok := install["allow_without_db"]; !ok {
		install["allow_without_db"] = true
	}
	root["install"] = install

	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func runSetupProvisionSteps() error {
	cmds := []string{
		strings.TrimSpace(os.Getenv("POWERX_SETUP_MIGRATE_CMD")),
		strings.TrimSpace(os.Getenv("POWERX_SETUP_SEED_CMD")),
	}
	for _, cmd := range cmds {
		if cmd == "" {
			continue
		}
		run := exec.Command("/bin/sh", "-lc", cmd)
		output, err := run.CombinedOutput()
		if err != nil {
			return fmt.Errorf("run setup command failed: %s: %w; output=%s", cmd, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func asMap(v any) map[string]any {
	if out, ok := v.(map[string]any); ok {
		return out
	}
	if out, ok := v.(map[any]any); ok {
		ret := make(map[string]any, len(out))
		for k, val := range out {
			ret[fmt.Sprint(k)] = val
		}
		return ret
	}
	return make(map[string]any)
}

func defaultSetupConfig() setupConfigPayload {
	defaultPorts := defaultPortsByEnv()
	return setupConfigPayload{
		Domain: setupDomainConfig{
			Domain:       "",
			APISubdomain: "",
			EnableCDN:    false,
			CDNDomain:    "",
		},
		HTTPS: setupHTTPSConfig{
			Mode:      "auto",
			CertEmail: "",
		},
		Storage: setupStorageConfig{
			Type:      "local",
			LocalPath: "/data/uploads",
		},
		Cache: setupCacheConfig{
			Type:      "redis",
			RedisHost: "localhost",
			RedisPort: 6379,
			RedisDB:   0,
		},
		Email: setupEmailConfig{
			Enabled:  false,
			SMTPPort: 587,
		},
		Ports: defaultPorts,
	}
}

func validateSetupConfig(in setupConfigPayload) error {
	if strings.TrimSpace(in.Domain.Domain) == "" {
		return errors.New("domain.domain 不能为空")
	}
	if in.Domain.EnableCDN && strings.TrimSpace(in.Domain.CDNDomain) == "" {
		return errors.New("domain.enable_cdn=true 时必须提供 domain.cdn_domain")
	}

	mode := strings.ToLower(strings.TrimSpace(in.HTTPS.Mode))
	switch mode {
	case "auto", "manual", "disable":
	default:
		return errors.New("https.mode 仅支持 auto/manual/disable")
	}
	if mode == "manual" {
		if strings.TrimSpace(in.HTTPS.CertContent) == "" || strings.TrimSpace(in.HTTPS.KeyContent) == "" {
			return errors.New("https.mode=manual 时必须提供证书和私钥内容")
		}
	}

	storageType := strings.ToLower(strings.TrimSpace(in.Storage.Type))
	switch storageType {
	case "local", "s3", "minio", "oss", "cos":
	default:
		return errors.New("storage.type 不合法")
	}
	if storageType == "local" {
		if strings.TrimSpace(in.Storage.LocalPath) == "" {
			return errors.New("storage.type=local 时必须提供 storage.local_path")
		}
	} else {
		if strings.TrimSpace(in.Storage.AccessKey) == "" || strings.TrimSpace(in.Storage.SecretKey) == "" || strings.TrimSpace(in.Storage.Bucket) == "" {
			return errors.New("对象存储必须提供 access_key/secret_key/bucket")
		}
	}

	cacheType := strings.ToLower(strings.TrimSpace(in.Cache.Type))
	switch cacheType {
	case "redis", "memcached", "file":
	default:
		return errors.New("cache.type 不合法")
	}
	if cacheType == "redis" {
		if strings.TrimSpace(in.Cache.RedisHost) == "" || in.Cache.RedisPort <= 0 {
			return errors.New("cache.type=redis 时必须提供 redis_host/redis_port")
		}
	}

	if in.Email.Enabled {
		if strings.TrimSpace(in.Email.SMTPHost) == "" || in.Email.SMTPPort <= 0 {
			return errors.New("email.enabled=true 时必须提供 smtp_host/smtp_port")
		}
		if strings.TrimSpace(in.Email.FromAddress) == "" {
			return errors.New("email.enabled=true 时必须提供 from_address")
		}
	}
	if in.Ports.BackendPort <= 0 || in.Ports.BackendPort > 65535 {
		return errors.New("ports.backend_port 必须在 1-65535")
	}
	if in.Ports.WebAdminPort <= 0 || in.Ports.WebAdminPort > 65535 {
		return errors.New("ports.web_admin_port 必须在 1-65535")
	}
	if in.Ports.BackendPort == in.Ports.WebAdminPort {
		return errors.New("ports.backend_port 与 ports.web_admin_port 不能相同")
	}

	return nil
}

func normalizeSetupPorts(in *setupConfigPayload) {
	if in == nil {
		return
	}
	defaults := defaultPortsByEnv()
	if in.Ports.BackendPort <= 0 {
		in.Ports.BackendPort = defaults.BackendPort
	}
	if in.Ports.WebAdminPort <= 0 {
		in.Ports.WebAdminPort = defaults.WebAdminPort
	}
}

func applyRuntimePortOverrides(in *setupConfigPayload) {
	if in == nil {
		return
	}
	normalizeSetupPorts(in)
	if port, ok := parsePortFromEnv("POWERX_BACKEND_PORT"); ok {
		in.Ports.BackendPort = port
	}
	if port, ok := parsePortFromEnv("POWERX_WEB_ADMIN_PORT"); ok {
		in.Ports.WebAdminPort = port
	}
}

func defaultPortsByEnv() setupPortsConfig {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("POWERX_ENV")))
	ports := setupPortsConfig{
		BackendPort:  8080,
		WebAdminPort: 3000,
	}
	if env == "dev" {
		ports.BackendPort = 8077
		ports.WebAdminPort = 3030
	}
	return ports
}

func parsePortFromEnv(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 || v > 65535 {
		return 0, false
	}
	return v, true
}
