package customer

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const ClientAppMiniProgram = "mini_program"

type ClientAppSecretCodec interface {
	SealSensitive(context.Context, string, string, datatypes.JSONMap, ...string) (datatypes.JSONMap, error)
	UnsealSensitive(context.Context, string, string, datatypes.JSONMap, any) error
}

type ClientApplicationService struct {
	db     *gorm.DB
	repo   *repo.ClientApplicationRepository
	keys   ClientAppSecretCodec
	mu     sync.RWMutex
	legacy []ClientApp
}

func NewClientApplicationService(db *gorm.DB, keys ClientAppSecretCodec) *ClientApplicationService {
	if keys == nil {
		keys = tenantkeys.NewTenantKeyService(db)
	}
	return &ClientApplicationService{db: db, repo: repo.NewClientApplicationRepository(db), keys: keys}
}

// SetLegacyApplications 仅提供显式迁移来源，不参与运行时身份认证回退。
func (s *ClientApplicationService) SetLegacyApplications(apps []ClientApp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.legacy = append([]ClientApp(nil), apps...)
}

type ClientAppConnectionInput struct {
	UUID      string  `json:"uuid,omitempty"`
	Provider  string  `json:"provider"`
	AppID     string  `json:"app_id"`
	AppSecret *string `json:"app_secret,omitempty"`
}
type ClientApplicationInput struct {
	ExpectedVersion int64                      `json:"expected_version"`
	Name            string                     `json:"name"`
	Kind            string                     `json:"kind"`
	Status          string                     `json:"status"`
	PluginIDs       []string                   `json:"plugin_ids"`
	Connections     []ClientAppConnectionInput `json:"connections"`
}
type ClientAppConnectionView struct {
	UUID             string `json:"uuid"`
	Provider         string `json:"provider"`
	Platform         string `json:"platform"`
	AppID            string `json:"app_id"`
	SecretConfigured bool   `json:"secret_configured"`
}
type ClientApplicationView struct {
	UUID        string                    `json:"uuid"`
	Name        string                    `json:"name"`
	Kind        string                    `json:"kind"`
	Status      string                    `json:"status"`
	Version     int64                     `json:"version"`
	PluginIDs   []string                  `json:"plugin_ids"`
	Connections []ClientAppConnectionView `json:"connections"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}
type ClientAppProvider struct {
	ID        string `json:"id"`
	Supported bool   `json:"supported"`
}
type LegacyClientAppView struct {
	Platform         string `json:"platform"`
	AppID            string `json:"app_id"`
	Name             string `json:"name"`
	SecretConfigured bool   `json:"secret_configured"`
}
type ClientAppCatalog struct {
	Kinds              []ClientAppProvider   `json:"kinds"`
	Providers          []ClientAppProvider   `json:"providers"`
	PluginIDs          []string              `json:"plugin_ids"`
	LegacyApplications []LegacyClientAppView `json:"legacy_applications"`
}

func clientAppError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}
func clientAppStorageError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return clientAppError(404, "CLIENT_APP_NOT_FOUND")
	}
	return clientAppError(503, "CLIENT_APP_STORAGE_UNAVAILABLE")
}
func validAppUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil
}

func appView(app model.ClientApplication, bindings []model.ClientAppBinding) ClientApplicationView {
	v := ClientApplicationView{UUID: app.UUID.String(), Name: app.Name, Kind: app.Kind, Status: app.Status, Version: app.Version, PluginIDs: []string{}, Connections: []ClientAppConnectionView{}, UpdatedAt: app.UpdatedAt}
	_ = json.Unmarshal(app.PluginIDs, &v.PluginIDs)
	for _, b := range bindings {
		if b.ApplicationUUID == v.UUID {
			v.Connections = append(v.Connections, ClientAppConnectionView{UUID: b.UUID.String(), Provider: b.Provider, Platform: b.Platform, AppID: b.AppID, SecretConfigured: len(b.SealedSecret) > 0})
		}
	}
	return v
}

func (s *ClientApplicationService) Catalog(ctx context.Context) (ClientAppCatalog, error) {
	tenant, _, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_client_apps_catalog", "list")
	if err != nil {
		return ClientAppCatalog{}, err
	}
	plugins, err := s.repo.TenantPlugins(ctx, tenant)
	if err != nil {
		return ClientAppCatalog{}, clientAppStorageError(err)
	}
	c := ClientAppCatalog{Kinds: []ClientAppProvider{{ClientAppMiniProgram, true}, {"mobile_app", false}, {"web_app", false}}, Providers: []ClientAppProvider{{"wechat", true}, {"alipay", false}, {"douyin", false}}, PluginIDs: plugins, LegacyApplications: []LegacyClientAppView{}}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.legacy {
		if !legacyTenantAllowed(ctx, a, tenant) {
			continue
		}
		c.LegacyApplications = append(c.LegacyApplications, LegacyClientAppView{Platform: a.Platform, AppID: a.AppID, Name: a.Name, SecretConfigured: a.Secret != ""})
	}
	return c, nil
}

func (s *ClientApplicationService) List(ctx context.Context, kind, provider, q string, page, size int) ([]ClientApplicationView, int64, error) {
	tenant, _, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_client_apps", "list")
	if err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 || len(q) > 128 {
		return nil, 0, clientAppError(400, "CLIENT_APP_INVALID_ARGUMENT")
	}
	apps, total, err := s.repo.List(ctx, tenant, kind, provider, strings.TrimSpace(q), page, size)
	if err != nil {
		return nil, 0, clientAppStorageError(err)
	}
	ids := []string{}
	for _, a := range apps {
		ids = append(ids, a.UUID.String())
	}
	bindings, err := s.repo.Bindings(ctx, tenant, ids)
	if err != nil {
		return nil, 0, clientAppStorageError(err)
	}
	views := make([]ClientApplicationView, 0, len(apps))
	for _, a := range apps {
		views = append(views, appView(a, bindings))
	}
	return views, total, nil
}

func (s *ClientApplicationService) Get(ctx context.Context, id string) (ClientApplicationView, error) {
	tenant, _, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_client_apps_app_uuid", "read")
	if err != nil {
		return ClientApplicationView{}, err
	}
	if !validAppUUID(id) {
		return ClientApplicationView{}, clientAppError(400, "CLIENT_APP_INVALID_ARGUMENT")
	}
	a, err := s.repo.Get(ctx, tenant, id, false)
	if err != nil {
		return ClientApplicationView{}, clientAppStorageError(err)
	}
	b, err := s.repo.Bindings(ctx, tenant, []string{id})
	if err != nil {
		return ClientApplicationView{}, clientAppStorageError(err)
	}
	return appView(a, b), nil
}

func (s *ClientApplicationService) validate(ctx context.Context, tenant string, in *ClientApplicationInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || strings.ContainsAny(in.Name, "\x00\r\n\t") || len([]rune(in.Name)) > 128 || len(in.Connections) != 1 || len(in.PluginIDs) == 0 || len(in.PluginIDs) > 64 || (in.Status != "active" && in.Status != "disabled") {
		return clientAppError(400, "CLIENT_APP_INVALID_ARGUMENT")
	}
	if in.Kind != ClientAppMiniProgram || in.Connections[0].Provider != "wechat" {
		return clientAppError(400, "CLIENT_APP_PROVIDER_UNSUPPORTED")
	}
	c := &in.Connections[0]
	c.AppID = strings.TrimSpace(c.AppID)
	if c.AppID == "" || len(c.AppID) > 128 || strings.ContainsAny(c.AppID, " \t\r\n/\\") {
		return clientAppError(400, "CLIENT_APP_INVALID_ARGUMENT")
	}
	if c.AppSecret != nil {
		v := strings.TrimSpace(*c.AppSecret)
		if v == "" || len(v) > 64 || strings.IndexFunc(v, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
			return clientAppError(400, "CLIENT_APP_SECRET_INVALID")
		}
		c.AppSecret = &v
	}
	allowed, err := s.repo.TenantPlugins(ctx, tenant)
	if err != nil {
		return clientAppStorageError(err)
	}
	seen := map[string]bool{}
	for i, id := range in.PluginIDs {
		id = strings.TrimSpace(id)
		in.PluginIDs[i] = id
		if seen[id] || !slices.Contains(allowed, id) {
			return clientAppError(403, "CLIENT_APP_PLUGIN_FORBIDDEN")
		}
		seen[id] = true
	}
	sort.Strings(in.PluginIDs)
	return nil
}

func (s *ClientApplicationService) Save(ctx context.Context, id string, in ClientApplicationInput) (ClientApplicationView, error) {
	resource, action := "admin_customers_client_apps", "create"
	if id != "" {
		resource, action = "admin_customers_client_apps_app_uuid", "update"
	}
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, resource, action)
	if err != nil {
		return ClientApplicationView{}, err
	}
	if id != "" && !validAppUUID(id) {
		return ClientApplicationView{}, clientAppError(400, "CLIENT_APP_INVALID_ARGUMENT")
	}
	if err = s.validate(ctx, tenant, &in); err != nil {
		return ClientApplicationView{}, err
	}
	if id == "" && (in.ExpectedVersion != 0 || in.Connections[0].UUID != "") || id != "" && in.ExpectedVersion < 1 {
		return ClientApplicationView{}, clientAppError(400, "CLIENT_APP_VERSION_REQUIRED")
	}
	c := in.Connections[0]
	var sealed datatypes.JSONMap
	if c.AppSecret != nil {
		sealed, err = s.keys.SealSensitive(ctx, "default", tenant, datatypes.JSONMap{"app_secret": *c.AppSecret}, "app_secret")
		if err != nil {
			return ClientApplicationView{}, clientAppError(503, "CLIENT_APP_SECRET_UNAVAILABLE")
		}
	}
	var view ClientApplicationView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r := repo.NewClientApplicationRepository(tx)
		// 写入时再次检查插件归属，不能仅依赖表单载入时的选项。
		plugins, e := r.TenantPlugins(ctx, tenant)
		if e != nil {
			return e
		}
		for _, p := range in.PluginIDs {
			if !slices.Contains(plugins, p) {
				return clientAppError(403, "CLIENT_APP_PLUGIN_FORBIDDEN")
			}
		}
		grants, _ := json.Marshal(in.PluginIDs)
		var a model.ClientApplication
		var bindings []model.ClientAppBinding
		if id == "" {
			a = model.ClientApplication{TenantUUID: tenant, Name: in.Name, Kind: in.Kind, Status: in.Status, Version: 1, PluginIDs: datatypes.JSON(grants)}
			bindings = []model.ClientAppBinding{{TenantUUID: tenant, Provider: c.Provider, Platform: WeChatMiniAppChannel, AppID: c.AppID, SealedSecret: sealed}}
			if in.Status == "active" && len(sealed) == 0 {
				return clientAppError(400, "CLIENT_APP_SECRET_REQUIRED")
			}
			if e = r.Create(ctx, &a, bindings); e != nil {
				return e
			}
			bindings, e = r.Bindings(ctx, tenant, []string{a.UUID.String()})
			if e != nil {
				return e
			}
		} else {
			a, e = r.Get(ctx, tenant, id, true)
			if e != nil {
				return e
			}
			if a.Version != in.ExpectedVersion {
				return clientAppError(409, "CLIENT_APP_VERSION_CONFLICT")
			}
			bindings, e = r.Bindings(ctx, tenant, []string{id})
			if e != nil {
				return e
			}
			if a.Kind != in.Kind || len(bindings) != 1 || bindings[0].AppID != c.AppID || bindings[0].Provider != c.Provider || bindings[0].UUID.String() != c.UUID {
				return clientAppError(400, "CLIENT_APP_IDENTITY_IMMUTABLE")
			}
			if c.AppSecret != nil {
				bindings[0].SealedSecret = sealed
			}
			if in.Status == "active" && len(bindings[0].SealedSecret) == 0 {
				return clientAppError(400, "CLIENT_APP_SECRET_REQUIRED")
			}
			fields := map[string]any{"name": in.Name, "status": in.Status, "plugin_ids": datatypes.JSON(grants), "version": a.Version + 1}
			if e = r.Update(ctx, a, fields, bindings); e != nil {
				return e
			}
			a, e = r.Get(ctx, tenant, id, false)
			if e != nil {
				return e
			}
		}
		changes, _ := json.Marshal(map[string]any{"fields": []string{"name", "status", "plugin_ids", "connections"}, "credential_changed": c.AppSecret != nil, "version": a.Version})
		actorID := int64(actor)
		if e = tx.Create(&modelaudit.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: tenant, ActorUserID: &actorID, Source: "customer.client_app", Operation: "customer.client_app.saved", ResourceType: "customer.client_app", ResourceID: a.UUID.String(), Outcome: "SUCCESS", Severity: "INFO", CorrelationID: reqctx.GetTraceID(ctx), ChangesAfter: datatypes.JSON(changes), ChangesRedacted: true}).Error; e != nil {
			return e
		}
		view = appView(a, bindings)
		return nil
	})
	if err != nil {
		if dto.CodeOf(err) != "" {
			return ClientApplicationView{}, err
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return ClientApplicationView{}, clientAppError(409, "CLIENT_APP_DUPLICATE")
		}
		return ClientApplicationView{}, clientAppStorageError(err)
	}
	return view, nil
}

// Lookup 只读取当前已验证租户的数据库登记。禁用、缺表或密钥失败都不回退部署配置。
func (s *ClientApplicationService) Lookup(ctx context.Context, tenant, plugin, platform, appID string) (ClientApp, bool, error) {
	a, b, err := s.repo.ResolveRuntime(ctx, tenant, platform, appID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ClientApp{}, false, nil
	}
	if err != nil {
		return ClientApp{}, false, ErrCustomerUpstreamDependency
	}
	var plugins []string
	if json.Unmarshal(a.PluginIDs, &plugins) != nil {
		return ClientApp{}, false, ErrCustomerUpstreamDependency
	}
	if a.Status != "active" || !slices.Contains(plugins, plugin) {
		return ClientApp{}, false, ErrCustomerClientAppForbidden
	}
	installed, err := s.repo.TenantPlugins(ctx, tenant)
	if err != nil {
		return ClientApp{}, false, ErrCustomerUpstreamDependency
	}
	if !slices.Contains(installed, plugin) {
		return ClientApp{}, false, ErrCustomerClientAppForbidden
	}
	if len(b.SealedSecret) == 0 {
		return ClientApp{}, false, ErrCustomerClientAppNotConfigured
	}
	var secret struct {
		AppSecret string `json:"app_secret"`
	}
	if err = s.keys.UnsealSensitive(ctx, "default", tenant, b.SealedSecret, &secret); err != nil {
		return ClientApp{}, false, ErrCustomerUpstreamDependency
	}
	if secret.AppSecret == "" {
		return ClientApp{}, false, ErrCustomerClientAppNotConfigured
	}
	return ClientApp{Platform: b.Platform, AppID: b.AppID, Name: a.Name, Secret: secret.AppSecret, PluginIDs: plugins, TenantUUIDs: []string{tenant}, CredentialVersion: a.Version, CredentialScope: tenant}, true, nil
}

func legacyTenantAllowed(ctx context.Context, a ClientApp, tenant string) bool {
	return len(a.TenantUUIDs) > 0 && containsTrimmed(a.TenantUUIDs, tenant) || len(a.TenantUUIDs) == 0 && reqctx.IsRoot(ctx)
}

// ImportLegacy 显式把旧配置复制为当前租户的独立记录；不会修改部署文件或其他租户。
func (s *ClientApplicationService) ImportLegacy(ctx context.Context, platform, appID string) (ClientApplicationView, error) {
	tenant, _, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_client_apps_import", "create")
	if err != nil {
		return ClientApplicationView{}, err
	}
	s.mu.RLock()
	var found *ClientApp
	for _, a := range s.legacy {
		if a.Platform == platform && a.AppID == appID && legacyTenantAllowed(ctx, a, tenant) {
			copy := a
			found = &copy
			break
		}
	}
	s.mu.RUnlock()
	if found == nil {
		return ClientApplicationView{}, clientAppError(404, "CLIENT_APP_NOT_FOUND")
	}
	plugins, err := s.repo.TenantPlugins(ctx, tenant)
	if err != nil {
		return ClientApplicationView{}, clientAppStorageError(err)
	}
	grants := []string{}
	for _, id := range found.PluginIDs {
		if slices.Contains(plugins, id) {
			grants = append(grants, id)
		}
	}
	secret := found.Secret
	var ptr *string
	if secret != "" {
		ptr = &secret
	}
	status := "disabled"
	if secret != "" && len(grants) > 0 {
		status = "active"
	}
	name := found.Name
	if name == "" {
		name = found.AppID
	}
	provider := strings.TrimSuffix(found.Platform, "_miniapp")
	return s.Save(ctx, "", ClientApplicationInput{Name: name, Kind: ClientAppMiniProgram, Status: status, PluginIDs: grants, Connections: []ClientAppConnectionInput{{Provider: provider, AppID: found.AppID, AppSecret: ptr}}})
}
