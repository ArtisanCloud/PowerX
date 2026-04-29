package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	capsync "github.com/ArtisanCloud/PowerX/internal/app/capability_sync"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

type workerFlags struct {
	configPath  string
	artifactDir string
	logFile     string
	watch       bool
	interval    time.Duration
}

func main() {
	flags := parseFlags()

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		fatalf("加载配置失败: %v", err)
	}
	config.GlobalConfig = cfg

	if err := ensureWorkerLogging(cfg, flags.logFile); err != nil {
		fatalf("初始化日志失败: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps, err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		fatalf("初始化核心依赖失败: %v", err)
	}
	defer closeDeps(deps)

	runner := capsync.NewRunner(deps, capsync.Options{
		ArtifactDir:  flags.artifactDir,
		Watch:        flags.watch,
		PollInterval: flags.interval,
	})

	if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fatalf("Capability Sync Worker 运行失败: %v", err)
	}
}

func parseFlags() workerFlags {
	var flags workerFlags
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	flag.StringVar(&flags.configPath, "config", defaultConfigPath, "配置文件路径")
	flag.StringVar(&flags.artifactDir, "artifacts", "tmp/plugins", "插件制品目录（.pxp 包所在位置）")
	flag.StringVar(&flags.logFile, "log-file", "logs/capability_sync.log", "日志文件路径")
	flag.BoolVar(&flags.watch, "watch", false, "是否以常驻轮询模式运行")
	flag.DurationVar(&flags.interval, "interval", 30*time.Second, "watch 模式下的轮询间隔")
	flag.Parse()
	return flags
}

func ensureWorkerLogging(cfg *config.Config, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if !cfg.LogConfig.File.Enable {
		cfg.LogConfig.File.Enable = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	cfg.LogConfig.File.InfoFilePath = path
	if strings.TrimSpace(cfg.LogConfig.File.ErrorFilePath) == "" {
		base := strings.TrimSuffix(path, filepath.Ext(path))
		ext := filepath.Ext(path)
		if ext == "" {
			cfg.LogConfig.File.ErrorFilePath = base + ".error.log"
		} else {
			cfg.LogConfig.File.ErrorFilePath = fmt.Sprintf("%s.error%s", base, ext)
		}
		if err := os.MkdirAll(filepath.Dir(cfg.LogConfig.File.ErrorFilePath), 0o755); err != nil {
			return fmt.Errorf("创建错误日志目录失败: %w", err)
		}
	}
	return nil
}

func closeDeps(deps *shared.Deps) {
	if deps == nil {
		return
	}
	if deps.AuditSvc != nil {
		deps.AuditSvc.Close()
	}
	closeSQL(deps.DB)
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
		pxlog.Warn(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "关闭数据库连接失败: "+err.Error())
	}
}

func fatalf(format string, args ...any) {
	pxlog.ErrorF(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), format, args...)
	os.Exit(1)
}
