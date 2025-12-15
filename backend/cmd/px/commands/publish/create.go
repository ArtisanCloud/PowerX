package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultPipelineGRPCAddr = "localhost:9090"

var createOpts = struct {
	grpcAddr         string
	tenantUUID       string
	pluginID         string
	version          string
	artifactURI      string
	commitHash       string
	releaseNotes     string
	releaseNotesFile string
	labels           []string
	timeout          time.Duration
}{
	grpcAddr: defaultPipelineGRPCAddr,
	timeout:  30 * time.Second,
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Submit a plugin release candidate",
	Long:  "Uploads metadata for a plugin release candidate and triggers the guardrail pipeline.",
	RunE:  runPublishCreate,
}

func init() {
	Command.AddCommand(createCmd)

	createCmd.Flags().StringVar(&createOpts.grpcAddr, "grpc-addr", createOpts.grpcAddr, "Plugin release gRPC endpoint")
	createCmd.Flags().StringVar(&createOpts.tenantUUID, "tenant-uuid", "", "Target tenant UUID (required)")
	createCmd.Flags().StringVar(&createOpts.pluginID, "plugin-id", "", "Plugin identifier (required)")
	createCmd.Flags().StringVar(&createOpts.version, "version", "", "Semantic version submitted for approval (required)")
	createCmd.Flags().StringVar(&createOpts.artifactURI, "artifact-uri", "", "Build artifact URI (required)")
	createCmd.Flags().StringVar(&createOpts.commitHash, "commit", "", "Commit hash associated with the build (required)")
	createCmd.Flags().StringVar(&createOpts.releaseNotes, "notes", "", "Release notes inline content")
	createCmd.Flags().StringVar(&createOpts.releaseNotesFile, "notes-file", "", "Path to a file containing release notes")
	createCmd.Flags().StringSliceVar(&createOpts.labels, "label", nil, "Optional labels (key=value)")
	createCmd.Flags().DurationVar(&createOpts.timeout, "timeout", createOpts.timeout, "RPC timeout")

	_ = createCmd.MarkFlagRequired("tenant-uuid")
	_ = createCmd.MarkFlagRequired("plugin-id")
	_ = createCmd.MarkFlagRequired("version")
	_ = createCmd.MarkFlagRequired("artifact-uri")
	_ = createCmd.MarkFlagRequired("commit")
}

func runPublishCreate(cmd *cobra.Command, _ []string) error {
	notes, err := resolveReleaseNotes()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), createOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, createOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	req := &pluginreleasepb.CreateReleaseCandidateRequest{
		TenantUuid:       strings.TrimSpace(createOpts.tenantUUID),
		PluginId:         strings.TrimSpace(createOpts.pluginID),
		Version:          strings.TrimSpace(createOpts.version),
		BuildArtifactUri: strings.TrimSpace(createOpts.artifactURI),
		CommitHash:       strings.TrimSpace(createOpts.commitHash),
		ReleaseNotes:     notes,
		Labels:           parseLabelPairs(createOpts.labels),
	}

	resp, err := client.CreateReleaseCandidate(ctx, req)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Release candidate %s (%s %s) submitted. Gate status=%s approval=%s\n",
		resp.GetCandidateId(),
		resp.GetPluginId(),
		resp.GetVersion(),
		resp.GetGateStatus(),
		resp.GetApprovalStatus(),
	)
	return nil
}

func resolveReleaseNotes() (string, error) {
	if note := strings.TrimSpace(createOpts.releaseNotes); note != "" {
		return note, nil
	}
	if strings.TrimSpace(createOpts.releaseNotesFile) == "" {
		return "", errors.New("either --notes or --notes-file must be provided")
	}
	content, err := os.ReadFile(createOpts.releaseNotesFile)
	if err != nil {
		return "", fmt.Errorf("read notes file: %w", err)
	}
	note := strings.TrimSpace(string(content))
	if note == "" {
		return "", errors.New("release notes file is empty")
	}
	return note, nil
}

func parseLabelPairs(pairs []string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
		}
		result[key] = value
	}
	return result
}
