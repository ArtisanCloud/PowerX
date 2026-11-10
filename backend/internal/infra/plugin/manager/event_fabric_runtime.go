package manager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
)

type eventFabricCheckpoint struct {
	PluginID   string                 `json:"plugin_id"`
	Version    string                 `json:"version"`
	TenantUUID string                 `json:"tenant_uuid"`
	Values     []byte                 `json:"values"`
	HostConfig *plugin_mgr.HostConfig `json:"host_config"`
}

func (m *managerImpl) eventFabricCheckpointPath(p *plugin_mgr.Plugin) (string, error) {
	credentialPath, err := runtimecredential.Path(m.opts.InstalledRoot, p.ID)
	if err != nil {
		return "", err
	}
	version := sha256.Sum256([]byte(p.Version))
	// 与运行凭据同属宿主管理目录，位于版本包之外，安装包不能伪造恢复检查点。
	return filepath.Join(filepath.Dir(credentialPath), fmt.Sprintf(".event-fabric-%x.pending.json", version[:8])), nil
}

// prepareEventFabricForEnable 普通恢复只核验；受控安装/管理员启用才准备资源并提交映射。
func (m *managerImpl) prepareEventFabricForEnable(ctx context.Context, p *plugin_mgr.Plugin, cred *PluginRuntimeCredential) (resultErr error) {
	if !eventFabricPreparationDeclared(*p) {
		return nil
	}
	if cred == nil || m.opts.PrepareEventFabric == nil || p.HostConfig == nil || m.opts.Registry == nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_PREPARER_REQUIRED")
	}
	path := p.Paths.HostValuesFile
	if path == "" {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_VALUES_REQUIRED")
	}
	provision := plugin_mgr.EventFabricProvisioning(ctx)
	checkpointPath, err := m.eventFabricCheckpointPath(p)
	if err != nil {
		return err
	}
	restore := func(checkpoint eventFabricCheckpoint) error {
		if err := runtimecredential.AtomicWrite(path, checkpoint.Values, 0600); err != nil {
			return err
		}
		if err := m.opts.Registry.UpdateHostConfig(context.WithoutCancel(ctx), p.ID, p.Version, checkpoint.HostConfig); err != nil {
			return err
		}
		if err := m.opts.Registry.Save(context.WithoutCancel(ctx)); err != nil {
			return err
		}
		p.HostConfig = cloneHostConfig(checkpoint.HostConfig)
		return os.Remove(checkpointPath)
	}
	if pending, err := os.ReadFile(checkpointPath); err == nil {
		if !provision {
			return fmt.Errorf("EVENT_FABRIC_INSTALL_RECOVERY_REQUIRED")
		}
		var checkpoint eventFabricCheckpoint
		if json.Unmarshal(pending, &checkpoint) != nil || checkpoint.HostConfig == nil || len(checkpoint.Values) == 0 || checkpoint.PluginID != p.ID || checkpoint.Version != p.Version || checkpoint.TenantUUID != cred.TenantUUID {
			return fmt.Errorf("EVENT_FABRIC_INSTALL_CHECKPOINT_INVALID")
		}
		if err := restore(checkpoint); err != nil {
			return fmt.Errorf("EVENT_FABRIC_INSTALL_RECOVERY_FAILED: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil || doc == nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_VALUES_INVALID")
	}
	previous := cloneHostConfig(p.HostConfig)
	working := cloneHostConfig(p.HostConfig)
	working.Spec = stripHostConfigMeta(cloneAnyMap(doc))
	p.HostConfig = working
	manifestPath, err := pluginbinding.ManifestPath(*p)
	if err != nil {
		p.HostConfig = previous
		return err
	}
	if !provision {
		return m.opts.PrepareEventFabric(ctx, *p, cred.TenantUUID, manifestPath, false, nil)
	}
	checkpoint := eventFabricCheckpoint{PluginID: p.ID, Version: p.Version, TenantUUID: cred.TenantUUID, Values: raw, HostConfig: previous}
	touched := false
	defer func() {
		if resultErr != nil {
			if touched {
				resultErr = errors.Join(resultErr, restore(checkpoint))
			} else {
				p.HostConfig = previous
			}
		}
	}()
	committed := false
	err = m.opts.PrepareEventFabric(ctx, *p, cred.TenantUUID, manifestPath, true, func(bindings map[string]string) error {
		if committed || len(bindings) == 0 {
			return fmt.Errorf("EVENT_FABRIC_INSTALL_RECEIPTS_REQUIRED")
		}
		seen := map[string]bool{}
		for key, full := range bindings {
			if key == "" || key != strings.TrimSpace(key) || len(key) > 255 || len(full) > 255 || full != strings.TrimSpace(full) || !strings.HasPrefix(full, cred.TenantUUID+".") || seen[full] || strings.ContainsAny(key+full, "\r\n\x00") {
				return fmt.Errorf("EVENT_FABRIC_INSTALL_RECEIPT_INVALID")
			}
			seen[full] = true
		}
		pluginbinding.SetBindings(doc, cred.TenantUUID, bindings)
		if rootEventFabricBridge(doc) != nil {
			setNestedValue(doc, []string{"event_bridge", "topic_bindings"}, pluginbinding.Bridge(doc)["topic_bindings"])
		}
		encoded, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		saved, err := json.Marshal(checkpoint)
		if err != nil {
			return err
		}
		if err := runtimecredential.AtomicWrite(checkpointPath, saved, 0600); err != nil {
			return err
		}
		touched = true
		if err := runtimecredential.AtomicWrite(path, encoded, 0600); err != nil {
			return err
		}
		p.HostConfig.Spec = stripHostConfigMeta(cloneAnyMap(doc))
		if err := m.opts.Registry.UpdateHostConfig(ctx, p.ID, p.Version, p.HostConfig); err != nil {
			return err
		}
		if err := m.opts.Registry.Save(ctx); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return err
	}
	if !committed {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_COMMIT_REQUIRED")
	}
	// PrepareEventFabric 返回成功才代表数据库已经提交。删除日志失败保留可重入检查点，不能假称跨文件原子提交。
	touched = false
	if err := os.Remove(checkpointPath); err != nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_CHECKPOINT_CLEANUP_FAILED: %w", err)
	}
	return nil
}

func eventFabricBridge(doc map[string]any) map[string]any { return pluginbinding.Bridge(doc) }
func rootEventFabricBridge(doc map[string]any) map[string]any {
	bridge, _ := doc["event_bridge"].(map[string]any)
	return bridge
}
func eventFabricPreparationDeclared(p plugin_mgr.Plugin) bool { return pluginbinding.Declared(p) }
