package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	media "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/redis/go-redis/v9"
)

// This operator command intentionally does not BootstrapApp: dry-run must not
// seed IAM, start workers, initialize plugins, or recover/enqueue any task.
func runDerivativeBackfill(args []string) error {
	fs := flag.NewFlagSet("derivatives-backfill", flag.ContinueOnError)
	path := fs.String("config", "etc/config.yaml", "目标 Core 配置；加载失败直接退出")
	tenant := fs.String("tenant", "", "可选租户筛选；省略时覆盖当前环境全部租户")
	assets := fs.String("assets", "", "资产 UUID，逗号分隔")
	dry := fs.Bool("dry-run", true, "只读扫描；执行登记须显式 --dry-run=false")
	batch := fs.Int("batch-size", 50, "每批扫描 1..200")
	concurrency := fs.Int("concurrency", 2, "每批处理并发 1..8")
	limit := fs.Int("limit", 0, "可选本次数量上限，默认 0 完整处理固定快照范围")
	after := fs.Uint64("after-id", 0, "上次完整输出记录的 ID")
	through := fs.Uint64("through-id", 0, "固定扫描上界；首轮由数据库快照确定")
	retry := fs.Bool("retry-failed", false, "显式为失败资产登记新的有限重试周期")
	reportPath := fs.String("report", "", "dry-run 清单 JSONL，独占创建，不覆盖已有文件")
	planPath := fs.String("plan", "", "执行已审阅的完整扫描清单；禁止重新扩大范围")
	progressPath := fs.String("progress", "", "执行进度 JSONL，默认 <plan>.progress.jsonl")
	confirm := fs.Bool("confirm", false, "确认执行清单，仅与 --plan --dry-run=false 一起使用")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *batch < 1 || *batch > 200 || *concurrency < 1 || *concurrency > 8 || *limit < 0 || fs.NArg() != 0 {
		return errors.New("tenant and valid batch/concurrency/limit required")
	}
	var plan *derivativePlan
	if *planPath != "" {
		if *dry || !*confirm || *reportPath != "" || *assets != "" || *after != 0 || *through != 0 {
			return errors.New("MEDIA_BACKFILL_PLAN_REQUIRES_CONFIRMED_APPLY_WITHOUT_SCOPE_OVERRIDES")
		}
		var err error
		plan, err = readDerivativePlan(*planPath)
		if err != nil {
			return err
		}
		if *tenant != "" && *tenant != plan.Header.TenantUUID {
			return errors.New("MEDIA_BACKFILL_PLAN_TENANT_MISMATCH")
		}
		*tenant = plan.Header.TenantUUID
		if *progressPath == "" {
			*progressPath = *planPath + ".progress.jsonl"
		}
	} else if *confirm || *progressPath != "" || (*reportPath != "" && !*dry) {
		return errors.New("MEDIA_BACKFILL_PLAN_FLAGS_INVALID")
	}
	*tenant = strings.TrimSpace(*tenant)
	if *tenant != "" && !canonicalMaintenanceUUID(*tenant) {
		return errors.New("MEDIA_BACKFILL_TENANT_INVALID")
	}
	if plan == nil && !*dry && *tenant == "" {
		return errors.New("MEDIA_BACKFILL_GLOBAL_APPLY_REQUIRES_REVIEWED_PLAN")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return errors.New("无法加载目标 Core 配置")
	}
	namespace, err := cfg.RedisNamespace()
	if err != nil {
		return err
	}
	if plan != nil && plan.Header.EnvironmentID != namespace {
		return errors.New("MEDIA_BACKFILL_PLAN_ENVIRONMENT_MISMATCH")
	}
	var progress *derivativeProgress
	if plan != nil {
		if sameMaintenanceFile(*progressPath, *path) {
			return errors.New("MEDIA_BACKFILL_PROGRESS_IS_CONFIG")
		}
		progress, err = openDerivativeProgress(*progressPath, *planPath, plan)
		if err != nil {
			return err
		}
		defer progress.close()
	}
	var report *os.File
	if *reportPath != "" {
		report, err = createDerivativeReport(*reportPath)
		if err != nil {
			return err
		}
		defer report.Close()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg.Database.LogLevel = "silent"
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		return errors.New("无法连接目标 Core 数据库")
	}
	defer closeSQL(db)
	_, svc := media.BuildMediaStack(ctx, db, nil, media.StorageOptions{ImageDerivatives: media.ImageDerivativeOptions{MaxSourceBytes: cfg.Storage.ImageDerivatives.MaxSourceBytes, MaxPixels: cfg.Storage.ImageDerivatives.MaxPixels, MaxWorkingBytes: cfg.Storage.ImageDerivatives.MaxWorkingBytes, MaxParallelJobs: cfg.Storage.ImageDerivatives.MaxParallelJobs}, RequireExistingLocalPath: true, DefaultDriver: cfg.Storage.DefaultDriver, TTLSeconds: cfg.Storage.TTLSeconds,
		Local: media.StorageLocalOptions{BasePath: cfg.Storage.Local.BasePath, PublicBaseURL: cfg.Storage.Local.PublicBaseURL},
		S3:    media.StorageS3Options{Endpoint: cfg.Storage.S3.Endpoint, Region: cfg.Storage.S3.Region, AccessKey: cfg.Storage.S3.AccessKey, SecretKey: cfg.Storage.S3.SecretKey, SessionToken: cfg.Storage.S3.SessionToken, Bucket: cfg.Storage.S3.Bucket, UseSSL: cfg.Storage.S3.UseSSL, ForcePathStyle: cfg.Storage.S3.ForcePathStyle, ExternalDomain: cfg.Storage.S3.ExternalDomain, PresignEndpoint: cfg.Storage.S3.PresignEndpoint},
	})
	if !*dry {
		addr, password, redisDB := cfg.Event.Fabric.RedisAddr, cfg.Event.Fabric.RedisPassword, cfg.Event.Fabric.RedisDB
		if addr == "" {
			addr = cfg.Queue.Redis.Addr
		}
		if addr == "" {
			addr = fmt.Sprintf("%s:%d", cfg.Cache.Host, cfg.Cache.Port)
		}
		if password == "" {
			password = cfg.Queue.Redis.Password
		}
		if password == "" {
			password = cfg.Cache.Password
		}
		if redisDB == 0 {
			redisDB = cfg.Queue.Redis.DB
		}
		client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: redisDB})
		defer client.Close()
		if err := client.Ping(ctx).Err(); err != nil {
			return errors.New("TaskBus Redis 不可用，未登记补图")
		}
		base, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute, namespace)
		if err != nil {
			return err
		}
		svc.ConfigureDerivativeQueue(taskqueue.NewStreamDriver(db, base))
		svc.SetDerivativeEventNamespace(namespace)
		svc.SetDerivativeEvents(func(ctx context.Context, tenant, asset string) {
			_ = client.Publish(ctx, svc.DerivativeEventChannel(tenant, asset), asset).Err()
		})
	}
	writer := io.Writer(os.Stdout)
	if report != nil {
		writer = io.MultiWriter(report, os.Stdout)
	}
	encoder := json.NewEncoder(writer)
	o := media.ImageDerivativeBackfillOptions{TenantUUID: strings.TrimSpace(*tenant), AllTenants: *tenant == "", AssetUUIDs: splitAndTrim(*assets), Apply: !*dry, BatchSize: *batch, Concurrency: *concurrency, Limit: *limit, AfterID: *after, ThroughID: *through, RetryFailed: *retry}
	if plan != nil {
		return executeDerivativePlan(ctx, svc, plan, progress, o, encoder)
	}
	if report != nil {
		scope := derivativeScopeTenant
		if o.AllTenants {
			scope = derivativeScopeAll
		}
		header := derivativePlanHeader{Contract: derivativePlanContract, EnvironmentID: namespace, Scope: scope, TenantUUID: o.TenantUUID, Recipe: media.ImageDerivativeRecipe, CreatedAt: time.Now().UTC(), AssetUUIDs: o.AssetUUIDs, AfterID: o.AfterID, ThroughID: o.ThroughID, Limit: o.Limit}
		if err := encoder.Encode(map[string]any{"kind": "plan", "plan": header}); err != nil {
			return err
		}
	}
	result, err := svc.BackfillImageDerivatives(ctx, o, func(item media.ImageDerivativeBackfillItem) error {
		if err := encoder.Encode(map[string]any{"kind": "asset", "result": item}); err != nil {
			return err
		}
		if report != nil {
			return report.Sync()
		}
		return nil
	})
	summary := map[string]any{"kind": "summary", "namespace": namespace, "result": result}
	if report != nil {
		summary["complete"] = err == nil
	}
	if e := encoder.Encode(summary); e != nil {
		return e
	}
	if report != nil {
		if e := report.Sync(); e != nil {
			return e
		}
	}
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return errors.New("部分资产失败，见 JSONL 结果；修复后可重跑同一范围")
	}
	return nil
}
