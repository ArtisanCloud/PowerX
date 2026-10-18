package system_recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

const CurrentCacheProtocol = "powerx.current-cache/v1"

// CurrentCacheNamespace 隔离当前安装的可重建缓存，不要求独占整个 Redis 库。
// 恢复只切换缓存代次；不删除共享库里的历史缓存或其他程序的数据。
func CurrentCacheNamespace(cfg *config.Config, configFile string) (string, error) {
	if cfg.Cache.Driver != "redis" || cfg.Deployment.Env != "dev" || cfg.Recovery.ProfileFile != "" || configFile == "" {
		return "", nil
	}
	file, err := filepath.Abs(configFile)
	if err != nil {
		return "", err
	}
	directory, _ := snapshots.DefaultStorage()
	if cfg.SystemSnapshots.Directory != "" {
		directory = cfg.SystemSnapshots.Directory
	}
	if resolved, e := filepath.EvalSymlinks(directory); e == nil {
		directory = resolved
	} else if !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	epoch := "initial"
	var state struct {
		Epoch string `json:"epoch"`
	}
	err = snapshots.ReadCoordinatorJSON(filepath.Join(currentDirectory(directory, file), "cache-epoch.json"), &state)
	if err == nil {
		if _, e := uuid.Parse(state.Epoch); e != nil {
			return "", errors.New("当前安装的缓存代次无效")
		}
		epoch = state.Epoch
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	identity := fmt.Sprintf("%s\x00%s:%d/%s\x00%s", file, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database, cfg.Database.DSN)
	sum := sha256.Sum256([]byte(identity))
	return CurrentCacheProtocol + ":" + hex.EncodeToString(sum[:12]) + ":" + epoch + ":", nil
}

func (r *CurrentRuntime) rotateCurrentCache() error {
	return snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "cache-epoch.json"), map[string]string{"epoch": uuid.NewString()})
}
