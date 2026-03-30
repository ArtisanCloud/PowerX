package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentsetting "github.com/ArtisanCloud/PowerX/internal/service/agent"
	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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
	RedisPass string `json:"redis_password,omitempty"`
}

type setupEmailConfig struct {
	Enabled     bool   `json:"enabled"`
	SMTPHost    string `json:"smtp_host,omitempty"`
	SMTPPort    int    `json:"smtp_port,omitempty"`
	FromName    string `json:"from_name,omitempty"`
	FromAddress string `json:"from_address,omitempty"`
}

type setupDatabaseConfig struct {
	Type       string `json:"type,omitempty"` // mysql/postgresql/sqlite
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Name       string `json:"name,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Charset    string `json:"charset,omitempty"`
	SSLMode    string `json:"ssl_mode,omitempty"`
	SQLitePath string `json:"sqlite_path,omitempty"`
}

type setupLLMConfig struct {
	Enabled     bool    `json:"enabled"`
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	BaseURL     string  `json:"base_url,omitempty"`
	APIKey      string  `json:"api_key,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

type setupPortsConfig struct {
	BackendPort  int `json:"backend_port,omitempty"`
	WebAdminPort int `json:"web_admin_port,omitempty"`
}

type setupConfigPayload struct {
	Domain   setupDomainConfig   `json:"domain"`
	HTTPS    setupHTTPSConfig    `json:"https"`
	Storage  setupStorageConfig  `json:"storage"`
	Cache    setupCacheConfig    `json:"cache"`
	Email    setupEmailConfig    `json:"email"`
	Database setupDatabaseConfig `json:"database"`
	LLM      setupLLMConfig      `json:"llm"`
	Ports    setupPortsConfig    `json:"ports"`
}

type SetupHandler struct {
	db       *gorm.DB
	s        *settingsvc.SettingService
	agentSvc *agentsetting.AgentSettingService
}

func NewSetupHandler(db *gorm.DB) *SetupHandler {
	h := &SetupHandler{db: db}
	if db != nil {
		h.s = settingsvc.NewSettingService(db)
		h.agentSvc = agentsetting.NewAgentSettingService(db)
	}
	return h
}

type setupLLMTestRequest struct {
	Env         string  `json:"env"`
	Provider    string  `json:"provider" binding:"required"`
	Model       string  `json:"model" binding:"required"`
	BaseURL     string  `json:"baseURL"`
	APIKey      string  `json:"apiKey"`
	SecretID    string  `json:"secretId"`
	SecretKey   string  `json:"secretKey"`
	Region      string  `json:"region"`
	AuthMode    string  `json:"authMode"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
}

type setupDatabaseTestRequest struct {
	Database setupDatabaseConfig `json:"database"`
}

type setupCacheTestRequest struct {
	Cache setupCacheConfig `json:"cache"`
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
	desiredPorts, desiredSource := h.resolveDesiredPorts()
	effectivePorts, effectiveSource := resolveEffectivePorts()
	restartRequired := desiredPorts.BackendPort != effectivePorts.BackendPort ||
		desiredPorts.WebAdminPort != effectivePorts.WebAdminPort

	dto.ResponseSuccess(c, gin.H{
		"configured":      configured,
		"requires_login":  requiresLogin,
		"completed_by_kv": completedBySetting,
		"install_status":  installStatus,
		"guard_mode":      guardMode,
		"version":         version,
		"desired_ports": gin.H{
			"backend_port":   desiredPorts.BackendPort,
			"web_admin_port": desiredPorts.WebAdminPort,
		},
		"effective_ports": gin.H{
			"backend_port":   effectivePorts.BackendPort,
			"web_admin_port": effectivePorts.WebAdminPort,
		},
		"restart_required": restartRequired,
		"config_source": gin.H{
			"desired_ports":   desiredSource,
			"effective_ports": effectiveSource,
		},
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
	if err := h.applySetupPortConfig(c.Request.Context(), runtimePath, draft); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "写入端口配置失败", err)
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
	if err := h.persistSetupLLM(c.Request.Context(), draft); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "安装初始化失败，请修复后重试", err)
		return
	}

	if err := writeRuntimeConfig(runtimePath, draft, "installed"); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "安装完成写入失败", err)
		return
	}
	if err := h.applySetupPortConfig(c.Request.Context(), runtimePath, draft); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "写入端口配置失败", err)
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

func (h *SetupHandler) Provision(c *gin.Context) {
	status := "installed"
	if cfg := config.GetGlobalConfig(); cfg != nil {
		status = cfg.Install.EffectiveStatus()
	}
	if status == "installed" {
		dto.ResponseSuccess(c, gin.H{"ok": true, "install_status": "installed"})
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
	if err := h.applySetupPortConfig(c.Request.Context(), runtimePath, draft); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "写入端口配置失败", err)
		return
	}
	if err := runSetupProvisionSteps(); err != nil {
		_ = os.WriteFile(runtimePath, original, 0o644)
		dto.ResponseError(c, http.StatusInternalServerError, "数据库初始化失败，请检查配置后重试", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"ok":             true,
		"install_status": "configuring",
		"provisioned":    true,
	})
}

func (h *SetupHandler) TestLLMConnection(c *gin.Context) {
	if h == nil || h.agentSvc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	var req setupLLMTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "dev"
	}
	if err := h.agentSvc.TestConnectionPreferInput(
		c.Request.Context(),
		env,
		nil,
		"llm",
		req.Provider,
		req.Model,
		req.BaseURL,
		req.APIKey,
		req.SecretID,
		req.SecretKey,
		req.Region,
		req.AuthMode,
	); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "连接测试失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SetupHandler) TestLLMQuickCall(c *gin.Context) {
	if h == nil || h.agentSvc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	var req setupLLMTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "dev"
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "ping"
	}
	result, err := h.agentSvc.QuickCallLLMResult(
		c.Request.Context(),
		env,
		nil,
		req.Provider,
		req.Model,
		req.BaseURL,
		req.APIKey,
		req.SecretID,
		req.SecretKey,
		req.Region,
		req.AuthMode,
		req.Temperature,
		req.MaxTokens,
		prompt,
	)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "试跑失败", err)
		return
	}
	out := gin.H{"ok": true}
	if result != nil {
		out["text"] = result.Text
		if strings.TrimSpace(result.FinishReason) != "" {
			out["finish_reason"] = result.FinishReason
		}
		if len(result.Usage) > 0 {
			out["usage"] = result.Usage
		}
	}
	dto.ResponseSuccess(c, out)
}

func (h *SetupHandler) TestDatabaseConnection(c *gin.Context) {
	var req setupDatabaseTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	dbCfg := req.Database
	if strings.TrimSpace(dbCfg.Type) == "" {
		dbCfg.Type = "postgresql"
	}
	if err := validateDatabaseOnly(dbCfg); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := pingDatabase(ctx, dbCfg); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "数据库连接测试失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SetupHandler) TestCacheConnection(c *gin.Context) {
	var req setupCacheTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	cacheCfg := req.Cache
	cacheType := strings.ToLower(strings.TrimSpace(cacheCfg.Type))
	if cacheType == "" {
		cacheType = "redis"
	}
	if cacheType != "redis" {
		dto.ResponseSuccess(c, gin.H{"ok": true, "message": "仅 Redis 提供在线连接测试"})
		return
	}
	if strings.TrimSpace(cacheCfg.RedisHost) == "" || cacheCfg.RedisPort <= 0 {
		dto.ResponseError(c, http.StatusBadRequest, "cache.type=redis 时必须提供 redis_host/redis_port", nil)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", strings.TrimSpace(cacheCfg.RedisHost), cacheCfg.RedisPort),
		Password: strings.TrimSpace(cacheCfg.RedisPass),
		DB:       cacheCfg.RedisDB,
	})
	defer func() { _ = client.Close() }()
	if err := client.Ping(ctx).Err(); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "Redis 连接测试失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
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
	if payload.Ports.WebAdminPort > 0 {
		root["web_admin_port"] = payload.Ports.WebAdminPort
	}

	db := asMap(root["database"])
	if err := applyDatabaseConfig(db, payload.Database); err != nil {
		return err
	}
	root["database"] = db

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
	migrateCmd := strings.TrimSpace(os.Getenv("POWERX_SETUP_MIGRATE_CMD"))
	seedCmd := strings.TrimSpace(os.Getenv("POWERX_SETUP_SEED_CMD"))
	if migrateCmd == "" {
		migrateCmd = defaultSetupMigrateCmd()
	}
	if seedCmd == "" {
		seedCmd = defaultSetupSeedCmd()
	}
	cmds := []string{
		migrateCmd,
		seedCmd,
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

func (h *SetupHandler) applySetupPortConfig(ctx context.Context, runtimePath string, payload setupConfigPayload) error {
	if h == nil || h.s == nil {
		return nil
	}
	return h.s.ApplySetupPortConfig(ctx, runtimePath, payload.Ports.BackendPort, payload.Ports.WebAdminPort)
}

func (h *SetupHandler) persistSetupLLM(ctx context.Context, payload setupConfigPayload) error {
	if h == nil || h.agentSvc == nil {
		return nil
	}
	if !payload.LLM.Enabled {
		return nil
	}
	provider := strings.TrimSpace(payload.LLM.Provider)
	model := strings.TrimSpace(payload.LLM.Model)
	if provider == "" || model == "" {
		return nil
	}
	cred := &dbmodel.AIProviderCredential{
		Name:     utils.Slug("dev-" + provider),
		Provider: provider,
		Data: datatypes.JSONMap{
			"api_key":  strings.TrimSpace(payload.LLM.APIKey),
			"base_url": strings.TrimSpace(payload.LLM.BaseURL),
		},
	}
	prof := &dbmodel.AIModelProfile{
		Modality: "llm",
		Provider: provider,
		Model:    model,
		Defaults: datatypes.JSONMap{
			"temperature": payload.LLM.Temperature,
			"topP":        payload.LLM.TopP,
			"maxTokens":   payload.LLM.MaxTokens,
			"stream":      payload.LLM.Stream,
		},
		Tags: []string{"llm"},
	}
	if err := h.agentSvc.SaveCredentialAndProfile(ctx, "dev", nil, cred, prof, true); err != nil {
		return fmt.Errorf("保存 setup LLM 配置失败: %w", err)
	}
	return nil
}

func defaultSetupMigrateCmd() string {
	return "if [ -d backend/cmd/database ]; then cd backend; fi; go run ./cmd/database migrate"
}

func defaultSetupSeedCmd() string {
	return "if [ -d backend/cmd/database ]; then cd backend; fi; go run ./cmd/database seed"
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
		Database: setupDatabaseConfig{
			Type:       "postgresql",
			Host:       "localhost",
			Port:       5432,
			Name:       "powerx",
			Username:   "root",
			Charset:    "utf8mb4",
			SSLMode:    "disable",
			SQLitePath: "/data/powerx.db",
		},
		LLM: setupLLMConfig{
			Enabled:     false,
			Provider:    "openai",
			Model:       "gpt-4.1-mini",
			BaseURL:     "",
			APIKey:      "",
			Temperature: 0.7,
			TopP:        1,
			MaxTokens:   4096,
			Stream:      true,
		},
		Ports: defaultPorts,
	}
}

func (h *SetupHandler) resolveDesiredPorts() (setupPortsConfig, string) {
	cfg := defaultSetupConfig()
	source := "default"
	if draft, ok := h.loadDraftConfig(); ok {
		cfg = draft
		source = "setup_draft"
	}
	normalizeSetupPorts(&cfg)
	if _, ok := parsePortFromEnv("POWERX_BACKEND_PORT"); ok {
		source = appendPortSource(source, "env:POWERX_BACKEND_PORT")
	}
	if _, ok := parsePortFromEnv("POWERX_WEB_ADMIN_PORT"); ok {
		source = appendPortSource(source, "env:POWERX_WEB_ADMIN_PORT")
	}
	applyRuntimePortOverrides(&cfg)
	return cfg.Ports, source
}

func resolveEffectivePorts() (setupPortsConfig, string) {
	ports := defaultPortsByEnv()
	source := "default"

	runtimePath := resolveRuntimeConfigPath()
	if strings.TrimSpace(runtimePath) != "" {
		raw, err := os.ReadFile(runtimePath)
		if err == nil {
			var root map[string]any
			if yaml.Unmarshal(raw, &root) == nil {
				server := asMap(root["server"])
				if port, ok := anyToPort(server["port"]); ok {
					ports.BackendPort = port
					source = "runtime_config"
				}
				if webAdmin, ok := anyToPort(root["web_admin_port"]); ok {
					ports.WebAdminPort = webAdmin
					source = appendPortSource(source, "runtime_config:web_admin_port")
				}
			}
		}
	}

	if port, ok := parsePortFromEnv("POWERX_BACKEND_PORT"); ok {
		ports.BackendPort = port
		source = appendPortSource(source, "env:POWERX_BACKEND_PORT")
	}
	if port, ok := parsePortFromEnv("POWERX_WEB_ADMIN_PORT"); ok {
		ports.WebAdminPort = port
		source = appendPortSource(source, "env:POWERX_WEB_ADMIN_PORT")
	}
	return ports, source
}

func appendPortSource(base, addon string) string {
	b := strings.TrimSpace(base)
	a := strings.TrimSpace(addon)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if strings.Contains(b, a) {
		return b
	}
	return b + "|" + a
}

func anyToPort(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		if x > 0 && x <= 65535 {
			return x, true
		}
	case int64:
		if x > 0 && x <= 65535 {
			return int(x), true
		}
	case float64:
		iv := int(x)
		if float64(iv) == x && iv > 0 && iv <= 65535 {
			return iv, true
		}
	case string:
		if p, ok := parsePortString(x); ok {
			return p, true
		}
	}
	return 0, false
}

func parsePortString(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 || v > 65535 {
		return 0, false
	}
	return v, true
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
	if in.LLM.Enabled {
		if strings.TrimSpace(in.LLM.Provider) == "" {
			return errors.New("llm.provider 不能为空")
		}
		if strings.TrimSpace(in.LLM.Model) == "" {
			return errors.New("llm.model 不能为空")
		}
	}
	dbType := strings.ToLower(strings.TrimSpace(in.Database.Type))
	switch dbType {
	case "", "postgresql", "mysql", "sqlite":
	default:
		return errors.New("database.type 不合法，仅支持 postgresql/mysql/sqlite")
	}
	if dbType == "sqlite" {
		if strings.TrimSpace(in.Database.SQLitePath) == "" {
			return errors.New("database.type=sqlite 时必须提供 database.sqlite_path")
		}
	} else {
		if strings.TrimSpace(in.Database.Host) == "" {
			return errors.New("database.host 不能为空")
		}
		if in.Database.Port <= 0 || in.Database.Port > 65535 {
			return errors.New("database.port 必须在 1-65535")
		}
		if strings.TrimSpace(in.Database.Name) == "" {
			return errors.New("database.name 不能为空")
		}
		if strings.TrimSpace(in.Database.Username) == "" {
			return errors.New("database.username 不能为空")
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

func applyDatabaseConfig(db map[string]any, in setupDatabaseConfig) error {
	if db == nil {
		return nil
	}
	dbType := strings.ToLower(strings.TrimSpace(in.Type))
	if dbType == "" {
		dbType = "postgresql"
	}
	switch dbType {
	case "postgresql", "postgres":
		driver := "postgres"
		db["driver"] = driver
		if v := strings.TrimSpace(in.Host); v != "" {
			db["host"] = v
		}
		if in.Port > 0 {
			db["port"] = in.Port
		}
		if v := strings.TrimSpace(in.Name); v != "" {
			db["database"] = v
		}
		if v := strings.TrimSpace(in.Username); v != "" {
			db["username"] = v
		}
		if in.Password != "" {
			db["password"] = in.Password
		}
		sslMode := strings.TrimSpace(in.SSLMode)
		if sslMode == "" {
			sslMode = "disable"
		}
		db["ssl_mode"] = sslMode
		dsn, err := buildDatabaseDSN("postgres", in)
		if err != nil {
			return err
		}
		db["dsn"] = dsn
	case "mysql":
		db["driver"] = "mysql"
		if v := strings.TrimSpace(in.Host); v != "" {
			db["host"] = v
		}
		if in.Port > 0 {
			db["port"] = in.Port
		}
		if v := strings.TrimSpace(in.Name); v != "" {
			db["database"] = v
		}
		if v := strings.TrimSpace(in.Username); v != "" {
			db["username"] = v
		}
		if in.Password != "" {
			db["password"] = in.Password
		}
		dsn, err := buildDatabaseDSN("mysql", in)
		if err != nil {
			return err
		}
		db["dsn"] = dsn
	case "sqlite":
		db["driver"] = "sqlite"
		sqlitePath := strings.TrimSpace(in.SQLitePath)
		if sqlitePath == "" {
			return fmt.Errorf("database.sqlite_path 不能为空")
		}
		db["dsn"] = sqlitePath
		if v := strings.TrimSpace(in.Name); v != "" {
			db["database"] = v
		}
	default:
		return fmt.Errorf("unsupported database.type: %s", in.Type)
	}
	return nil
}

func buildDatabaseDSN(driver string, in setupDatabaseConfig) (string, error) {
	host := strings.TrimSpace(in.Host)
	name := strings.TrimSpace(in.Name)
	user := strings.TrimSpace(in.Username)
	if host == "" || name == "" || user == "" || in.Port <= 0 {
		return "", fmt.Errorf("database 配置不完整，无法生成 dsn")
	}
	switch driver {
	case "postgres":
		sslMode := strings.TrimSpace(in.SSLMode)
		if sslMode == "" {
			sslMode = "disable"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			url.QueryEscape(user),
			url.QueryEscape(in.Password),
			host,
			in.Port,
			name,
			url.QueryEscape(sslMode),
		), nil
	case "mysql":
		charset := strings.TrimSpace(in.Charset)
		if charset == "" {
			charset = "utf8mb4"
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
			user,
			in.Password,
			host,
			in.Port,
			name,
			charset,
		), nil
	default:
		return "", fmt.Errorf("unsupported driver: %s", driver)
	}
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

func validateDatabaseOnly(in setupDatabaseConfig) error {
	dbType := strings.ToLower(strings.TrimSpace(in.Type))
	switch dbType {
	case "", "postgresql", "mysql", "sqlite":
	default:
		return errors.New("database.type 不合法，仅支持 postgresql/mysql/sqlite")
	}
	if dbType == "sqlite" {
		if strings.TrimSpace(in.SQLitePath) == "" {
			return errors.New("database.type=sqlite 时必须提供 database.sqlite_path")
		}
		return nil
	}
	if strings.TrimSpace(in.Host) == "" {
		return errors.New("database.host 不能为空")
	}
	if in.Port <= 0 || in.Port > 65535 {
		return errors.New("database.port 必须在 1-65535")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("database.name 不能为空")
	}
	if strings.TrimSpace(in.Username) == "" {
		return errors.New("database.username 不能为空")
	}
	return nil
}

func pingDatabase(ctx context.Context, in setupDatabaseConfig) error {
	driver := strings.ToLower(strings.TrimSpace(in.Type))
	if driver == "" {
		driver = "postgresql"
	}
	var (
		db  *gorm.DB
		err error
	)
	switch driver {
	case "postgresql", "postgres":
		dsn, dsnErr := buildDatabaseDSN("postgres", in)
		if dsnErr != nil {
			return dsnErr
		}
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	case "mysql":
		dsn, dsnErr := buildDatabaseDSN("mysql", in)
		if dsnErr != nil {
			return dsnErr
		}
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(strings.TrimSpace(in.SQLitePath)), &gorm.Config{})
	default:
		return fmt.Errorf("unsupported database.type: %s", in.Type)
	}
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	return sqlDB.PingContext(ctx)
}
