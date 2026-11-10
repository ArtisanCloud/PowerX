package manager

import (
	"context"
	"errors"
	"os"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// commitHostConfigUpdate 文件与 registry 分步提交；任一步报错恢复原文件、内存和登记。
// 进程/机器中断不是跨文件原子事务；离线修复另存完整备份以支持恢复。
func (m *managerImpl) commitHostConfigUpdate(ctx context.Context, p *plugin_mgr.Plugin, next *plugin_mgr.HostConfig, data []byte, saveRegistry bool) (resultErr error) {
	before := cloneHostConfig(p.HostConfig)
	path := p.Paths.HostValuesFile
	var old []byte
	existed := false
	if path != "" {
		var err error
		old, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		existed = err == nil
	}
	if saveRegistry && m.opts.Registry == nil {
		return errors.New("HOST_CONFIG_REGISTRY_REQUIRED")
	}
	wroteFile, changedRegistry := false, false
	defer func() {
		if resultErr == nil {
			return
		}
		p.HostConfig = before
		if wroteFile {
			var err error
			if existed {
				err = runtimecredential.AtomicWrite(path, old, 0600)
			} else {
				err = os.Remove(path)
				if os.IsNotExist(err) {
					err = nil
				}
			}
			resultErr = errors.Join(resultErr, err)
		}
		if changedRegistry {
			rollbackCtx := context.WithoutCancel(ctx)
			resultErr = errors.Join(resultErr, m.opts.Registry.UpdateHostConfig(rollbackCtx, p.ID, p.Version, before))
			resultErr = errors.Join(resultErr, m.opts.Registry.Save(rollbackCtx))
		}
	}()
	if path != "" {
		if err := runtimecredential.AtomicWrite(path, data, 0600); err != nil {
			return err
		}
		wroteFile = true
	}
	if saveRegistry {
		changedRegistry = true
		if err := m.opts.Registry.UpdateHostConfig(ctx, p.ID, p.Version, next); err != nil {
			return err
		}
		if err := m.opts.Registry.Save(ctx); err != nil {
			return err
		}
	}
	p.HostConfig = cloneHostConfig(next)
	return nil
}
