package system_recovery

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
	"github.com/google/uuid"
)

const CurrentCacheProtocol = "powerx.current-cache/v1"

// CurrentCacheNamespace 隔离当前安装的可重建缓存，不要求独占整个 Redis 库。
// 恢复只切换缓存代次；不删除共享库里的历史缓存或其他程序的数据。
func CurrentCacheNamespace(cfg *config.Config, configFile string) (string, error) {
	if cfg.Cache.Driver != "redis" {
		return "", nil
	}
	namespace, err := cfg.RedisNamespace()
	if err != nil {
		return "", err
	}
	prefix := redisnamespace.Key(namespace, "cache:")
	if cfg.Deployment.Env != "dev" || cfg.Recovery.ProfileFile != "" {
		return prefix, nil
	}
	directory := filepath.Dir(cfg.EnvironmentIdentityFile())
	if cfg.EnvironmentIdentityFile() == "" {
		if configFile == "" {
			return "", errors.New("当前环境身份记录路径未初始化")
		}
		file, e := filepath.Abs(configFile)
		if e != nil {
			return "", e
		}
		root, _ := snapshots.DefaultStorage()
		if cfg.SystemSnapshots.Directory != "" {
			root = cfg.SystemSnapshots.Directory
		}
		if resolved, e := filepath.EvalSymlinks(root); e == nil {
			root = resolved
		} else if !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
		directory = currentDirectory(root, file)
	}
	epoch := "initial"
	var state struct {
		Epoch string `json:"epoch"`
	}
	err = snapshots.ReadCoordinatorJSON(filepath.Join(directory, "cache-epoch.json"), &state)
	if err == nil {
		if _, e := uuid.Parse(state.Epoch); e != nil {
			return "", errors.New("当前安装的缓存代次无效")
		}
		epoch = state.Epoch
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return prefix + CurrentCacheProtocol + ":" + epoch + ":", nil
}

func (r *CurrentRuntime) rotateCurrentCache() error {
	return snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "cache-epoch.json"), map[string]string{"epoch": uuid.NewString()})
}
