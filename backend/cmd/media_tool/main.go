package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	mediarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/media"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbmaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
)

const (
	commandCleanup = "cleanup"
)

type cleanupFlags struct {
	configPath string
	dryRun     bool
	before     time.Duration
	limit      int
	tenantUUID string
	drivers    []string
}

type cleanupStats struct {
	processed int
	succeeded int
	failed    int
}

func main() {
	if len(os.Args) < 2 {
		printRootUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "-h", "--help", "help":
		if len(os.Args) >= 3 {
			printCommandUsage(os.Args[2])
			return
		}
		printRootUsage()
		return
	case commandCleanup:
		if err := runCleanupCommand(os.Args[2:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			logger.ErrorF(context.Background(), "执行清理任务失败: %v", err)
			os.Exit(1)
		}
	default:
		logger.ErrorF(context.Background(), "未知子命令: %s", os.Args[1])
		printRootUsage()
		os.Exit(2)
	}
}

func runCleanupCommand(args []string) error {
	flags, err := parseCleanupFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("解析参数失败: %w", err)
	}

	cfg := loadConfig(flags.configPath)
	ctx := context.Background()

	deps, err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("初始化核心依赖失败: %w", err)
	}
	if deps.AuditSvc != nil {
		defer deps.AuditSvc.Close()
	}
	defer closeSQL(deps.DB)

	repo := mediarepo.NewAssetRepository(deps.DB)
	manager := deps.MediaMgr
	if manager == nil {
		return errors.New("MediaManager 未初始化，请检查存储配置")
	}

	if flags.before <= 0 {
		flags.before = 24 * time.Hour
	}
	if flags.limit <= 0 {
		flags.limit = 100
	}

	filter := mediarepo.CleanupFilter{
		TenantUUID: strings.TrimSpace(flags.tenantUUID),
		Drivers:    flags.drivers,
		Before:     time.Now().Add(-flags.before),
		Limit:      flags.limit,
	}

	stats := executeCleanup(ctx, repo, manager, deps.AuditSvc, filter, flags.dryRun)

	logger.InfoF(ctx, "[media_tool.cleanup] 已扫描 %d 条记录，成功清理 %d 条，失败 %d 条", stats.processed, stats.succeeded, stats.failed)
	if stats.failed > 0 {
		return errors.New("部分清理任务失败，请检查日志")
	}
	return nil
}

func parseCleanupFlags(args []string) (cleanupFlags, error) {
	var cfg cleanupFlags
	var drivers string

	flagSet := flag.NewFlagSet(commandCleanup, flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	flagSet.StringVar(&cfg.configPath, "config", defaultConfigPath, "配置文件路径")
	flagSet.BoolVar(&cfg.dryRun, "dry-run", false, "仅打印待清理对象，不执行删除")
	flagSet.DurationVar(&cfg.before, "before", 24*time.Hour, "仅清理早于该时长的软删除记录")
	flagSet.IntVar(&cfg.limit, "limit", 100, "单次扫描的最大数量")
	flagSet.StringVar(&cfg.tenantUUID, "tenant", "", "仅处理指定租户 UUID，留空表示全部租户")
	flagSet.StringVar(&drivers, "drivers", "", "仅处理指定驱动，逗号分隔")

	if err := flagSet.Parse(args); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(drivers) != "" {
		cfg.drivers = splitAndTrim(drivers)
	}
	return cfg, nil
}

func loadConfig(path string) *config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		logger.WarnF(context.Background(), "加载配置失败(%v)，回退到默认配置", err)
		return config.GetDefaults()
	}
	return cfg
}

func executeCleanup(ctx context.Context, repo *mediarepo.AssetRepository, manager *mediamgr.MediaManager, audit auditsvc.Service, filter mediarepo.CleanupFilter, dryRun bool) cleanupStats {
	stats := cleanupStats{}
	processedKeys := make(map[string]struct{})
	for {
		assets, err := repo.CleanupCandidates(ctx, filter)
		if err != nil {
			logger.ErrorF(ctx, "查询待清理资产失败: %v", err)
			stats.failed++
			return stats
		}
		if len(assets) == 0 {
			return stats
		}

		var batch []mediamodel.MediaAsset
		for _, asset := range assets {
			key := fmt.Sprintf("%s:%s", asset.TenantUUID, asset.UUID.String())
			if _, seen := processedKeys[key]; seen {
				continue
			}
			processedKeys[key] = struct{}{}
			batch = append(batch, asset)
		}
		if len(batch) == 0 {
			// 所有候选项均已处理过（例如 dry-run 或删除失败），避免无限循环
			return stats
		}

		for _, asset := range batch {
			stats.processed++
			if dryRun {
				logger.InfoF(ctx, "[DRY-RUN] tenant=%s uuid=%s driver=%s key=%s", asset.TenantUUID, asset.UUID.String(), asset.Driver, asset.StorageKey)
				continue
			}
			if err := purgeObject(ctx, manager, &asset); err != nil {
				stats.failed++
				emitCleanupAudit(ctx, audit, &asset, err)
				logger.ErrorF(ctx, "清理对象失败 tenant=%s uuid=%s: %v", asset.TenantUUID, asset.UUID.String(), err)
				continue
			}
			if err := removeRecord(ctx, repo, &asset); err != nil {
				stats.failed++
				emitCleanupAudit(ctx, audit, &asset, err)
				logger.ErrorF(ctx, "删除数据库记录失败 tenant=%s uuid=%s: %v", asset.TenantUUID, asset.UUID.String(), err)
				continue
			}
			stats.succeeded++
			emitCleanupAudit(ctx, audit, &asset, nil)
			logger.InfoF(ctx, "[media_tool.cleanup] 已清理 tenant=%s uuid=%s", asset.TenantUUID, asset.UUID.String())
		}
		if len(assets) < filter.Limit {
			return stats
		}
	}
}

func purgeObject(ctx context.Context, manager *mediamgr.MediaManager, asset *mediamodel.MediaAsset) error {
	if manager == nil {
		return errors.New("media manager 未配置")
	}
	input := driver.DeleteObjectInput{
		Bucket:    asset.Bucket,
		ObjectKey: asset.StorageKey,
		Force:     true,
	}
	if err := manager.Delete(ctx, asset.Driver, input); err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return nil
		}
		return err
	}
	return nil
}

func removeRecord(ctx context.Context, repo *mediarepo.AssetRepository, asset *mediamodel.MediaAsset) error {
	if repo == nil || repo.BaseRepository == nil || repo.BaseRepository.DB == nil {
		return errors.New("repository 未初始化")
	}
	result := repo.BaseRepository.DB.WithContext(ctx).
		Unscoped().
		Where("tenant_uuid = ? AND uuid = ?", asset.TenantUUID, asset.UUID).
		Delete(&mediamodel.MediaAsset{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func emitCleanupAudit(ctx context.Context, audit auditsvc.Service, asset *mediamodel.MediaAsset, err error) {
	if audit == nil || asset == nil {
		return
	}
	meta := map[string]any{
		"driver":     asset.Driver,
		"bucket":     asset.Bucket,
		"object_key": asset.StorageKey,
	}
	if asset.DeletedAt.Valid {
		meta["deleted_at"] = asset.DeletedAt.Time.Format(time.RFC3339)
	}
	outcome := "SUCCESS"
	severity := "INFO"
	if err != nil {
		outcome = "FAILED"
		severity = "ERROR"
		meta["error"] = err.Error()
	}
	payload, _ := json.Marshal(meta)
	_ = audit.Emit(ctx, &dbmaudit.AuditEvent{
		OccurredAt:   time.Now(),
		TenantUUID:   asset.TenantUUID,
		Source:       "media.tool.cleanup",
		Operation:    "media.asset.cleanup",
		ResourceType: "media.asset",
		ResourceID:   asset.UUID.String(),
		Outcome:      outcome,
		Severity:     severity,
		Meta:         datatypes.JSON(payload),
	})
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	var out []string
	for _, item := range parts {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func closeSQL(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	if err = sqlDB.Close(); err != nil {
		logger.Warn(context.Background(), "关闭数据库连接失败: "+err.Error())
	}
}

func printRootUsage() {
	logger.InfoF(context.Background(), "用法: %s <子命令> [参数]", os.Args[0])
	logger.InfoF(context.Background(), "可用子命令:")
	logger.InfoF(context.Background(), "  %s\t清理软删除媒资并写入审计记录", commandCleanup)
	logger.InfoF(context.Background(), "示例:")
	logger.InfoF(context.Background(), "  go run ./cmd/media_tool %s --dry-run --before=24h", commandCleanup)
}

func printCommandUsage(cmd string) {
	switch cmd {
	case commandCleanup:
		flagSet := flag.NewFlagSet(commandCleanup, flag.ExitOnError)
		flagSet.SetOutput(os.Stderr)
		var dummy cleanupFlags
		var drivers string
		flagSet.StringVar(&dummy.configPath, "config", "etc/config.yaml", "配置文件路径")
		flagSet.BoolVar(&dummy.dryRun, "dry-run", false, "仅打印待清理对象，不执行删除")
		flagSet.DurationVar(&dummy.before, "before", 24*time.Hour, "仅清理早于该时长的软删除记录")
		flagSet.IntVar(&dummy.limit, "limit", 100, "单次扫描的最大数量")
		flagSet.StringVar(&dummy.tenantUUID, "tenant", "", "仅处理指定租户 UUID，留空表示全部租户")
		flagSet.StringVar(&drivers, "drivers", "", "仅处理指定驱动，逗号分隔")
		logger.InfoF(context.Background(), "用法: %s %s [参数]", os.Args[0], commandCleanup)
		flagSet.PrintDefaults()
	default:
		logger.ErrorF(context.Background(), "未知子命令: %s", cmd)
		printRootUsage()
	}
}
