package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	media "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// 独立维护命令，不 BootstrapApp，不连接 Redis，不运行迁移或业务 worker。
func run(args []string) error {
	fs := flag.NewFlagSet("png-checksum-repair", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("config", "etc/config.yaml", "")
	tenant := fs.String("tenant", "", "")
	asset := fs.String("asset", "", "")
	report := fs.String("report", "", "")
	planFile := fs.String("plan", "", "")
	apply := fs.Bool("apply", false, "")
	confirm := fs.Bool("confirm", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *report == "" || (*apply && (!*confirm || *planFile == "" || *tenant != "" || *asset != "")) || (!*apply && (*confirm || *planFile != "" || *tenant == "" || *asset == "")) {
		return errors.New("MEDIA_PNG_REPAIR_ARGUMENTS_INVALID")
	}
	var plan media.PNGChecksumRepairPlan
	if *apply {
		f, err := os.Open(*planFile)
		if err != nil {
			return errors.New("MEDIA_PNG_REPAIR_PLAN_UNAVAILABLE")
		}
		d := json.NewDecoder(io.LimitReader(f, 1<<20))
		d.DisallowUnknownFields()
		err = d.Decode(&plan)
		var extra any
		if err == nil && d.Decode(&extra) != io.EOF {
			err = errors.New("MEDIA_PNG_REPAIR_PLAN_INVALID")
		}
		f.Close()
		if err != nil {
			return errors.New("MEDIA_PNG_REPAIR_PLAN_INVALID")
		}
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return errors.New("MEDIA_PNG_REPAIR_CONFIG_INVALID")
	}
	// 凭据只在进程内用于连接，不进入清单、日志或错误正文。
	identity, _ := json.Marshal([]any{cfg.Database, cfg.Storage})
	sum := sha256.Sum256(identity)
	environment := hex.EncodeToString(sum[:])
	cfg.Database.LogLevel = "silent"
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		return errors.New("MEDIA_PNG_REPAIR_DATABASE_UNAVAILABLE")
	}
	pool, err := db.DB()
	if err != nil {
		return errors.New("MEDIA_PNG_REPAIR_DATABASE_UNAVAILABLE")
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, svc := media.BuildMediaStack(ctx, db, nil, media.StorageOptions{ImageDerivatives: media.ImageDerivativeOptions{MaxSourceBytes: cfg.Storage.ImageDerivatives.MaxSourceBytes, MaxPixels: cfg.Storage.ImageDerivatives.MaxPixels, MaxWorkingBytes: cfg.Storage.ImageDerivatives.MaxWorkingBytes, MaxParallelJobs: cfg.Storage.ImageDerivatives.MaxParallelJobs}, RequireExistingLocalPath: true, DefaultDriver: cfg.Storage.DefaultDriver, TTLSeconds: cfg.Storage.TTLSeconds, Local: media.StorageLocalOptions{BasePath: cfg.Storage.Local.BasePath, PublicBaseURL: cfg.Storage.Local.PublicBaseURL}})
	out, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("MEDIA_PNG_REPAIR_REPORT_UNAVAILABLE")
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	if !*apply {
		plan, err = svc.ScanPNGChecksumRepair(ctx, *tenant, *asset, environment)
		if err != nil {
			return err
		}
		if err = encoder.Encode(plan); err != nil {
			return err
		}
	} else {
		variants, applyErr := svc.ApplyPNGChecksumRepair(ctx, plan, environment)
		result := map[string]any{"contract": "powerx.png_checksum_execution.v1", "plan_seal": plan.Seal, "asset_uuid": plan.Asset, "variants": variants, "success": applyErr == nil}
		if applyErr != nil {
			result["error_code"] = applyErr.Error()
		}
		if err = encoder.Encode(result); err != nil {
			return err
		}
		if err = out.Sync(); err != nil {
			return err
		}
		if applyErr != nil {
			return applyErr
		}
	}
	if err = out.Sync(); err != nil {
		return err
	}
	fmt.Println("MEDIA_PNG_REPAIR_COMPLETE")
	return nil
}
