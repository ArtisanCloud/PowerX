package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
)

func main() {
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	configPath := flag.String("config", defaultConfigPath, "PowerX config path")
	tenantUUID := flag.String("tenant-uuid", "", "target tenant UUID")
	seedPath := flag.String("seed", metasvc.DefaultSeedPath, "metadata governance seed file")
	dryRun := flag.Bool("dry-run", false, "validate seed without writing")
	requireCanonical := flag.Bool("require-canonical", false, "require canonical seed definitions for plugin local or tenant bootstrap initialization")
	flag.Parse()

	if strings.TrimSpace(*tenantUUID) == "" {
		fatalf("metadata seed requires -tenant-uuid")
	}
	if *dryRun {
		seed, err := metasvc.LoadSeedFile(*seedPath)
		if err != nil {
			fatalf("load seed file: %v", err)
		}
		if *requireCanonical {
			if err := metasvc.ValidateCanonicalSeedDefinitions(seed); err != nil {
				fatalf("validate canonical seed definitions: %v", err)
			}
		}
		fmt.Printf("metadata seed valid: tenant_uuid=%s module=%s version=%d\n", *tenantUUID, seed.Module, seed.Version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("load config failed: %v", err)
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		fatalf("read server.secret_key failed: %v", err)
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		fatalf("connect database failed: %v", err)
	}
	service, err := metasvc.NewSeedService(metasvc.SeedServiceOptions{DB: db, SeedPath: *seedPath})
	if err != nil {
		fatalf("initialize metadata seed service: %v", err)
	}
	result, seed, err := service.Execute(context.Background(), metasvc.SeedExecutionInput{
		TenantUUID:                  *tenantUUID,
		SeedPath:                    *seedPath,
		RequireCanonicalDefinitions: *requireCanonical,
	})
	if err != nil {
		fatalf("seed metadata: %v", err)
	}
	fmt.Printf("metadata seed ok: tenant_uuid=%s module=%s dictionary_namespaces=%d dictionary_items=%d taxonomies=%d taxonomy_nodes=%d resource_types=%d tags=%d\n",
		*tenantUUID, seed.Module, result.DictionaryNamespaces, result.DictionaryItems, result.Taxonomies, result.TaxonomyNodes, result.ResourceTypes, result.Tags)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
