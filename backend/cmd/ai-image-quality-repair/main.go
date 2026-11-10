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
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agent "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

type report struct {
	DatabaseFingerprint string                       `json:"database_fingerprint"`
	Plan                agent.ImageQualityRepairPlan `json:"plan"`
	Applied             bool                         `json:"applied"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// 不启动 Core、worker 或迁移；预览与应用均输出到独立的 0600 报告。
func run(args []string) error {
	fs := flag.NewFlagSet("ai-image-quality-repair", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "etc/config.yaml", "")
	tenant := fs.String("tenant", "", "")
	output := fs.String("report", "", "")
	planPath := fs.String("plan", "", "")
	apply := fs.Bool("apply", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *output == "" || (*apply && (*planPath == "" || *tenant != "")) || (!*apply && (*tenant == "" || *planPath != "")) {
		return errors.New("AI_IMAGE_REPAIR_ARGUMENTS_INVALID")
	}
	var previous report
	if *apply {
		file, err := os.Open(*planPath)
		if err != nil {
			return errors.New("AI_IMAGE_REPAIR_PLAN_UNAVAILABLE")
		}
		defer file.Close()
		decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
		decoder.DisallowUnknownFields()
		var extra any
		if decoder.Decode(&previous) != nil || decoder.Decode(&extra) != io.EOF {
			return errors.New("AI_IMAGE_REPAIR_PLAN_INVALID")
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return errors.New("AI_IMAGE_REPAIR_CONFIG_INVALID")
	}
	identity, _ := json.Marshal(cfg.Database)
	sum := sha256.Sum256(identity)
	fingerprint := hex.EncodeToString(sum[:])
	if *apply && previous.DatabaseFingerprint != fingerprint {
		return errors.New("AI_IMAGE_REPAIR_DATABASE_MISMATCH")
	}
	cfg.Database.LogLevel = "silent"
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		return errors.New("AI_IMAGE_REPAIR_DATABASE_UNAVAILABLE")
	}
	pool, err := db.DB()
	if err != nil {
		return errors.New("AI_IMAGE_REPAIR_DATABASE_UNAVAILABLE")
	}
	defer pool.Close()
	registry := catalog.NewRegistry()
	if registry.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"config/agents/providers.d"}, FailIfEmpty: true}, nil) != nil {
		return errors.New("AI_IMAGE_REPAIR_CONTRACT_UNAVAILABLE")
	}
	catalog.GlobalAIRegister = registry
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("AI_IMAGE_REPAIR_REPORT_UNAVAILABLE")
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc := agent.NewAgentSettingService(db)
	result := report{DatabaseFingerprint: fingerprint}
	if *apply {
		result.Plan = previous.Plan
		result.Applied, err = svc.ApplyImageQualityRepair(ctx, previous.Plan)
	} else {
		var plan *agent.ImageQualityRepairPlan
		plan, err = svc.PreviewImageQualityRepair(ctx, *tenant)
		if err == nil {
			result.Plan = *plan
		}
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(file).Encode(result)
}
