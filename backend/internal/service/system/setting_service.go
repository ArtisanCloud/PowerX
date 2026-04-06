package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gorm.io/datatypes"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type SettingService struct {
	db         *gorm.DB
	sysRepo    *repo.SystemSettingRepository
	tenantRepo *repo.TenantSettingRepository
}

func NewSettingService(db *gorm.DB) *SettingService {
	return &SettingService{
		db:         db,
		sysRepo:    repo.NewSystemSettingRepository(db),
		tenantRepo: repo.NewTenantSettingRepository(db),
	}
}

// =============== System ===============
func (s *SettingService) ListSystem(ctx context.Context, group, prefix string, page, size int) (items []*dbsetting.SystemSetting, total int64, err error) {
	db := s.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}

	q := db.Model(&dbsetting.SystemSetting{})
	if group != "" {
		q = q.Where("group = ?", group)
	}
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return
}

func (s *SettingService) GetSystem(ctx context.Context, key string) (*dbsetting.SystemSetting, error) {
	return s.sysRepo.GetByKey(ctx, key)
}

func (s *SettingService) UpsertSystem(ctx context.Context, key string, value datatypes.JSON, group, desc *string, editable *bool) error {
	m := &dbsetting.SystemSetting{
		Key:       key,
		ValueJSON: value,
	}
	if group != nil {
		m.Group = *group
	}
	if desc != nil {
		m.Description = desc
	}
	if editable != nil {
		m.Editable = *editable
	}
	return s.sysRepo.Upsert(ctx, m)
}

func (s *SettingService) DeleteSystem(ctx context.Context, key string, soft bool) error {
	return s.sysRepo.DeleteByKey(ctx, key, soft)
}

// =============== Tenant ===============
func (s *SettingService) ListTenant(ctx context.Context, tenantUUID, prefix string, page, size int) (items []*dbsetting.TenantSetting, total int64, err error) {
	db := s.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}

	q := db.Model(&dbsetting.TenantSetting{}).Where("tenant_uuid = ?", tenantUUID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return
}

func (s *SettingService) GetTenant(ctx context.Context, tenantUUID, key string) (*dbsetting.TenantSetting, error) {
	return s.tenantRepo.GetByTenantAndKey(ctx, tenantUUID, key)
}

func (s *SettingService) UpsertTenant(ctx context.Context, tenantUUID, key string, value datatypes.JSON, group, desc *string, editable *bool) error {
	m := &dbsetting.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        key,
		ValueJSON:  value,
	}
	if group != nil {
		m.Group = *group
	}
	if desc != nil {
		m.Description = desc
	}
	if editable != nil {
		m.Editable = *editable
	}
	return s.tenantRepo.Upsert(ctx, m)
}

func (s *SettingService) DeleteTenant(ctx context.Context, tenantUUID, key string, soft bool) error {
	return s.tenantRepo.Delete(ctx, tenantUUID, key, soft)
}

// =============== Effective (DB 层) ===============
func (s *SettingService) GetEffectiveFromDB(ctx context.Context, tenantUUID *string, key string) (json.RawMessage, string, error) {
	return s.tenantRepo.GetEffective(ctx, tenantUUID, key, s.sysRepo)
}

func (s *SettingService) GetSystemJSON(ctx context.Context, key string, out any) (bool, error) {
	item, err := s.GetSystem(ctx, key)
	if err != nil {
		return false, err
	}
	if item == nil || len(item.ValueJSON) == 0 {
		return false, nil
	}
	if out == nil {
		return false, errors.New("out cannot be nil")
	}
	if err := json.Unmarshal(item.ValueJSON, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SettingService) UpsertSystemJSON(ctx context.Context, key string, value any, group, desc string, editable bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("key cannot be empty")
	}
	bs, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.UpsertSystem(ctx, key, datatypes.JSON(bs), &group, &desc, &editable)
}

func (s *SettingService) ApplySetupPortConfig(ctx context.Context, runtimeConfigPath string, backendPort, webAdminPort int) error {
	_ = ctx
	if backendPort <= 0 || backendPort > 65535 {
		return fmt.Errorf("invalid backend port: %d", backendPort)
	}
	if webAdminPort <= 0 || webAdminPort > 65535 {
		return fmt.Errorf("invalid web admin port: %d", webAdminPort)
	}
	if strings.TrimSpace(runtimeConfigPath) != "" {
		if err := updateRuntimePortConfig(runtimeConfigPath, backendPort, webAdminPort); err != nil {
			return err
		}
	}
	configDir := resolveDistConfigDir(runtimeConfigPath)
	if configDir == "" {
		return nil
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	if err := upsertEnvFile(filepath.Join(configDir, "powerx.env"), map[string]string{
		"POWERX_BACKEND_PORT":   strconv.Itoa(backendPort),
		"POWERX_WEB_ADMIN_PORT": strconv.Itoa(webAdminPort),
	}); err != nil {
		return err
	}
	upstream := fmt.Sprintf("http://127.0.0.1:%d", backendPort)
	wsUpstream := fmt.Sprintf("ws://127.0.0.1:%d/api/ws", backendPort)
	if err := upsertEnvFile(filepath.Join(configDir, "web-admin.env"), map[string]string{
		"POWERX_BACKEND":    upstream,
		"WS_UPSTREAM": wsUpstream,
	}); err != nil {
		return err
	}
	return nil
}

func resolveDistConfigDir(runtimeConfigPath string) string {
	if p := strings.TrimSpace(os.Getenv("POWERX_SETUP_DIST_CONFIG_DIR")); p != "" {
		return p
	}
	if strings.TrimSpace(runtimeConfigPath) == "" {
		return ""
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(runtimeConfigPath), "..", ".."))
	return filepath.Join(root, "config")
}

func updateRuntimePortConfig(path string, backendPort, webAdminPort int) error {
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
	server := toMap(root["server"])
	server["port"] = backendPort
	root["server"] = server
	root["web_admin_port"] = webAdminPort
	data, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func upsertEnvFile(path string, kv map[string]string) error {
	lines := make([]string, 0)
	if raw, err := os.ReadFile(path); err == nil {
		text := strings.ReplaceAll(string(raw), "\r\n", "\n")
		text = strings.TrimRight(text, "\n")
		if text != "" {
			lines = strings.Split(text, "\n")
		}
	}
	used := make(map[string]struct{}, len(kv))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if val, exists := kv[key]; exists {
			lines[i] = key + "=" + val
			used[key] = struct{}{}
		}
	}
	for key, val := range kv {
		if _, ok := used[key]; ok {
			continue
		}
		lines = append(lines, key+"="+val)
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func toMap(v any) map[string]any {
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
