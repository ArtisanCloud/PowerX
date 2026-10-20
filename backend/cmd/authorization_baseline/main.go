// authorization_baseline 是显式初始化工具；不在启动、迁移或 GET 中初始化。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	svc "github.com/ArtisanCloud/PowerX/internal/service/authorization_baseline"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "etc/config.yaml", "目标 Core 配置")
	tenant := flag.String("tenant", "", "租户 UUID")
	plugin := flag.String("plugin", "", "正式插件 ID")
	codes := flag.String("permissions", "", "已登记权限码，以逗号分隔")
	apply := flag.Bool("apply", false, "显式初始化，默认仅预检")
	guard := flag.String("activation-guard", "", "插件持久化写入门禁收据 UUID")
	actor := flag.String("operator", "", "操作人审计标识")
	quiesced := flag.Bool("confirm-plugin-quiesced", false, "已验证插件事件写入处于受控门禁内")
	noPending := flag.Bool("confirm-no-pending-events", false, "已在门禁内核对无待保留事件、游标和投递任务")
	flag.Parse()
	if *apply && (!*quiesced || !*noPending) {
		return fmt.Errorf("IAM_BASELINE_INITIALIZATION_GUARD_REQUIRED")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	config.GlobalConfig = cfg
	db, err := database.Connect(cfg.Database)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	permissions := []string{}
	for _, code := range strings.Split(*codes, ",") {
		permissions = append(permissions, strings.TrimSpace(code))
	}
	result, err := svc.NewService(db).Initialize(context.Background(), svc.InitializationInput{TenantUUID: *tenant, PluginID: *plugin, PermissionCodes: permissions, GuardUUID: *guard, Operator: *actor, Apply: *apply})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
