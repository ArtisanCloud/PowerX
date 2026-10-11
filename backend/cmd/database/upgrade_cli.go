package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	logconfig "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gorm.io/gorm"
)

// 新参数在旧 CLI 加载配置前分流，所有失败路径也输出一份脱敏 JSON。
func handleUpgradeCommand(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "migrate" {
		return false, 0
	}
	requested := false
	for _, a := range args[1:] {
		if a == "--action" || a == "-action" || strings.HasPrefix(a, "--action=") || strings.HasPrefix(a, "-action=") {
			requested = true
		}
	}
	if !requested {
		if os.Getenv("POWERX_UPGRADE_MODE") == "1" {
			out := database.NewUpgradeResult("invalid", "")
			out.Block("UPGRADE_ACTION_REQUIRED", "Upgrade mode requires an explicit status, plan or apply action")
			_ = json.NewEncoder(stdout).Encode(out)
			return true, 2
		}
		return false, 0
	}
	fs := flag.NewFlagSet("database migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	action := fs.String("action", "", "status|plan|apply")
	format := fs.String("format", "json", "json only")
	target := fs.String("target-release", "", "exact sha-<40 character source SHA>")
	source := fs.String("source-sha", "", "verified source image SHA")
	path := fs.String("config", envUpgradeConfig(), "existing installed configuration")
	dataDir := fs.String("data-dir", "/data", "existing data mount")
	expected := fs.String("expected-plan", "", "exact plan hash")
	lockTimeout := fs.Duration("lock-timeout", 30*time.Second, "0..5m")
	offline := fs.Bool("confirm-offline", false, "all application and worker writers have been stopped")
	out := database.NewUpgradeResult("invalid", "")
	emit := func(code int) (bool, int) {
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			_, _ = fmt.Fprintln(stderr, "UPGRADE_RESULT_WRITE_FAILED")
			return true, 5
		}
		return true, code
	}
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		out.Block("UPGRADE_ARGUMENT_INVALID", "Invalid migration arguments")
		return emit(2)
	}
	if *action == "status" || *action == "plan" || *action == "apply" {
		out.Operation = *action
	}
	if regexp.MustCompile(`^sha-[a-f0-9]{40}$`).MatchString(*target) {
		out.TargetRelease = *target
	}
	if regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*source) {
		out.SourceSHA = *source
		out.SourceRelease = "sha-" + *source
	}
	if *format != "json" || (*action != "status" && *action != "plan" && *action != "apply") {
		out.Block("UPGRADE_ARGUMENT_INVALID", "Supported actions: status, plan, apply; format: json")
		return emit(2)
	}
	if *source != "" && !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*source) {
		out.Block("UPGRADE_SOURCE_INVALID", "Source SHA must contain exactly 40 lowercase hexadecimal characters")
		return emit(2)
	}
	if *action != "status" && (*target != "sha-"+commit || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(commit)) {
		out.Block("UPGRADE_TARGET_RELEASE_MISMATCH", "Use the release identity compiled into this target image")
		return emit(2)
	}
	if *action == "apply" && !*offline {
		out.Block("UPGRADE_OFFLINE_CONFIRMATION_REQUIRED", "Stop Core and worker writers before applying migrations")
		return emit(2)
	}
	// 不输出底层 SQL/配置错误正文，它们可能包含 DSN 或秘密值。
	logger.InitGlobalLogger(&logconfig.LogConfig{Level: "error", Console: false})
	cfg, fingerprint, err := config.LoadForUpgrade(*path, *dataDir)
	if err != nil {
		out.Block(err.Error(), "Installed configuration or data mount failed preflight")
		return emit(2)
	}
	config.GlobalConfig = cfg
	cfg.Database.LogLevel = "silent"
	db, err := database.Connect(cfg.Database)
	if err != nil {
		out.Block("UPGRADE_DATABASE_UNAVAILABLE", "Cannot connect to the configured database")
		return emit(2)
	}
	sqlDB, err := db.DB()
	if err != nil {
		out.Block("UPGRADE_DATABASE_UNAVAILABLE", "Cannot access database connection")
		return emit(2)
	}
	defer sqlDB.Close()
	steps, migrationVersion, err := installedUpgradeSteps(cfg)
	if err != nil {
		out.Block("UPGRADE_REGISTRY_INVALID", "Target image migration registry is invalid")
		return emit(2)
	}
	runner := &database.UpgradeRunner{DB: db, ReleaseID: "sha-" + commit, MigrationVersion: migrationVersion, SourceSHA: *source, Steps: steps, Fingerprint: fingerprint}
	if *action != "status" {
		runner.Preflight = func(ctx context.Context, conn *gorm.DB) ([]database.UpgradeBlocker, string, error) {
			_, current, err := config.LoadForUpgrade(*path, *dataDir)
			if err != nil {
				return []database.UpgradeBlocker{{Code: err.Error(), Message: "Configuration changed or became unavailable"}}, "", nil
			}
			if current != fingerprint {
				return []database.UpgradeBlocker{{Code: "UPGRADE_CONFIG_CHANGED", Message: "Configuration changed after the command started"}}, current, nil
			}
			return installedUpgradePreflight(ctx, conn, *source, current)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *action == "apply" {
		out, err = runner.Apply(ctx, *expected, *lockTimeout)
	} else {
		out, err = runner.Plan(ctx, *action)
	}
	if err != nil {
		out.Status = "failed"
		out.ErrorCode = "UPGRADE_DATABASE_OPERATION_FAILED"
		out.Message = "Database operation failed; no secret-bearing SQL is returned"
		out.Retryable = true
		return emit(4)
	}
	switch out.Status {
	case "blocked":
		if out.ErrorCode == "UPGRADE_LOCK_BUSY" || out.ErrorCode == "UPGRADE_PLAN_CHANGED" {
			return emit(3)
		}
		return emit(2)
	case "failed":
		if out.RequiresRestore {
			return emit(5)
		}
		return emit(4)
	}
	return emit(0)
}

func envUpgradeConfig() string {
	if p := os.Getenv("POWERX_CONFIG"); p != "" {
		return p
	}
	return "/etc/powerx/config.yaml"
}

// 先验证真实 schema / 数据，再允许已声明来源；不补记任意旧迁移为成功。
func installedUpgradePreflight(ctx context.Context, db *gorm.DB, source, configHash string) ([]database.UpgradeBlocker, string, error) {
	blockers := []database.UpgradeBlocker{}
	if source != supportedUpgradeSource && source != commit {
		blockers = append(blockers, database.UpgradeBlocker{Code: "UPGRADE_SOURCE_UNSUPPORTED", Message: "Source release is outside the fixed verified baseline"})
	}
	var serverVersion int
	if err := db.WithContext(ctx).Raw("SELECT current_setting('server_version_num')::integer").Scan(&serverVersion).Error; err != nil {
		return nil, "", err
	}
	if serverVersion < 140000 {
		blockers = append(blockers, database.UpgradeBlocker{Code: "UPGRADE_POSTGRES_UNSUPPORTED", Message: "PostgreSQL 14 or later is required; production release support is further restricted by the manifest"})
	}
	for _, name := range []string{"iam_user", "iam_tenant", "iam_member", "iam_role", "iam_permission"} {
		if !db.Migrator().HasTable(coremodel.PowerXSchema + "." + name) {
			blockers = append(blockers, database.UpgradeBlocker{Code: "UPGRADE_BASELINE_SCHEMA_MISSING", Message: "Installed IAM baseline tables are missing"})
		}
	}
	if db.Migrator().HasTable(coremodel.PowerXSchema + ".iam_user") {
		var roots int64
		if err := db.Table(coremodel.PowerXSchema+".iam_user").Where("is_root = ?", true).Count(&roots).Error; err != nil {
			return nil, "", err
		}
		if roots == 0 {
			blockers = append(blockers, database.UpgradeBlocker{Code: "UPGRADE_BASELINE_ADMIN_MISSING", Message: "Installed administrator is missing; Setup is not an upgrade fallback"})
		}
	}
	table := coremodel.PowerXSchema + "." + coremodel.TablePluginReleaseLocalInstallSessions
	if db.Migrator().HasTable(table) {
		var rows int64
		if db.Migrator().HasColumn(table, "plugin_id") && db.Migrator().HasColumn(table, "service_actor") {
			if err := db.Table(table).Where("plugin_id IS NULL OR trim(plugin_id) = '' OR service_actor IS NULL OR trim(service_actor) = ''").Count(&rows).Error; err != nil {
				return nil, "", err
			}
		} else {
			if err := db.Table(table).Count(&rows).Error; err != nil {
				return nil, "", err
			}
		}
		if rows > 0 {
			blockers = append(blockers, database.UpgradeBlocker{Code: "UPGRADE_LEGACY_PLUGIN_ACTOR_UNRESOLVED", Message: "Historical install sessions need audited owner resolution or retention archive; no rows were deleted"})
		}
	}
	// schema 指纹包含列、索引、约束，避免预览后 DDL 变化被忽略。
	var columns, indexes, constraints []string
	if err := db.Raw("SELECT table_name || ':' || column_name || ':' || data_type || ':' || is_nullable || ':' || coalesce(column_default,'') FROM information_schema.columns WHERE table_schema = ? ORDER BY table_name, ordinal_position", coremodel.PowerXSchema).Scan(&columns).Error; err != nil {
		return nil, "", err
	}
	if err := db.Raw("SELECT tablename || ':' || indexdef FROM pg_indexes WHERE schemaname = ? ORDER BY tablename,indexname", coremodel.PowerXSchema).Scan(&indexes).Error; err != nil {
		return nil, "", err
	}
	if err := db.Raw("SELECT conrelid::regclass::text || ':' || pg_get_constraintdef(c.oid) FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace WHERE n.nspname = ? ORDER BY conrelid,conname", coremodel.PowerXSchema).Scan(&constraints).Error; err != nil {
		return nil, "", err
	}
	body, _ := json.Marshal([]any{configHash, serverVersion, columns, indexes, constraints, blockers})
	sum := sha256.Sum256(body)
	return blockers, hex.EncodeToString(sum[:]), nil
}
