// snapshot-tool 提供离线包操作、独立协调器和受控实例启动；HTTP 不接受命令或路径。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	recoveryops "github.com/ArtisanCloud/PowerX/internal/service/system_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

var version = "unversioned"
var commit = "unknown"
var buildTime = "unknown"

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	action := flag.String("action", "list", "local、list、export、import、initialize-instance、coordinator、recovery-access、run-active、run-instance、resume-candidate")
	defaultDirectory, directorySource := snapshotops.DefaultStorage()
	if override := os.Getenv("POWERX_SNAPSHOT_DIRECTORY"); override != "" {
		defaultDirectory = override
		directorySource = "environment"
	}
	directory := flag.String("directory", defaultDirectory, "独立快照保存根目录（绝对路径；优先于 POWERX_SNAPSHOT_DIRECTORY）")
	id := flag.String("id", "", "要导出或初始化的快照 UUID")
	file := flag.String("file", "", "导入包或新的导出文件路径")
	passwordFile := flag.String("password-file", "", "仅当前用户可读的口令文件，不在命令行传明文口令")
	profileFile := flag.String("profile", "", "私有恢复协调器部署合同")
	runFile := flag.String("run-file", "", "协调器生成的私有实例启动文件")
	jobID := flag.String("job", "", "需要核对的候选维护任务 UUID")
	confirmation := flag.String("confirmation", "", "明确确认的维护任务或初始化快照 UUID")
	configFile := flag.String("config", "etc/config.yaml", "本地演练来源配置")
	coreFile := flag.String("core-binary", "./bin/powerx", "本地演练 Core 发布产物")
	home, _ := os.UserHomeDir()
	localDirectory := flag.String("local-directory", filepath.Join(home, ".powerx/r/local"), "本地受控演练目录")
	probesFile := flag.String("business-probes", "", "只读业务探针 JSON 文件，可选")
	overridesFile := flag.String("artifact-overrides", "", "本地演练明确批准的原生程序覆盖 JSON（path/source/sha256/version），不替换现有安装文件")
	webPort := flag.Int("web-port", 3030, "本地演练 Web 入口")
	apiPort := flag.Int("api-port", 8077, "本地演练 API 入口")
	prepareOnly := flag.Bool("prepare-only", false, "仅本地克隆及只读验证，不启动活动演练环境")
	generation := flag.Uint64("generation", 0, "明确核对的当前实例代次")
	showVersion := flag.Bool("version", false, "显示程序版本")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "directory" {
			directorySource = "explicit"
		}
	})
	if *showVersion {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"program": "snapshot-tool", "version": version, "commit": commit, "build_time": buildTime, "platform": runtime.GOOS + "/" + runtime.GOARCH})
	}
	if *action != "recovery-access" && *action != "initialize-instance" && *action != "recovery-status" && *action != "rollback-environment" && *action != "cleanup-environment" && *action != "local-preflight" && *action != "local" && *action != "list" && *action != "export" && *action != "import" && *action != "coordinator" && *action != "run-instance" && *action != "run-active" && *action != "resume-candidate" {
		return errors.New("未知操作；请使用 -help 查看支持的受控操作")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch *action {
	case "recovery-access":
		profile, e := recoveryops.ReadProfile(*profileFile)
		if e != nil {
			return e
		}
		store, e := snapshotops.NewCoordinatorStore(profile.Directory, profile.Targets)
		if e != nil {
			return e
		}
		coordinator := &recoveryops.Coordinator{Store: store}
		keyFile, e := coordinator.EnsureRecoveryAccess()
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"entry": "/__powerx_recovery/", "key_file": keyFile})
	case "initialize-instance":
		return recoveryops.InitializeInstallation(ctx, *profileFile, *id, *confirmation)
	case "recovery-status":
		state, e := recoveryops.EnvironmentStatus(*profileFile)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(state)
	case "rollback-environment", "cleanup-environment":
		job, e := recoveryops.QueueEnvironmentAction(ctx, *profileFile, *action, *jobID, *confirmation, *generation)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(job)

	case "local-preflight":
		return recoveryops.CheckLocalEntryPorts(*webPort, *apiPort)
	case "local":
		cfg, e := config.Load(*configFile)
		if e != nil {
			return e
		}
		core, e := filepath.Abs(*coreFile)
		if e != nil {
			return e
		}
		core, e = filepath.EvalSymlinks(core)
		if e != nil {
			return e
		}
		sources, e := bootstrap.LocalRecoverySources(cfg, core)
		if e != nil {
			return e
		}
		canonical := func(file string) (string, error) {
			file, e := filepath.Abs(file)
			if e != nil {
				return "", e
			}
			return filepath.EvalSymlinks(file)
		}
		locate := func(name string) (string, error) {
			file, e := exec.LookPath(name)
			if e != nil {
				return "", e
			}
			return canonical(file)
		}
		postgres, e := locate("postgres")
		if e != nil {
			return e
		}
		redis, e := locate("redis-server")
		if e != nil {
			return e
		}
		node, e := locate("node")
		if e != nil {
			return e
		}
		executable, e := os.Executable()
		if e != nil {
			return e
		}
		executable, e = canonical(executable)
		if e != nil {
			return e
		}
		sandbox := "/usr/bin/sandbox-exec"
		if runtime.GOOS != "darwin" {
			sandbox, e = locate("bwrap")
			if e != nil {
				return e
			}
		}
		cwd, e := os.Getwd()
		if e != nil {
			return e
		}
		cwd, e = canonical(cwd)
		if e != nil {
			return e
		}
		configuration, e := canonical(*configFile)
		if e != nil {
			return e
		}
		var probes []recoveryops.BusinessProbe
		if *probesFile != "" {
			raw, e := os.ReadFile(*probesFile)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &probes); e != nil {
				return e
			}
		}
		var overrides []recoveryops.ArtifactOverride
		if *overridesFile != "" {
			raw, e := os.ReadFile(*overridesFile)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &overrides); e != nil {
				return e
			}
		}
		return recoveryops.RunLocal(ctx, *localDirectory, sources, recoveryops.Deployment{ID: "source", Root: cwd, ConfigFile: configuration, CoreBinary: core, WebOutput: sources.Artifacts["core/web-admin"]}, recoveryops.Tools{PostgresBin: filepath.Dir(postgres), RedisServer: redis, Node: node, Coordinator: executable, Sandbox: sandbox}, probes, *webPort, *apiPort, *prepareOnly, overrides...)

	case "resume-candidate":
		job, e := recoveryops.QueueResumeCandidate(ctx, *profileFile, *jobID, *confirmation)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(job)
	case "coordinator":
		return recoveryops.RunCoordinator(ctx, *profileFile)
	case "run-instance":
		return recoveryops.RunInstance(ctx, *runFile)
	case "run-active":
		return recoveryops.RunActive(ctx, *profileFile)
	}
	claims := &reqctx.CoreXClaims{IsRoot: true}
	claims.Subject = "operator.snapshot_tool"
	ctx = reqctx.WithIsRoot(reqctx.WithClaims(ctx, claims), true)
	service := snapshotops.NewService(ctx, snapshotops.Config{Directory: *directory, DirectorySource: directorySource, Instance: "offline"}, snapshotops.Options{})
	defer service.Close()
	if *action == "list" {
		out, e := service.Overview(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"directory": out.Directory, "directory_source": out.DirectorySource, "snapshots": out.Snapshots, "restores": out.Restores})
	}
	if *file == "" || *passwordFile == "" {
		return errors.New("导入／导出必须指定 -file 和 -password-file")
	}
	info, e := os.Lstat(*passwordFile)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 2048 {
		return errors.New("口令文件必须为私有普通文件，最大 2048 字节")
	}
	secret, e := os.ReadFile(*passwordFile)
	if e != nil {
		return errors.New("无法读取口令文件")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(secret), "\n"), "\r")
	if *action == "import" {
		in, e := os.Open(*file)
		if e != nil {
			return e
		}
		defer in.Close()
		snapshot, e := service.Import(ctx, in, password)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(snapshot)
	}
	source, cleanup, e := service.ExportFile(ctx, *id, password)
	if e != nil {
		return e
	}
	defer cleanup()
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(*file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return errors.New("导出目标必须为不存在的新文件")
	}
	written := false
	defer func() {
		out.Close()
		if !written {
			_ = os.Remove(*file)
		}
	}()
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	if e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	written = true
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "exported", "snapshot_id": *id})
}
