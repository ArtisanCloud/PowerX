package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RecoveryProtocol 是协调器、Core 和恢复候选共同遵守的启动协议。
const RecoveryProtocol = 1

// RecoveryConfig 只能由受信任的本机协调器写入私有配置，不由 HTTP 请求提供路径或命令。
type RecoveryConfig struct {
	HTTPSocket    string `yaml:"http_socket,omitempty" json:"http_socket,omitempty"`
	ProfileFile   string `yaml:"profile_file,omitempty" json:"profile_file,omitempty"`
	Mode          string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Root          string `yaml:"root,omitempty" json:"root,omitempty"`
	JobID         string `yaml:"job_id,omitempty" json:"job_id,omitempty"`
	SealSHA256    string `yaml:"seal_sha256,omitempty" json:"seal_sha256,omitempty"`
	PluginPortMin int    `yaml:"plugin_port_min,omitempty" json:"plugin_port_min,omitempty"`
	PluginPortMax int    `yaml:"plugin_port_max,omitempty" json:"plugin_port_max,omitempty"`
}

// RecoverySeal 绑定候选目录、任务和启动令牌；不进入公开状态或日志。
type RecoverySeal struct {
	Protocol int    `json:"protocol"`
	JobID    string `json:"job_id"`
	Root     string `json:"root"`
	Token    string `json:"token"`
}

func (c RecoveryConfig) ReadOnly() bool { return c.Mode == "verify" || c.Mode == "hold" }

func BackgroundWorkersDisabled(c *Config) bool {
	if os.Getenv("POWERX_RECOVERY_VERIFY_SERVER") == "1" {
		return true
	}
	return c != nil && (c.Recovery.ReadOnly() || (c.Recovery.Mode != "" && c.Deployment.Env == "dev" && os.Getenv("POWERX_LOCAL_REHEARSAL") == "1")) || os.Getenv("POWERX_COORDINATOR_MAINTENANCE") == "1"
}

// ValidateRecoveryStartup 在数据库、缓存、插件或 worker 初始化之前校验所有目标地址。
func (c *Config) ValidateRecoveryStartup() error {
	if c.Recovery.Mode == "" {
		return nil
	}
	r := c.Recovery
	if (!r.ReadOnly() && r.Mode != "active") || !filepath.IsAbs(r.Root) || uuid.Validate(r.JobID) != nil || r.JobID == uuid.Nil.String() {
		return errors.New("RECOVERY_STARTUP_INVALID: 恢复模式、任务或目录无效")
	}
	root, e := filepath.EvalSymlinks(r.Root)
	if e != nil || root != r.Root {
		return errors.New("RECOVERY_STARTUP_INVALID: 候选目录必须是规范路径")
	}
	file := filepath.Join(root, "recovery-seal.json")
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return errors.New("RECOVERY_STARTUP_INVALID: 缺少私有启动凭据")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		return errors.New("RECOVERY_STARTUP_INVALID: 启动凭据不可读")
	}
	sum := sha256.Sum256(raw)
	var seal RecoverySeal
	if json.Unmarshal(raw, &seal) != nil || seal.Protocol != RecoveryProtocol || seal.Root != root || seal.JobID != r.JobID || hex.EncodeToString(sum[:]) != r.SealSHA256 || len(seal.Token) < 32 || os.Getenv("POWERX_RECOVERY_TOKEN") != seal.Token {
		return errors.New("RECOVERY_STARTUP_INVALID: 启动凭据不匹配")
	}
	paths := []string{c.Storage.Local.BasePath, c.Plugin.InstalledDir, c.Plugin.RegistryFile, c.Recovery.HTTPSocket}
	if c.SystemSnapshots.Directory != "" {
		paths = append(paths, c.SystemSnapshots.Directory)
	}
	for _, p := range paths {
		rel, e := filepath.Rel(root, p)
		if e != nil || !filepath.IsAbs(p) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("RECOVERY_TARGET_INVALID: 配置仍引用候选目录以外的存储")
		}
	}
	if c.Database.DSN == "" || c.Database.Driver != "postgres" || c.Storage.DefaultDriver != "local" || c.Cache.Host != "127.0.0.1" || c.Server.Host != "127.0.0.1" || c.Cache.Port < 1024 || c.Server.Port < 1024 || r.PluginPortMin < 1024 || r.PluginPortMax < r.PluginPortMin || r.PluginPortMax > 65535 {
		return errors.New("RECOVERY_TARGET_INVALID: 目标数据库、缓存、媒体或监听地址无效")
	}
	db, e := pgx.ParseConfig(c.Database.DSN)
	if e != nil || !filepath.IsAbs(db.Host) || !strings.HasPrefix(db.Host, root+string(filepath.Separator)) || db.Database != "powerx" || db.User != "powerx_recovery" || len(db.Fallbacks) != 0 || len(c.Database.Replicas) != 0 {
		return errors.New("RECOVERY_TARGET_INVALID: 数据库必须使用候选的私有 Unix socket")
	}
	targetRedis := fmt.Sprintf("127.0.0.1:%d", c.Cache.Port)
	if c.Queue.Driver != "redis" || c.Queue.Redis.Addr != targetRedis || (c.Event.Bus.Type != "redis" && c.Event.Bus.Type != "local") || c.Event.Bus.RedisAddr != targetRedis || c.Event.Fabric.RedisAddr != targetRedis || c.Event.Fabric.Authorization.RedisAddr != targetRedis || c.KnowledgeSpace.RedisAddr != targetRedis || c.Server.GRPC.Host != "127.0.0.1" {
		return errors.New("RECOVERY_TARGET_INVALID: 队列、事件或 gRPC 仍引用其他环境")
	}
	return nil
}
