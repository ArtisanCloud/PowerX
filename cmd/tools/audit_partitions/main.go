package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit/partition"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func main() {
	var (
		mode         string
		past         int
		future       int
		retentionMon int
		schema       string
		table        string
	)
	flag.StringVar(&mode, "mode", "", "ensure|retire|ensure-parent")
	flag.IntVar(&past, "past", 1, "months to pre-create in the past")
	flag.IntVar(&future, "future", 2, "months to pre-create in the future")
	flag.IntVar(&retentionMon, "retention", 6, "retain recent N months; older will be dropped")
	flag.StringVar(&schema, "schema", "public", "schema name")
	flag.StringVar(&table, "table", "audit_event", "table name")
	flag.Parse()

	if mode == "" {
		fmt.Println("usage: -mode=ensure|retire|ensure-parent [flags]")
		os.Exit(2)
	}

	cfg := config.GetGlobalConfig()
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db init failed: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pm := partition.NewManager(db, schema, table)

	switch mode {
	case "ensure-parent":
		if err := pm.EnsureParent(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "ensure-parent failed: %v\n", err)
			os.Exit(1)
		}
	case "ensure":
		now := time.Now()
		if ok, _ := pm.TryAdvisoryLock(ctx, 43210); ok {
			defer pm.AdvisoryUnlock(ctx, 43210)
			if err := pm.EnsureMonthlyPartitions(ctx, now, past, future); err != nil {
				fmt.Fprintf(os.Stderr, "ensure failed: %v\n", err)
				os.Exit(1)
			}
		}
	case "retire":
		// 计算 cutoff=本月首日 - retentionMon 月
		cutoff := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local).
			AddDate(0, -retentionMon, 0)
		if ok, _ := pm.TryAdvisoryLock(ctx, 43211); ok {
			defer pm.AdvisoryUnlock(ctx, 43211)
			if err := pm.RetireOlderThan(ctx, cutoff); err != nil {
				fmt.Fprintf(os.Stderr, "retire failed: %v\n", err)
				os.Exit(1)
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", mode)
		os.Exit(2)
	}
}
