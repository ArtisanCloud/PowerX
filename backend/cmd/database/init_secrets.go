package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
)

// handleInitSecrets 在完整配置校验和数据库连接之前执行离线首次初始化。
func handleInitSecrets(args []string, output, errorOutput io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "init-secrets" {
		return false, 0
	}
	flags := flag.NewFlagSet("init-secrets", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	path := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if path == "" {
		path = "etc/config.yaml"
	}
	configPath := flags.String("config", path, "实际运行 config.yaml；生成一次 JWT 和 server.secret_key，保留已有有效密钥")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, 0
		}
		return true, 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errorOutput, "init-secrets 不接受额外参数")
		return true, 2
	}
	if os.Getenv("POWERX_UPGRADE_MODE") == "1" {
		fmt.Fprintln(errorOutput, "升级模式禁止首次密钥初始化；请保留已安装实例的原配置")
		return true, 1
	}
	result, err := config.PrepareInstallationSecrets(*configPath)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return true, 1
	}
	fmt.Fprintf(output, "密钥初始化完成：JWT=%s，server.secret_key=%s。配置已持久保存或沿用外部密钥；未连接数据库、未创建管理员。\n", result.JWT, result.Server)
	return true, 0
}
