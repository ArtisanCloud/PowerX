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
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	agentcatalog "github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	agentllm "github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	agentsetting "github.com/ArtisanCloud/PowerX/internal/service/agent"
	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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

const (
	defaultRuntimeRoot = "/etc/powerx"
	defaultLinksRoot   = "/opt/powerx"
)

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

type setupAdminConfig struct {
	Username                 string `json:"username,omitempty"`
	Email                    string `json:"email,omitempty"`
	Password                 string `json:"password,omitempty"`
	DisplayName              string `json:"display_name,omitempty"`
	Phone                    string `json:"phone,omitempty"`
	TenantName               string `json:"tenant_name,omitempty"`
	TenantCode               string `json:"tenant_code,omitempty"`
	TenantDescription        string `json:"tenant_description,omitempty"`
	CreateDefaultDepartments bool   `json:"create_default_departments,omitempty"`
	CompanyName              string `json:"company_name,omitempty"`
}

type setupConfigPayload struct {
	Domain   setupDomainConfig   `json:"domain"`
	HTTPS    setupHTTPSConfig    `json:"https"`
	Storage  setupStorageConfig  `json:"storage"`
	Cache    setupCacheConfig    `json:"cache"`
	Email    setupEmailConfig    `json:"email"`
	Database setupDatabaseConfig `json:"database"`
	Admin    setupAdminConfig    `json:"admin"`
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
	saasSignupEnabled := false
	if cfg := config.GetGlobalConfig(); cfg != nil {
		installStatus = cfg.Install.EffectiveStatus()
		guardMode = cfg.Install.EffectiveLockMode()
		version = cfg.EffectiveSystemVersion()
		saasSignupEnabled = cfg.FeatureGate.EnableSaaSSignup
	}

	userCount, tenantCount, aiProfileCount := int64(0), int64(0), int64(0)
	if h.db != nil {
		userCount = h.safeCount(c.Request.Context(), coremodel.TableIAMUser)
		tenantCount = h.safeCount(c.Request.Context(), coremodel.TableIAMTenant)
		aiProfileCount = h.safeCount(c.Request.Context(), coremodel.TableAgentProviderProfiles)
	}

	completedBySetting := h.readSetupCompletedFlag(c.Request.Context())
	configured := installStatus == "installed"
	desiredPorts, desiredSource := h.resolveDesiredPorts()
	effectivePorts, effectiveSource := resolveEffectivePorts()
	restartRequired := desiredPorts.BackendPort != effectivePorts.BackendPort ||
		desiredPorts.WebAdminPort != effectivePorts.WebAdminPort
	// setup-only 启动模式下 h.db 为空；即使 install_status 已切到 installed，也需要重启后端加载完整路由与依赖。
	if installStatus == "installed" && h.db == nil {
		restartRequired = true
	}
	requiresLogin := configured && !restartRequired
	if setupStatusTraceEnabled() {
		logger.InfoF(
			c.Request.Context(),
			"setup status trace host=%s method=%s uri=%s remote=%s xff=%s xfh=%s xfp=%s install_status=%s configured=%t requires_login=%t restart_required=%t desired_ports={backend:%d,web:%d} effective_ports={backend:%d,web:%d} config_source={desired:%s,effective:%s} checks={users:%d,tenants:%d,ai_profiles:%d}",
			c.Request.Host,
			c.Request.Method,
			c.Request.RequestURI,
			c.Request.RemoteAddr,
			c.GetHeader("X-Forwarded-For"),
			c.GetHeader("X-Forwarded-Host"),
			c.GetHeader("X-Forwarded-Proto"),
			installStatus,
			configured,
			requiresLogin,
			restartRequired,
			desiredPorts.BackendPort,
			desiredPorts.WebAdminPort,
			effectivePorts.BackendPort,
			effectivePorts.WebAdminPort,
			desiredSource,
			effectiveSource,
			userCount,
			tenantCount,
			aiProfileCount,
		)
	}

	dto.ResponseSuccess(c, gin.H{
		"configured":          configured,
		"requires_login":      requiresLogin,
		"completed_by_kv":     completedBySetting,
		"install_status":      installStatus,
		"guard_mode":          guardMode,
		"version":             version,
		"saas_signup_enabled": saasSignupEnabled,
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

func setupStatusTraceEnabled() bool {
	v := strings.TrimSpace(os.Getenv("POWERX_SETUP_STATUS_TRACE"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
	if !setupWriteAllowed() {
		dto.ResponseError(c, http.StatusConflict, "系统已安装，禁止再次修改初始化向导配置", nil)
		return
	}
	var req setupConfigPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	normalizeSetupPorts(&req)
	if err := validateSetupDraftConfig(req); err != nil {
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
	if !setupWriteAllowed() {
		dto.ResponseError(c, http.StatusConflict, "系统已安装，禁止再次执行初始化", nil)
		return
	}
	status := "installed"
	if cfg := config.GetGlobalConfig(); cfg != nil {
		status = cfg.Install.EffectiveStatus()
	}

	draft, ok := h.loadDraftConfig()
	if !ok {
		if status == "installed" {
			dto.ResponseSuccess(c, gin.H{"ok": true, "configured": true, "install_status": "installed"})
			return
		}
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
	if err := runSetupProvisionSteps(runtimePath); err != nil {
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

	// setup-only 模式下（无 DB 依赖），完成安装后触发进程内自动重启，
	// 使新的 installed/端口配置立即生效，避免用户手工杀进程。
	if h.db == nil && !isGoTestProcess() {
		scheduleSetupOnlyAutoRestart(runtimePath)
	}
}

func (h *SetupHandler) Provision(c *gin.Context) {
	if !setupWriteAllowed() {
		dto.ResponseError(c, http.StatusConflict, "系统已安装，禁止再次执行数据库初始化", nil)
		return
	}
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
	if err := validateSetupDraftConfig(draft); err != nil {
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
	if err := runSetupProvisionSteps(runtimePath); err != nil {
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
	var req setupLLMTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	env := resolveSetupAIEnv(req.Env)

	svc := h.resolveAgentService(c.Request.Context())
	if svc != nil {
		if err := svc.TestConnectionPreferInput(
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
		if err := h.saveSetupVerifiedLLMCredential(c.Request.Context(), svc, env, req); err != nil {
			dto.ResponseError(c, http.StatusInternalServerError, "连接测试通过，但保存凭据失败", err)
			return
		}
		dto.ResponseSuccess(c, gin.H{"ok": true})
		return
	}

	if _, err := h.invokeLLMWithoutStore(c.Request.Context(), req, "ping", 0, 0); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "连接测试失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SetupHandler) saveSetupVerifiedLLMCredential(ctx context.Context, svc *agentsetting.AgentSettingService, env string, req setupLLMTestRequest) error {
	if svc == nil {
		return nil
	}
	p := strings.TrimSpace(req.Provider)
	if p == "" {
		return nil
	}
	scheme := "bearer"
	if m, ok := agentcatalog.GetGlobalAIRegister().Manifest(p); ok && m != nil {
		if s := strings.TrimSpace(m.Auth.Scheme); s != "" {
			scheme = s
		}
	}
	cred := &dbmodel.AIProviderCredential{
		Env:        env,
		TenantUUID: nil,
		Name:       utils.Slug(env + "-" + p),
		Provider:   p,
		AuthScheme: scheme,
		Data: datatypes.JSONMap{
			"api_key":    strings.TrimSpace(req.APIKey),
			"secret_id":  strings.TrimSpace(req.SecretID),
			"secret_key": strings.TrimSpace(req.SecretKey),
			"auth_mode":  strings.TrimSpace(req.AuthMode),
			"base_url":   strings.TrimSpace(req.BaseURL),
			"region":     strings.TrimSpace(req.Region),
		},
	}
	return svc.SaveCredentialOnly(ctx, env, nil, cred)
}

func (h *SetupHandler) TestLLMQuickCall(c *gin.Context) {
	var req setupLLMTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "请求参数格式错误", err)
		return
	}
	env := resolveSetupAIEnv(req.Env)
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "ping"
	}
	var (
		result *agentcfg.InvokeResult
		err    error
	)
	svc := h.resolveAgentService(c.Request.Context())
	if svc != nil {
		result, err = svc.QuickCallLLMResult(
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
	} else {
		result, err = h.invokeLLMWithoutStore(c.Request.Context(), req, prompt, req.Temperature, req.MaxTokens)
	}
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

func (h *SetupHandler) resolveAgentService(ctx context.Context) *agentsetting.AgentSettingService {
	if h == nil {
		return nil
	}
	if h.agentSvc != nil {
		return h.agentSvc
	}
	if h.db == nil {
		if db, err := tryOpenProvisionedDB(ctx, h); err == nil && db != nil {
			h.db = db
		}
	}
	if h.db == nil {
		return nil
	}
	if h.s == nil {
		h.s = settingsvc.NewSettingService(h.db)
	}
	h.agentSvc = agentsetting.NewAgentSettingService(h.db)
	return h.agentSvc
}

func tryOpenProvisionedDB(ctx context.Context, h *SetupHandler) (*gorm.DB, error) {
	candidates := make([]setupDatabaseConfig, 0, 2)
	if h != nil {
		if draft, ok := h.loadDraftConfig(); ok {
			candidates = append(candidates, draft.Database)
		}
	}
	if runtimeCfg, ok := loadRuntimeDatabaseConfig(); ok {
		candidates = append(candidates, runtimeCfg)
	}
	for _, cfg := range candidates {
		if strings.TrimSpace(cfg.Type) == "" {
			cfg.Type = "postgresql"
		}
		if err := validateDatabaseOnly(cfg); err != nil {
			continue
		}
		db, err := openSetupDatabase(cfg)
		if err != nil {
			continue
		}
		sqlDB, err := db.DB()
		if err != nil {
			continue
		}
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pingErr := sqlDB.PingContext(pingCtx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		_ = sqlDB.Close()
	}
	return nil, fmt.Errorf("no provisioned database available")
}

func loadRuntimeDatabaseConfig() (setupDatabaseConfig, bool) {
	path := resolveRuntimeConfigPath()
	if strings.TrimSpace(path) == "" {
		return setupDatabaseConfig{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return setupDatabaseConfig{}, false
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return setupDatabaseConfig{}, false
	}
	dbMap := asMap(root["database"])
	if len(dbMap) == 0 {
		return setupDatabaseConfig{}, false
	}

	cfg := setupDatabaseConfig{
		Type:       normalizeDBType(fmt.Sprint(dbMap["driver"])),
		Host:       strings.TrimSpace(fmt.Sprint(dbMap["host"])),
		Port:       intFromAny(dbMap["port"]),
		Name:       strings.TrimSpace(fmt.Sprint(dbMap["database"])),
		Username:   strings.TrimSpace(fmt.Sprint(dbMap["username"])),
		Password:   fmt.Sprint(dbMap["password"]),
		Charset:    strings.TrimSpace(fmt.Sprint(dbMap["charset"])),
		SSLMode:    strings.TrimSpace(fmt.Sprint(dbMap["ssl_mode"])),
		SQLitePath: strings.TrimSpace(fmt.Sprint(dbMap["dsn"])),
	}
	if cfg.Type == "" {
		cfg.Type = normalizeDBType(fmt.Sprint(dbMap["type"]))
	}
	if cfg.Type == "" {
		cfg.Type = "postgresql"
	}
	return cfg, true
}

func normalizeDBType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "postgres", "postgresql":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "sqlite":
		return "sqlite"
	default:
		return strings.ToLower(strings.TrimSpace(v))
	}
}

func intFromAny(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func setupWriteAllowed() bool {
	status := "installed"
	if cfg := config.GetGlobalConfig(); cfg != nil {
		status = cfg.Install.EffectiveStatus()
	}
	if status != "installed" {
		return true
	}
	// 仅用于紧急排障，默认禁止安装后再次写 setup。
	flag := strings.TrimSpace(strings.ToLower(os.Getenv("POWERX_ALLOW_SETUP_REENTRY")))
	return flag == "1" || flag == "true" || flag == "yes" || flag == "on"
}

func openSetupDatabase(in setupDatabaseConfig) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(in.Type))
	if driver == "" {
		driver = "postgresql"
	}
	switch driver {
	case "postgresql", "postgres":
		dsn, err := buildDatabaseDSN("postgres", in)
		if err != nil {
			return nil, err
		}
		return gorm.Open(postgres.Open(dsn), &gorm.Config{})
	case "mysql":
		dsn, err := buildDatabaseDSN("mysql", in)
		if err != nil {
			return nil, err
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "sqlite":
		return gorm.Open(sqlite.Open(strings.TrimSpace(in.SQLitePath)), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported database.type: %s", in.Type)
	}
}

func (h *SetupHandler) invokeLLMWithoutStore(
	ctx context.Context,
	req setupLLMTestRequest,
	prompt string,
	temperature float64,
	maxTokens int,
) (*agentcfg.InvokeResult, error) {
	provider := strings.TrimSpace(req.Provider)
	model := strings.TrimSpace(req.Model)
	if provider == "" {
		return nil, fmt.Errorf("provider 不能为空")
	}
	if model == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "ping"
	}
	if maxTokens < 0 {
		maxTokens = 0
	}

	baseURL := strings.TrimSpace(req.BaseURL)
	apiKey := strings.TrimSpace(req.APIKey)
	secretID := strings.TrimSpace(req.SecretID)
	secretKey := strings.TrimSpace(req.SecretKey)
	region := strings.TrimSpace(req.Region)

	if strings.EqualFold(provider, "hunyuan") {
		return invokeHunyuanWithoutStore(ctx, model, baseURL, apiKey, secretID, secretKey, region, req.AuthMode, prompt, temperature, maxTokens)
	}

	auth := agentcatalog.AuthReqFromCatalog(provider)
	baseSource := "input"
	if baseURL == "" {
		if v := agentcatalog.DefaultBaseURLForModel(provider, model); strings.TrimSpace(v) != "" {
			baseURL = strings.TrimSpace(v)
			baseSource = "model_default"
		}
	}
	// 防止非 openai provider 在 catalog 未命中/无默认地址时误回退到 OpenAI 官方地址。
	if baseURL == "" && strings.TrimSpace(auth.DefaultBaseURL) == "" && !strings.EqualFold(provider, "openai") {
		return nil, fmt.Errorf("缺少 BaseURL（%s 需要 base_url；请在 Setup 中填写 Base URL）", provider)
	}
	if auth.NeedBaseURL && baseURL == "" {
		baseURL = strings.TrimSpace(auth.DefaultBaseURL)
		baseSource = "provider_default"
		if baseURL == "" {
			return nil, fmt.Errorf("缺少 BaseURL（%s 要求 base_url）", provider)
		}
	}
	if strings.TrimSpace(req.BaseURL) == "" && baseSource == "input" && baseURL != "" {
		baseSource = "runtime_fallback"
	}
	logger.InfoF(
		ctx,
		"setup llm test resolved endpoint env=%s provider=%s model=%s source=%s base_url=%s",
		resolveSetupAIEnv(req.Env),
		provider,
		model,
		baseSource,
		baseURL,
	)
	if auth.NeedKey && apiKey == "" {
		return nil, fmt.Errorf("缺少 API Key（%s 要求 api_key）", provider)
	}
	if err := validateSetupBaseURL(baseURL); err != nil {
		return nil, err
	}

	cli, err := agentllm.NewClient(provider)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.Invoke(callCtx, &agentcfg.ModelConfig{
		Provider:     provider,
		Endpoint:     baseURL,
		APIKey:       apiKey,
		AccessToken:  apiKey,
		Model:        model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  temperature,
		MaxTokens:    maxTokens,
	}, prompt)
}

func invokeHunyuanWithoutStore(
	ctx context.Context,
	model, baseURL, apiKey, secretID, secretKey, region, authMode, prompt string,
	temperature float64,
	maxTokens int,
) (*agentcfg.InvokeResult, error) {
	mode := strings.ToLower(strings.TrimSpace(authMode))
	if mode == "" {
		mode = "openai"
	}
	defOpenAIBase, defTC3Base, defTC3Region := hunyuanDefaults()

	if mode == "tc3" {
		if baseURL == "" {
			baseURL = defTC3Base
		}
		if baseURL == "" {
			return nil, fmt.Errorf("缺少 BaseURL（hunyuan tc3 模式需要 base_url）")
		}
		if err := validateSetupBaseURL(baseURL); err != nil {
			return nil, err
		}
		if secretID == "" {
			return nil, fmt.Errorf("缺少 SecretID（hunyuan 要求 secret_id）")
		}
		if secretKey == "" {
			return nil, fmt.Errorf("缺少 SecretKey（hunyuan 要求 secret_key）")
		}
		if region == "" {
			region = defTC3Region
		}
		if region == "" {
			region = "ap-guangzhou"
		}
		cli, err := agentllm.NewClient("hunyuan")
		if err != nil {
			return nil, err
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return cli.Invoke(callCtx, &agentcfg.ModelConfig{
			Provider:     "hunyuan",
			Endpoint:     baseURL,
			SecretID:     secretID,
			SecretKey:    secretKey,
			Region:       region,
			Model:        model,
			SystemPrompt: "You are a helpful assistant.",
			Temperature:  temperature,
			MaxTokens:    maxTokens,
		}, prompt)
	}

	if baseURL == "" {
		baseURL = defOpenAIBase
	}
	if baseURL == "" {
		return nil, fmt.Errorf("缺少 BaseURL（混元 OpenAI SDK/API Key 模式需要 base_url）")
	}
	if u, e := url.Parse(baseURL); e == nil {
		host := strings.ToLower(strings.TrimSpace(u.Host))
		if strings.Contains(host, "tencentcloudapi.com") {
			baseURL = defOpenAIBase
		} else if u.Path == "" || u.Path == "/" {
			baseURL = strings.TrimRight(baseURL, "/") + "/v1"
		}
	}
	if err := validateSetupBaseURL(baseURL); err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("缺少 API Key（hunyuan OpenAI 模式需要 api_key）")
	}
	cli, err := agentllm.NewClient("openai")
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return cli.Invoke(callCtx, &agentcfg.ModelConfig{
		Provider:     "openai",
		Endpoint:     baseURL,
		APIKey:       apiKey,
		AccessToken:  apiKey,
		Model:        model,
		SystemPrompt: "You are a helpful assistant.",
		Temperature:  temperature,
		MaxTokens:    maxTokens,
	}, prompt)
}

func hunyuanDefaults() (openAIBase string, tc3Base string, tc3Region string) {
	reg := agentcatalog.GetGlobalAIRegister()
	m, ok := reg.Manifest("hunyuan")
	if !ok || m == nil {
		return "", "", ""
	}
	for _, md := range m.Auth.Modes {
		switch strings.ToLower(strings.TrimSpace(md.ID)) {
		case "openai":
			if md.Defaults != nil && strings.TrimSpace(md.Defaults["base_url"]) != "" {
				openAIBase = strings.TrimSpace(md.Defaults["base_url"])
			}
		case "tc3":
			if md.Defaults != nil {
				if strings.TrimSpace(md.Defaults["base_url"]) != "" {
					tc3Base = strings.TrimSpace(md.Defaults["base_url"])
				}
				if strings.TrimSpace(md.Defaults["region"]) != "" {
					tc3Region = strings.TrimSpace(md.Defaults["region"])
				}
			}
		}
	}
	return openAIBase, tc3Base, tc3Region
}

func validateSetupBaseURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base_url 非法: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("base_url 非法: %s", raw)
	}
	return nil
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
	// SaaS IAM 迁移必须保留 setup 完成记录；巡检只读取这些 key，不重建或删除历史安装状态。
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
	if runtimePath := resolveRuntimeConfigPath(); runtimePath != "" {
		return filepath.Join(filepath.Dir(runtimePath), "setup.wizard.config.json")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join("etc", "setup.wizard.config.json")
	}
	return filepath.Join(cwd, "etc", "setup.wizard.config.json")
}

func resolveRuntimeConfigPath() string {
	if root := resolveRuntimeRoot(); root != "" {
		if candidate := filepath.Join(root, "config.yaml"); fileExists(candidate) {
			return candidate
		}
	}
	if path := strings.TrimSpace(os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH")); path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if fileExists(path) {
			return path
		}
	}
	if path := strings.TrimSpace(config.GetGlobalConfigPath()); path != "" {
		if fileExists(path) {
			return path
		}
	}
	if path := strings.TrimSpace(os.Getenv("POWERX_CONFIG")); path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if fileExists(path) {
			return path
		}
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

func runSetupProvisionSteps(runtimePath string) error {
	migrateCmd := defaultSetupMigrateCmd()
	seedCmd := defaultSetupSeedCmd()
	type setupCmd struct {
		stage string
		cmd   string
	}
	cmds := []setupCmd{
		{stage: "migrate", cmd: migrateCmd},
		{stage: "seed", cmd: seedCmd},
	}
	for _, item := range cmds {
		if item.cmd == "" {
			continue
		}
		run := exec.Command("/bin/sh", "-lc", item.cmd)
		if workDir := resolveSetupProvisionWorkDir(runtimePath); workDir != "" {
			run.Dir = workDir
		}
		env := os.Environ()
		if rp := strings.TrimSpace(runtimePath); rp != "" {
			// 明确把 setup 当前运行配置传给子进程，避免回退读取旧 release 的 etc/config.yaml。
			env = append(env, "POWERX_CONFIG="+rp)
			env = append(env, "POWERX_SETUP_RUNTIME_CONFIG_PATH="+rp)
		}
		run.Env = env
		output, err := run.CombinedOutput()
		if err != nil {
			return fmt.Errorf("setup %s failed: %s", item.stage, summarizeSetupCommandOutput(output))
		}
	}
	return nil
}

func summarizeSetupCommandOutput(output []byte) string {
	s := strings.TrimSpace(string(output))
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	keep := make([]string, 0, 4)
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		low := strings.ToLower(l)
		if strings.Contains(low, "fatal:") ||
			strings.Contains(low, "sqlstate") ||
			strings.Contains(low, "连接数据库失败") ||
			strings.Contains(low, "database migrate tool not found") {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		if len(lines) > 6 {
			lines = lines[len(lines)-6:]
		}
		keep = make([]string, 0, len(lines))
		for _, line := range lines {
			l := strings.TrimSpace(line)
			if l != "" {
				keep = append(keep, l)
			}
		}
	}
	msg := strings.Join(keep, " | ")
	if len(msg) > 1200 {
		msg = msg[:1200] + "..."
	}
	return msg
}

func resolveSetupProvisionWorkDir(runtimePath string) string {
	candidates := make([]string, 0, 4)

	// 兼容旧布局：<backend>/etc/config.yaml -> <backend>
	rp := strings.TrimSpace(runtimePath)
	if rp != "" {
		etcDir := filepath.Dir(rp)
		if strings.TrimSpace(etcDir) != "" {
			backendDir := filepath.Dir(etcDir)
			if strings.TrimSpace(backendDir) != "" && backendDir != "." && backendDir != "/" {
				candidates = append(candidates, backendDir)
			}
		}
	}

	// 优先使用当前可执行文件所在目录（systemd 下通常是 /opt/powerx/backend）。
	if exe, err := os.Executable(); err == nil {
		if dir := strings.TrimSpace(filepath.Dir(exe)); dir != "" {
			candidates = append(candidates, dir)
		}
	}

	// 再尝试当前工作目录。
	if cwd, err := os.Getwd(); err == nil {
		if dir := strings.TrimSpace(cwd); dir != "" {
			candidates = append(candidates, dir)
		}
	}

	// systemd 软链目录兜底（可配置）。
	linksRoot := resolveLinksRoot()
	candidates = append(candidates, filepath.Join(linksRoot, "backend"))

	for _, dir := range candidates {
		if isSetupProvisionWorkDir(dir) {
			return dir
		}
	}
	return ""
}

func resolveRuntimeRoot() string {
	if root := strings.TrimSpace(os.Getenv("POWERX_RUNTIME_ROOT")); root != "" {
		return root
	}
	if cfgPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG")); cfgPath != "" {
		return strings.TrimSpace(filepath.Dir(cfgPath))
	}
	return defaultRuntimeRoot
}

func resolveLinksRoot() string {
	if root := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); root != "" {
		return root
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := strings.TrimSpace(filepath.Dir(exe))
		if strings.HasSuffix(exeDir, string(filepath.Separator)+"backend") {
			return filepath.Dir(exeDir)
		}
	}
	return defaultLinksRoot
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func isSetupProvisionWorkDir(dir string) bool {
	d := strings.TrimSpace(dir)
	if d == "" {
		return false
	}
	if fi, err := os.Stat(filepath.Join(d, "database")); err == nil && !fi.IsDir() && (fi.Mode()&0o111) != 0 {
		return true
	}
	if fi, err := os.Stat(filepath.Join(d, "backend", "cmd", "database")); err == nil && fi.IsDir() {
		return true
	}
	return false
}

func scheduleSetupOnlyAutoRestart(runtimePath string) {
	exe, err := os.Executable()
	if err != nil {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup auto-restart skipped: resolve executable failed: %v", err)
		return
	}
	workDir := resolveSetupProvisionWorkDir(runtimePath)
	if strings.TrimSpace(workDir) == "" {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			workDir = cwd
		}
	}
	if strings.TrimSpace(workDir) == "" {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup auto-restart skipped: empty workdir")
		return
	}

	// 优先在当前进程内原地 exec（保留当前终端会话与 PID 语义）。
	// 仅当不可用时，回退为后台 nohup 启动。
	if isInteractiveSession() {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup auto-restart scheduled in-place: exe=%s workdir=%s", exe, workDir)
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = os.Chdir(workDir)
			if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
				logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup in-place restart failed, fallback to detached mode: %v", err)
				if fbErr := scheduleDetachedRestart(exe, workDir); fbErr != nil {
					logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup detached restart fallback failed: %v", fbErr)
					return
				}
				time.Sleep(300 * time.Millisecond)
				os.Exit(0)
			}
		}()
		return
	}

	if err := scheduleDetachedRestart(exe, workDir); err != nil {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "admin.setup"}), "setup auto-restart skipped: detached restart failed: %v", err)
		return
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
}

func scheduleDetachedRestart(exe string, workDir string) error {
	logPath := filepath.Join(workDir, "tmp", "setup-auto-restart.log")
	pidPath := filepath.Join(workDir, "tmp", "setup-auto-restart.pid")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	script := fmt.Sprintf("cd %q && (sleep 1; nohup %q > %q 2>&1 & echo $! > %q)", workDir, exe, logPath, pidPath)
	cmd := exec.Command("/bin/sh", "-lc", script)
	if err := cmd.Start(); err != nil {
		return err
	}
	logger.InfoF(
		context.Background(),
		"setup auto-restart scheduled detached: exe=%s workdir=%s log=%s pid=%s",
		exe,
		workDir,
		logPath,
		pidPath,
	)
	return nil
}

func isInteractiveSession() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stdinInfo.Mode()&os.ModeCharDevice) != 0 && (stdoutInfo.Mode()&os.ModeCharDevice) != 0
}

func isGoTestProcess() bool {
	base := filepath.Base(strings.TrimSpace(os.Args[0]))
	return strings.HasSuffix(base, ".test")
}

func (h *SetupHandler) applySetupPortConfig(ctx context.Context, runtimePath string, payload setupConfigPayload) error {
	if h == nil || h.s == nil {
		return nil
	}
	return h.s.ApplySetupPortConfig(ctx, runtimePath, payload.Ports.BackendPort, payload.Ports.WebAdminPort)
}

func (h *SetupHandler) persistSetupLLM(ctx context.Context, payload setupConfigPayload) error {
	if h == nil {
		return nil
	}
	if !payload.LLM.Enabled {
		return nil
	}
	provider := strings.TrimSpace(payload.LLM.Provider)
	model := strings.TrimSpace(payload.LLM.Model)
	env := resolveSetupAIEnv("")
	if provider == "" || model == "" {
		return nil
	}
	svc := h.resolveAgentService(ctx)
	if svc == nil {
		return fmt.Errorf("setup ai service unavailable")
	}
	cred := &dbmodel.AIProviderCredential{
		Name:     utils.Slug(env + "-" + provider),
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
	if err := svc.SaveCredentialAndProfile(ctx, env, nil, cred, prof, true); err != nil {
		return fmt.Errorf("保存 setup LLM 配置失败: %w", err)
	}

	tenantUUID, err := h.resolveSetupTenantUUID(ctx, payload)
	if err != nil {
		logger.WarnF(
			ctx,
			"setup llm tenant scope skipped: resolve tenant failed, env=%s provider=%s model=%s err=%v",
			env,
			provider,
			model,
			err,
		)
		logger.InfoF(ctx, "setup llm saved: env=%s provider=%s model=%s scopes=system", env, provider, model)
		return nil
	}
	if strings.TrimSpace(tenantUUID) == "" {
		logger.WarnF(
			ctx,
			"setup llm tenant scope skipped: tenant not found, env=%s provider=%s model=%s",
			env,
			provider,
			model,
		)
		logger.InfoF(ctx, "setup llm saved: env=%s provider=%s model=%s scopes=system", env, provider, model)
		return nil
	}
	credTenant := &dbmodel.AIProviderCredential{
		Name:     utils.Slug(env + "-" + provider),
		Provider: provider,
		Data: datatypes.JSONMap{
			"api_key":  strings.TrimSpace(payload.LLM.APIKey),
			"base_url": strings.TrimSpace(payload.LLM.BaseURL),
		},
	}
	profTenant := &dbmodel.AIModelProfile{
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
	if err := svc.SaveCredentialAndProfile(ctx, env, &tenantUUID, credTenant, profTenant, true); err != nil {
		logger.WarnF(
			ctx,
			"setup llm tenant scope skipped: save tenant scope failed, env=%s provider=%s model=%s tenant_uuid=%s err=%v",
			env,
			provider,
			model,
			tenantUUID,
			err,
		)
		logger.InfoF(ctx, "setup llm saved: env=%s provider=%s model=%s scopes=system", env, provider, model)
		return nil
	}
	if err := svc.SetTenantCurrentAIEnv(ctx, tenantUUID, env); err != nil {
		logger.WarnF(
			ctx,
			"setup llm tenant scope partial: set tenant env failed, env=%s provider=%s model=%s tenant_uuid=%s err=%v",
			env,
			provider,
			model,
			tenantUUID,
			err,
		)
		logger.InfoF(
			ctx,
			"setup llm saved: env=%s provider=%s model=%s scopes=system,tenant tenant_uuid=%s tenant_env_set=false",
			env,
			provider,
			model,
			tenantUUID,
		)
		return nil
	}
	logger.InfoF(
		ctx,
		"setup llm saved: env=%s provider=%s model=%s scopes=system,tenant tenant_uuid=%s",
		env,
		provider,
		model,
		tenantUUID,
	)
	return nil
}

func (h *SetupHandler) resolveSetupTenantUUID(ctx context.Context, payload setupConfigPayload) (string, error) {
	if h == nil || h.db == nil {
		return "", nil
	}
	tenantKey := strings.TrimSpace(payload.Admin.TenantCode)
	if tenantKey == "" {
		tenantKey = "default"
	}

	var tenant tenantmodel.Tenant
	err := h.db.WithContext(ctx).
		Model(&tenantmodel.Tenant{}).
		Where("key = ?", tenantKey).
		First(&tenant).Error
	if err == nil {
		return tenant.UUID.String(), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	err = h.db.WithContext(ctx).
		Model(&tenantmodel.Tenant{}).
		Order("id asc").
		First(&tenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return tenant.UUID.String(), nil
}

func resolveSetupAIEnv(reqEnv string) string {
	if v := strings.TrimSpace(reqEnv); v != "" {
		return v
	}
	return "prod"
}

func defaultSetupMigrateCmd() string {
	return "if [ -x ./database ]; then ./database migrate -config \"${POWERX_CONFIG:-etc/config.yaml}\"; elif [ -d backend/cmd/database ] && command -v go >/dev/null 2>&1; then cd backend && go run ./cmd/database migrate -config \"${POWERX_CONFIG:-etc/config.yaml}\"; else echo 'database migrate tool not found (expect ./database or go toolchain in dev)' >&2; exit 127; fi"
}

func defaultSetupSeedCmd() string {
	return "if [ -x ./database ]; then ./database seed -config \"${POWERX_CONFIG:-etc/config.yaml}\"; elif [ -d backend/cmd/database ] && command -v go >/dev/null 2>&1; then cd backend && go run ./cmd/database seed -config \"${POWERX_CONFIG:-etc/config.yaml}\"; else echo 'database seed tool not found (expect ./database or go toolchain in dev)' >&2; exit 127; fi"
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
		Admin: setupAdminConfig{
			Username:                 "admin",
			TenantName:               "默认租户",
			TenantCode:               "default",
			CreateDefaultDepartments: true,
			CompanyName:              "我的公司",
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
	} else {
		// 无 setup 草稿时，期望端口应优先与当前运行配置对齐，避免默认值与 runtime 端口不一致导致误判 restart_required。
		if runtimePorts, runtimeSource := resolveEffectivePorts(); strings.Contains(runtimeSource, "runtime_config") {
			cfg.Ports = runtimePorts
			source = "runtime_config"
		}
	}
	normalizeSetupPorts(&cfg)
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

func validateSetupDraftConfig(in setupConfigPayload) error {
	return validateSetupConfigInternal(in, false)
}

func validateSetupConfig(in setupConfigPayload) error {
	return validateSetupConfigInternal(in, true)
}

func validateSetupConfigInternal(in setupConfigPayload, requireDomain bool) error {
	if requireDomain && strings.TrimSpace(in.Domain.Domain) == "" {
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
}

func defaultPortsByEnv() setupPortsConfig {
	ports := setupPortsConfig{
		BackendPort:  8080,
		WebAdminPort: 3000,
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWERX_ENV")), "dev") {
		ports.BackendPort = 8077
		ports.WebAdminPort = 3030
	}
	if v, ok := parseEnvPort("POWERX_BACKEND_PORT"); ok {
		ports.BackendPort = v
	}
	if v, ok := parseEnvPort("POWERX_WEB_ADMIN_PORT"); ok {
		ports.WebAdminPort = v
	}
	return ports
}

func parseEnvPort(key string) (int, bool) {
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
