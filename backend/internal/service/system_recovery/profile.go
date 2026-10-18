package system_recovery

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

type Tools struct {
	PostgresBin string `json:"postgres_bin"`
	RedisServer string `json:"redis_server"`
	Node        string `json:"node"`
	Coordinator string `json:"coordinator"`
	Sandbox     string `json:"sandbox"`
}
type ArtifactOverride struct {
	Path    string `json:"path"`
	Source  string `json:"source"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

// BusinessProbe 是运维预先登记的只读业务检查，HTTP 调用方不能传路径或方法。
type BusinessProbe struct {
	Name     string `json:"name"`
	PluginID string `json:"plugin_id,omitempty"`
	Path     string `json:"path"`
}

// Profile 是仅本机运维可改的部署合同；HTTP 不接受命令、路径、连接串或覆盖文件。
type Profile struct {
	InPlace              bool                       `json:"restore_in_place,omitempty"`
	Local                bool                       `json:"local,omitempty"`
	GatewayWebListen     string                     `json:"gateway_web_listen,omitempty"`
	GatewayBackendListen string                     `json:"gateway_backend_listen,omitempty"`
	File                 string                     `json:"-"`
	StatusListen         string                     `json:"status_listen"`
	InstanceID           string                     `json:"instance_id"`
	Protocol             int                        `json:"protocol"`
	Directory            string                     `json:"directory"`
	SnapshotDirectory    string                     `json:"snapshot_directory"`
	CandidateDirectory   string                     `json:"candidate_directory"`
	Tools                Tools                      `json:"tools"`
	Targets              []snapshots.RecoveryTarget `json:"targets"`
	Initial              Deployment                 `json:"initial"`
	WriterUnits          []string                   `json:"writer_units"`
	ExclusiveWriters     bool                       `json:"exclusive_writers"`
	DedicatedRedis       bool                       `json:"dedicated_redis"`
	SourceRedisRDB       string                     `json:"source_redis_rdb"`
	Overrides            []ArtifactOverride         `json:"artifact_overrides"`
	BusinessProbes       []BusinessProbe            `json:"business_probes,omitempty"`
}

func ReadProfile(file string) (Profile, error) {
	var p Profile
	info, e := os.Lstat(file)
	if e != nil || !filepath.IsAbs(file) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return p, errors.New("recovery profile must be an absolute private regular file")
	}
	if e = snapshots.ReadCoordinatorJSON(file, &p); e != nil {
		return p, e
	}
	p.File = file
	return p, p.Validate()
}
func (p Profile) Validate() error {
	seenAddresses := map[string]bool{}
	for _, address := range []string{p.GatewayWebListen, p.GatewayBackendListen} {
		if address == "" {
			continue
		}
		host, port, e := net.SplitHostPort(address)
		number, parse := strconv.Atoi(port)
		if e != nil || parse != nil || host != "127.0.0.1" || number < 1024 || number > 65535 || seenAddresses[address] || address == p.StatusListen {
			return errors.New("恢复入口必须是不同的本地非特权端口")
		}
		seenAddresses[address] = true
	}
	if uuid.Validate(p.InstanceID) != nil {
		return errors.New("a stable instance UUID is required")
	}
	if p.Protocol != config.RecoveryProtocol || !p.ExclusiveWriters || !p.DedicatedRedis {
		return errors.New("recovery requires protocol 1, an exclusive managed writer set, and dedicated Redis")
	}
	for _, dir := range []string{p.Directory, p.SnapshotDirectory, p.CandidateDirectory, p.Initial.Root, p.Initial.ConfigFile, p.Initial.CoreBinary, p.Initial.WebOutput, p.SourceRedisRDB, p.Tools.PostgresBin, p.Tools.RedisServer, p.Tools.Node, p.Tools.Coordinator, p.Tools.Sandbox} {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
			return errors.New("profile paths must be canonical absolute paths")
		}
	}
	for _, dir := range []string{p.Directory, p.SnapshotDirectory, p.CandidateDirectory, p.Initial.Root} {
		if !canonicalNewPath(dir) {
			return errors.New("source and recovery directories must use real canonical paths, without symlink aliases")
		}
	}
	for _, unit := range p.WriterUnits {
		if !regexp.MustCompile(`^[a-zA-Z0-9_.@-]+\.service$`).MatchString(unit) {
			return errors.New("invalid writer unit")
		}
	}
	ids := map[string]bool{}
	for _, t := range p.Targets {
		if ids[t.ID] || config.ValidateDeploymentEnv(t.Environment) != nil {
			return errors.New("duplicate or invalid target environment")
		}
		ids[t.ID] = true
		if (p.Local && t.Environment != "dev") || (t.AllowOnlineData && (!p.Local || t.Environment != "dev")) {
			return errors.New("在线包完整站点演练和原地覆盖仅限本地 dev 合同")
		}
	}
	if len(p.Targets) == 0 {
		return errors.New("no recovery targets configured")
	}
	if p.InPlace {
		if p.Initial.SourceManaged || uuid.Validate(p.Initial.ID) != nil || p.Initial.Root != filepath.Join(p.CandidateDirectory, p.Initial.ID) {
			return errors.New("原地恢复需要登记由执行器独占管理的固定实例；不会覆盖未接入的既有服务")
		}
		if len(p.Targets) != 1 || !p.Targets[0].AllowOverwrite {
			return errors.New("原地恢复必须且只能登记一个当前实例目标")
		}
		if p.Initial.ConfigFile != filepath.Join(p.Initial.Root, "etc/config.yaml") || p.Initial.PostgresData != filepath.Join(p.Initial.Root, "postgres") || p.Initial.SocketDirectory != filepath.Join(p.Initial.Root, "s") || p.Initial.CoreBinary != filepath.Join(p.Initial.Root, "programs/core/powerx") || p.Initial.WebOutput != filepath.Join(p.Initial.Root, "programs/core/web-admin") {
			return errors.New("固定实例的正式配置、PG 数据及程序必须位于登记根目录内的标准路径")
		}
	}
	ownedLocalInitial := (p.Local || p.InPlace) && uuid.Validate(p.Initial.ID) == nil && p.Initial.Root == filepath.Join(p.CandidateDirectory, p.Initial.ID) && !p.Initial.SourceManaged
	if inside(p.Initial.Root, p.CandidateDirectory) || (inside(p.CandidateDirectory, p.Initial.Root) && !ownedLocalInitial) || inside(p.CandidateDirectory, p.Directory) || inside(p.CandidateDirectory, p.SnapshotDirectory) {
		return errors.New("candidate, source and operational directories must not contain one another")
	}
	for _, override := range p.Overrides {
		if override.Path == "" || filepath.IsAbs(override.Path) || strings.Contains(override.Path, "\\") || filepath.Clean(override.Path) != override.Path || strings.HasPrefix(override.Path, "..") || !filepath.IsAbs(override.Source) || len(override.SHA256) != 64 || override.Version == "" {
			return errors.New("invalid native artifact override")
		}
	}
	for _, probe := range p.BusinessProbes {
		if probe.Name == "" || !strings.HasPrefix(probe.Path, "/") || strings.ContainsAny(probe.Path, "?#\\") || strings.Contains(probe.Path, "..") || strings.Contains(probe.Path, "://") {
			return errors.New("invalid read-only business probe")
		}
	}
	return nil
}

// 目录尚未创建时也验证现有父目录，防止软链接把候选指向源数据。
func canonicalNewPath(path string) bool {
	current := path
	parts := []string{}
	for {
		if _, e := os.Lstat(current); e == nil {
			resolved, e := filepath.EvalSymlinks(current)
			if e != nil {
				return false
			}
			for i := len(parts) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, parts[i])
			}
			return resolved == path
		} else if !errors.Is(e, os.ErrNotExist) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		parts = append(parts, filepath.Base(current))
		current = parent
	}
}
func inside(root, file string) bool {
	rel, e := filepath.Rel(root, file)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
