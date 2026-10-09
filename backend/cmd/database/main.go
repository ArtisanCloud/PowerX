// cmd/database/main.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/cmd/database/seed"
	"github.com/ArtisanCloud/PowerX/config"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	plugincredential "github.com/ArtisanCloud/PowerX/internal/service/plugin_credential"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

func main() {
	handled, err := handleDatabaseInformation(os.Args[1:], os.Stdout)
	if err != nil {
		fatalf("%v", err)
	}
	if handled {
		return
	}
	cmd := os.Args[1]
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	fs := flag.NewFlagSet("database", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "配置文件路径")
	confirm := fs.Bool("confirm", false, "确认执行修复")
	pluginID := fs.String("plugin-id", "", "要修复运行凭证的插件 ID")
	confirmDatabase := fs.String("confirm-database", "", "refresh 必须填写实际数据库名")
	confirmSchema := fs.String("confirm-schema", "", "refresh 必须填写实际 schema 名")
	policyID := fs.Uint64("policy-id", 0, "backup-run 指定策略；不填则只运行到期策略")
	keepRestoreDB := fs.Bool("keep-restore-database", false, "验证完成后保留本次隔离库用于业务核对")
	jobID := fs.Uint64("job-id", 0, "backup-restore-verify 的来源备份任务")
	rotate := fs.Bool("rotate", false, "无法恢复原凭证时，明确允许轮换 secret；须同时传 -confirm")
	if err := fs.Parse(os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fatalf("解析参数失败: %v", err)
	}

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("加载配置失败: %v", err)
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		fatalf("读取 server.secret_key 失败: %v", err)
	}

	logger.InitGlobalLogger(&cfg.LogConfig)
	if cmd == "refresh" {
		if err := validateRefreshEnvironment(cfg.Deployment.Env); err != nil {
			fatalf("%v", err)
		}
	}
	ctx := context.Background()
	// 连接数据库
	db, err := database.Connect(cfg.Database)
	if err != nil {
		fatalf("连接数据库失败: %v", err)
	}

	switch cmd {
	case "migrate":
		if err := MigrateDatabase(ctx, db, cfg); err != nil {
			fatalf("migrate failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "migrate ok")

	case "seed":
		if err := seed.SeedCoreX(ctx, db, cfg); err != nil {
			fatalf("seed failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "seed ok")

	case "seed-native-marketing-skills":
		if err := seed.SeedNativeMarketingSkills(db, cfg); err != nil {
			fatalf("native marketing skill seed failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{"module": "skills"}), "native marketing skill revisions published")

	case "refresh":
		var actualDB string
		if err := db.Raw("SELECT current_database()").Scan(&actualDB).Error; err != nil {
			fatalf("%v", err)
		}
		if err := validateRefreshConfirmation(actualDB, coremodel.PowerXSchema, *confirm, *confirmDatabase, *confirmSchema); err != nil {
			fatalf("%v", err)
		}
		logger.WarnF(logger.WithLogFields(ctx, map[string]interface{}{"database": actualDB, "schema": coremodel.PowerXSchema, "operation": "database.refresh"}), "explicit destructive refresh started")
		// 已确认，仅开发或测试库可以执行清空。
		if err := ResetDatabase(ctx, db); err != nil {
			fatalf("reset failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "reset ok")

		// 再 migrate
		if err := MigrateDatabase(ctx, db, cfg); err != nil {
			fatalf("migrate failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "migrate ok")

		// 最后 seed
		if err := seed.SeedCoreX(ctx, db, cfg); err != nil {
			fatalf("seed failed: %v", err)
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "seed ok")

	case "backup-run":
		svc := backupops.NewJobService(db)
		if *policyID == 0 {
			if err := svc.RunDue(ctx); err != nil {
				fatalf("scheduled backup failed: %v", err)
			}
			printJSON(map[string]any{"status": "ok", "mode": "due_policies"})
		} else {
			job, err := svc.TriggerJob(ctx, backupops.TriggerJobRequest{PolicyID: *policyID, Operator: "operator.cli"})
			if err != nil {
				fatalf("backup failed: %v", err)
			}
			printJSON(job)
			if job.Status != "success" {
				fatalf("backup job failed: %s", job.ErrorMessage)
			}
		}
	case "backup-restore-verify":
		keep := "0"
		if *keepRestoreDB {
			keep = "1"
		}
		if err := os.Setenv("POWERX_OPS_RESTORE_KEEP_DB", keep); err != nil {
			fatalf("%v", err)
		}
		record, err := backupops.NewRestoreDrillService(db).Trigger(ctx, backupops.TriggerRestoreDrillRequest{SourceJobID: *jobID, Operator: "operator.cli", Reason: "manual_restore_verify"})
		if err != nil {
			fatalf("restore verification failed: %v", err)
		}
		printJSON(record)
		if record.Status != "success" {
			fatalf("restore verification failed: %s", record.ReportURI)
		}
	case "status":
		status, err := databaseStatus(ctx, db)
		if err != nil {
			fatalf("status failed: %v", err)
		}
		printJSON(status)

	case "iam-report":
		report, err := iamsvc.NewIAMMigrationReportService(db).Report(ctx)
		if err != nil {
			fatalf("iam migration report failed: %v", err)
		}
		printJSON(report)

	case "iam-fix-owner":
		result, err := iamsvc.NewIAMMigrationReportService(db).FixMissingOwnersAsSystem(ctx)
		if err != nil {
			fatalf("iam migration fix-owner failed: %v", err)
		}
		printJSON(result)

	case "iam-fix-role-binding-duplicates":
		result, err := iamsvc.NewIAMMigrationReportService(db).FixDuplicateRoleBindingsAsSystem(ctx, *confirm)
		if err != nil {
			fatalf("iam migration fix role binding duplicates failed: %v", err)
		}
		printJSON(result)

	case "repair-agent-run-state":
		result, err := repairLegacyAgentRunStates(ctx, db, agenttrace.ConfigFromEnv().LocalDir, *confirm)
		if err != nil {
			fatalf("agent run state repair failed: %v", err)
		}
		printJSON(result)

	case "repair-plugin-runtime-credentials", "prepare-plugin-runtime-credentials":
		svc := plugincredential.NewRuntimeCredentialRepairService(db, cfg)
		opts := plugincredential.RuntimeCredentialRepairOptions{
			PluginID: *pluginID, Confirm: *confirm, Rotate: *rotate,
		}
		var result *plugincredential.RuntimeCredentialRepairResult
		var err error
		if cmd == "prepare-plugin-runtime-credentials" {
			result, err = svc.PrepareInstall(ctx, opts)
		} else {
			result, err = svc.Repair(ctx, opts)
		}
		if result != nil {
			printJSON(result)
		}
		if err != nil {
			fatalf("plugin runtime credential repair failed: %v", err)
		}

	default:
		fatalf("Unknown command: %s", cmd)
	}
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(os.Stderr, msg)
	logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "%s", msg)
	os.Exit(1)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatalf("encode json failed: %v", err)
	}
}

type tableStatus struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Rows   int64  `json:"rows"`
}

type dbStatus struct {
	Database                    string        `json:"database"`
	Tables                      []tableStatus `json:"tables"`
	DuplicateRoleBindingGroups  int64         `json:"duplicate_role_binding_groups"`
	MarketingKnowledgePackCount int64         `json:"marketing_knowledge_pack_count"`
	MarketingSkillCount         int64         `json:"marketing_skill_count"`
}

func databaseStatus(ctx context.Context, db interface {
	WithContext(context.Context) *gorm.DB
}) (*dbStatus, error) {
	tx := db.WithContext(ctx)
	out := &dbStatus{}
	if err := tx.Raw(`SELECT current_database()`).Scan(&out.Database).Error; err != nil {
		return nil, err
	}
	for _, table := range []string{
		"iam_role_binding",
		"skills_registry_records",
		"workflow_definitions",
		"workflow_pack_installations",
		"knowledge_chunks",
	} {
		status := tableStatus{Name: table}
		if err := tx.Raw(`SELECT to_regclass(?) IS NOT NULL`, "public."+table).Scan(&status.Exists).Error; err != nil {
			return nil, err
		}
		if status.Exists {
			if err := tx.Raw(fmt.Sprintf(`SELECT COUNT(1) FROM public.%s`, table)).Scan(&status.Rows).Error; err != nil {
				return nil, err
			}
		}
		out.Tables = append(out.Tables, status)
	}
	if err := tx.Raw(`
		SELECT COUNT(1)
		FROM (
			SELECT tenant_uuid, subject_uuid, role_uuid, data_scope
			FROM public.iam_role_binding
			WHERE subject_type = 'MEMBER'
			  AND data_scope = 'TENANT'
			  AND role_uuid IS NOT NULL
			  AND btrim(role_uuid) <> ''
			  AND subject_uuid IS NOT NULL
			  AND btrim(subject_uuid) <> ''
			GROUP BY tenant_uuid, subject_uuid, role_uuid, data_scope
			HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&out.DuplicateRoleBindingGroups).Error; err != nil {
		return nil, err
	}
	if err := tx.Raw(`SELECT COUNT(1) FROM public.workflow_definitions WHERE workflow_pack_key = ?`, "marketing_knowledge_capture").Scan(&out.MarketingKnowledgePackCount).Error; err != nil {
		return nil, err
	}
	if err := tx.Raw(`SELECT COUNT(1) FROM public.skills_registry_records WHERE skill_id IN (?, ?) AND status = ?`, "marketing.audio_or_document_parse", "marketing.extract_methodology", "published").Scan(&out.MarketingSkillCount).Error; err != nil {
		return nil, err
	}
	return out, nil
}
