package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

var version = "unversioned"
var commit = "unknown"
var buildTime = "unknown"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"program": "archive-worker", "version": version, "git_commit": commit, "build_time": buildTime, "go_version": runtime.Version()})
		return
	}
	defaults := os.Getenv("POWERX_OPS_ARCHIVE_CONFIG")
	if defaults == "" {
		defaults = filepath.Join(archiveops.DefaultDirectory(), "runtime-config.yaml")
	}
	path := flag.String("config", defaults, "私有归档配置文件")
	flag.Parse()
	cfg, e := archiveops.LoadConfig(*path)
	if e != nil {
		fmt.Fprintln(os.Stderr, "archive config unavailable:", e)
		os.Exit(1)
	}
	if cfg.StorageBudget != nil {
		c, err := budget.New(*cfg.StorageBudget)
		if err != nil {
			fmt.Fprintln(os.Stderr, "shared storage budget unavailable")
			os.Exit(1)
		}
		budget.SetDefault(c)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	svc := archiveops.NewService(ctx, cfg, archiveops.Options{})
	defer svc.Close()
	if e = svc.Tick(ctx); e != nil {
		fmt.Fprintln(os.Stderr, e)
		svc.Close()
		os.Exit(1)
	}
	fmt.Println("archive tick completed")
}
