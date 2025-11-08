package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var importOpts = struct {
	grpcAddr   string
	tenantID   string
	packageURI string
	checksum   string
	dryRun     bool
	offline    bool
	token      string
	timeout    time.Duration
}{
	grpcAddr: defaultGRPCAddr,
	timeout:  30 * time.Second,
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import plugin packages into a tenant",
	Long:  "Triggers offline package import for a tenant via gRPC.",
	RunE:  runPluginImport,
}

func init() {
	Command.AddCommand(importCmd)

	importCmd.Flags().StringVar(&importOpts.grpcAddr, "grpc-addr", importOpts.grpcAddr, "Plugin release gRPC endpoint")
	importCmd.Flags().StringVar(&importOpts.tenantID, "tenant-id", "", "Target tenant identifier (required)")
	importCmd.Flags().StringVar(&importOpts.packageURI, "package-uri", "", "Offline package URI (required)")
	importCmd.Flags().StringVar(&importOpts.checksum, "checksum", "", "Package checksum (required)")
	importCmd.Flags().BoolVar(&importOpts.dryRun, "dry-run", false, "Validate without applying changes")
	importCmd.Flags().BoolVar(&importOpts.offline, "offline", false, "Import from offline package storage")
	importCmd.Flags().StringVar(&importOpts.token, "token", "", "Bearer token for Authorization metadata")
	importCmd.Flags().DurationVar(&importOpts.timeout, "timeout", importOpts.timeout, "RPC timeout")

	_ = importCmd.MarkFlagRequired("tenant-id")
	_ = importCmd.MarkFlagRequired("package-uri")
	_ = importCmd.MarkFlagRequired("checksum")
}

func runPluginImport(cmd *cobra.Command, _ []string) error {
	if !importOpts.offline {
		return errors.New("only offline imports are supported; pass --offline")
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), importOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, importOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	callCtx := attachAuth(ctx, importOpts.token)
	resp, err := client.ImportOfflinePackage(callCtx, &pluginreleasepb.ImportOfflinePackageRequest{
		TenantId:   strings.TrimSpace(importOpts.tenantID),
		PackageUri: strings.TrimSpace(importOpts.packageURI),
		Checksum:   strings.TrimSpace(importOpts.checksum),
		DryRun:     importOpts.dryRun,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Import job %s status=%s\n", resp.GetJobId(), resp.GetStatus())
	return nil
}
