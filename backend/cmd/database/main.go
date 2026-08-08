// cmd/database/main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/cmd/database/seed"
	"github.com/ArtisanCloud/PowerX/config"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("Usage: %s [migrate|seed|refresh|status|iam-report|iam-fix-owner|iam-fix-role-binding-duplicates]", os.Args[0])
	}
	cmd := os.Args[1]
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	fs := flag.NewFlagSet("database", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "配置文件路径")
	confirm := fs.Bool("confirm", false, "确认执行修复")
	if err := fs.Parse(os.Args[2:]); err != nil {
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

	case "refresh":
		// 先 drop database（或 drop all tables）
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
