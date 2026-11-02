package storagegov

import (
	"context"
	"errors"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Service 缓存只读观测，避免页面刷新反复遍历文件系统；不启动后台清理。
type Service struct {
	dbRetention *caprepo.RetentionRepository
	opts        Options
	mu          sync.Mutex
	policyMu    sync.Mutex // 保存保护规则与本实例的有界手动清理互斥。
	cached      *Overview
	store       stateStore
	runs        cleanupStore
	eventMu     sync.Mutex
	listeners   map[chan RuntimeEvent]struct{}
	runtimeOnce sync.Once
}

// NewService 严格验证登记来源，错误配置不静默跳过或推断环境。
func NewService(opts Options) (*Service, error) {
	if opts.Collector == nil || !filepath.IsAbs(opts.FilesystemPath) || strings.ContainsRune(opts.FilesystemPath, 0) || len(opts.Resources) > 64 {
		return nil, errors.New("存储观测需要采集器和绝对文件系统路径")
	}
	for _, id := range []string{opts.Identity.HostID, opts.Identity.InstanceID, opts.Identity.FilesystemID} {
		if id != "" && !idPattern.MatchString(id) {
			return nil, errors.New("存储观测身份格式无效")
		}
	}
	validScope := func(scope string) bool {
		return scope == "dev" || scope == "test" || scope == "staging" || scope == "prod"
	}
	if opts.Identity.Environment != "" && !validScope(opts.Identity.Environment) {
		return nil, errors.New("存储观测部署环境无效")
	}
	seen := map[string]bool{}
	for _, r := range opts.Resources {
		if r.TenantUUID != "" {
			if _, err := uuid.Parse(r.TenantUUID); err != nil || r.Scope == "shared" {
				return nil, errors.New("租户来源身份无效")
			}
		}
		if !idPattern.MatchString(r.ID) || seen[r.ID] || strings.TrimSpace(r.Name) == "" || len(r.Name) > 200 || !filepath.IsAbs(r.Path) || strings.ContainsRune(r.Path, 0) {
			return nil, errors.New("存储观测来源标识、名称或路径无效")
		}
		if r.Scope != "shared" && (!validScope(r.Scope) || !idPattern.MatchString(r.InstanceID)) {
			return nil, errors.New("实例来源必须具有明确环境和安装身份")
		}
		if r.Scope == "shared" && r.InstanceID != "" {
			return nil, errors.New("共享来源不能重复归属于某个实例")
		}
		switch r.Kind {
		case "backup_files", "snapshot_files", "archive_objects", "media_files", "application_logs", "audit_logs", "system_logs", "temporary_files", "postgresql_logs", "journal", "loki":
		default:
			return nil, errors.New("不支持的存储观测来源类型")
		}
		seen[r.ID] = true
	}
	opts.Resources = append([]Resource(nil), opts.Resources...)
	sort.Slice(opts.Resources, func(i, j int) bool { return opts.Resources[i].ID < opts.Resources[j].ID })
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 5 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Service{opts: opts, listeners: map[chan RuntimeEvent]struct{}{}}
	if opts.DB != nil {
		s.dbRetention = caprepo.NewRetentionRepository(opts.DB)
		s.store = repo.NewStateRepository(opts.DB)
		s.runs = repo.NewCleanupRepository(opts.DB)
	}
	return s, nil
}

// Overview 返回进程可见文件系统的实测快照，不宣称已取得 XDocker 宿主机视图。
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Overview{}, err
	}
	now := s.opts.Now().UTC()
	if s.cached != nil && now.Before(s.cached.ExpiresAt) {
		return *s.cached, nil
	}
	snap := s.opts.Collector.Collect(ctx, s.opts.FilesystemPath, s.opts.Resources)
	if err := ctx.Err(); err != nil {
		return Overview{}, err
	}
	out := Overview{
		Mode: "observe", Identity: s.opts.Identity, ObservedAt: now, ExpiresAt: now.Add(s.opts.CacheTTL),
		Filesystem: snap.Filesystem, Resources: snap.Resources, Coverage: "partial",
		Limitations: []string{"registered_files_only", "process_filesystem_view", "database_metrics_not_connected", "gross_ingress_not_instrumented", "archive_coverage_not_verified", "cleanup_not_enabled"},
		Sampling:    s.samplingStatus(),
		Inventory:   append([]CoverageItem{}, s.opts.Inventory...),
	}
	if out.Resources == nil {
		out.Resources = []Usage{}
	}
	if s.opts.DatabaseCollector != nil {
		database := s.opts.DatabaseCollector.CollectDatabase(ctx)
		out.Database = &database
		out.Limitations[2] = "database_metrics_not_added_to_filesystem_total"
	}
	if s.opts.Budget != nil || s.opts.LogDelivery != nil {
		runtime := s.runtimeSnapshot(ctx)
		out.Runtime = &runtime
		if runtime.Problem == "" && runtime.Lifecycle != nil && runtime.Lifecycle.AllowEnforce && runtime.WorkerEnabled {
			for _, p := range runtime.Lifecycle.Policies {
				if p.Enabled && p.OwnerInstance == s.opts.Identity.InstanceID {
					out.Mode = "enforce"
					out.CleanupEnabled = true
					break
				}
			}
		}
		if out.CleanupEnabled {
			out.Limitations = out.Limitations[:len(out.Limitations)-1]
		}
	}
	out.Alerts = EvaluateCapacity(out)
	if out.Filesystem.UsedBytes != nil {
		var known int64
		for _, r := range out.Resources {
			if r.FilesystemID == out.Filesystem.ID && r.AttributedBytes != nil {
				known += *r.AttributedBytes
			}
		}
		if known <= *out.Filesystem.UsedBytes {
			remaining := *out.Filesystem.UsedBytes - known
			out.UnattributedBytes = &remaining
		}
	}
	s.cached = &out
	return out, nil
}
