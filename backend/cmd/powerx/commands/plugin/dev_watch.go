package plugin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	defaultGRPCAddr  = "localhost:9090"
	defaultChunkSize = 256 * 1024 // 256 KiB
)

var (
	devCmd = &cobra.Command{
		Use:   "dev",
		Short: "Developer utilities for plugin hotload",
	}

	devWatchOpts = struct {
		grpcAddr     string
		tenantID     string
		developerID  uint64
		artifactPath string
		artifactURI  string
		featureFlags []string
		resetCache   bool
		token        string
		stop         bool
		chunkSize    int
		timeout      time.Duration
	}{
		stop:      true,
		chunkSize: defaultChunkSize,
		timeout:   30 * time.Second,
	}

	devWatchCmd = &cobra.Command{
		Use:   "watch",
		Short: "Push a hotload bundle to a tenant session",
		Long:  "Establish a local install session via gRPC, push artifact chunks, and optionally close the session once the update is applied.",
		RunE:  runDevWatch,
	}
)

func init() {
	Command.AddCommand(devCmd)
	devCmd.AddCommand(devWatchCmd)

	devWatchCmd.Flags().StringVar(&devWatchOpts.grpcAddr, "grpc-addr", defaultGRPCAddr, "Plugin release gRPC endpoint")
	devWatchCmd.Flags().StringVar(&devWatchOpts.tenantID, "tenant-id", "", "Tenant identifier used during local install (required)")
	devWatchCmd.Flags().Uint64Var(&devWatchOpts.developerID, "developer-id", 0, "Developer identifier used for the session (required)")
	devWatchCmd.Flags().StringVar(&devWatchOpts.artifactPath, "artifact", "", "Path to the hotload artifact to stream via PushHotReload")
	devWatchCmd.Flags().StringVar(&devWatchOpts.artifactURI, "artifact-uri", "", "Override artifact URI sent in StartLocalInstall; defaults to derived file:// URI when --artifact is supplied")
	devWatchCmd.Flags().StringSliceVar(&devWatchOpts.featureFlags, "feature-flag", nil, "Feature flags to enable during the hotload session")
	devWatchCmd.Flags().BoolVar(&devWatchOpts.resetCache, "reset-cache", false, "Request backend cache reset before applying the hotload bundle")
	devWatchCmd.Flags().StringVar(&devWatchOpts.token, "token", "", "Bearer token used for gRPC metadata Authorization header")
	devWatchCmd.Flags().BoolVar(&devWatchOpts.stop, "stop", true, "Automatically stop the local install session after a successful push")
	devWatchCmd.Flags().IntVar(&devWatchOpts.chunkSize, "chunk-size", defaultChunkSize, "Hot reload stream chunk size in bytes")
	devWatchCmd.Flags().DurationVar(&devWatchOpts.timeout, "timeout", devWatchOpts.timeout, "Overall RPC timeout")

	_ = devWatchCmd.MarkFlagRequired("tenant-id")
	_ = devWatchCmd.MarkFlagRequired("developer-id")
}

func runDevWatch(cmd *cobra.Command, args []string) error {
	if devWatchOpts.chunkSize <= 0 {
		return errors.New("chunk-size must be positive")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), devWatchOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, devWatchOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)

	callCtx := attachAuth(ctx, devWatchOpts.token)

	artifactURI, err := resolveArtifactURI(devWatchOpts.artifactURI, devWatchOpts.artifactPath)
	if err != nil {
		return err
	}

	startResp, err := client.StartLocalInstall(callCtx, &pluginreleasepb.StartLocalInstallRequest{
		TenantId:     strings.TrimSpace(devWatchOpts.tenantID),
		DeveloperId:  devWatchOpts.developerID,
		ArtifactUri:  artifactURI,
		FeatureFlags: devWatchOpts.featureFlags,
		ResetCache:   devWatchOpts.resetCache,
	})
	if err != nil {
		return fmt.Errorf("start local install: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Session %s started for tenant %s developer %d\n", startResp.GetSessionId(), startResp.GetTenantId(), startResp.GetDeveloperId())

	if err := pushHotReload(callCtx, client, startResp.GetSessionId(), devWatchOpts.artifactPath, devWatchOpts.chunkSize, cmd.OutOrStdout()); err != nil {
		return fmt.Errorf("push hot reload: %w", err)
	}

	sessionInfo, err := client.GetLocalInstallSession(callCtx, &pluginreleasepb.GetLocalInstallSessionRequest{
		SessionId: startResp.GetSessionId(),
	})
	if err == nil {
		printSessionSummary(cmd, "Session status", sessionInfo)
	} else {
		logger.WarnF(callCtx, "fetch session status failed: %v", err)
	}

	if devWatchOpts.stop {
		stopResp, err := client.StopLocalInstall(callCtx, &pluginreleasepb.StopLocalInstallRequest{
			SessionId: startResp.GetSessionId(),
		})
		if err != nil {
			return fmt.Errorf("stop local install: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Session %s closed with status %s\n", stopResp.GetSessionId(), stopResp.GetStatus())
	}

	return nil
}

func pushHotReload(ctx context.Context, client pluginreleasepb.PluginReleaseServiceClient, sessionID, artifactPath string, chunkSize int, out io.Writer) error {
	stream, err := client.PushHotReload(ctx)
	if err != nil {
		return err
	}

	var (
		sequence int64 = 1
		sentAny        = false
	)

	if strings.TrimSpace(artifactPath) != "" {
		file, err := os.Open(artifactPath)
		if err != nil {
			return fmt.Errorf("open artifact: %w", err)
		}
		defer file.Close()

		reader := bufio.NewReader(file)
		buf := make([]byte, chunkSize)

		for {
			n, readErr := io.ReadFull(reader, buf)
			if errors.Is(readErr, io.ErrUnexpectedEOF) {
				// final chunk less than requested size
			} else if readErr != nil && !errors.Is(readErr, io.EOF) {
				return fmt.Errorf("read artifact: %w", readErr)
			}

			if n > 0 {
				chunk := &pluginreleasepb.HotReloadChunk{
					SessionId: sessionID,
					Sequence:  sequence,
					Content:   buf[:n],
				}
				sequence++
				sentAny = true
				if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
					chunk.Eof = true
				}
				if err := stream.Send(chunk); err != nil {
					return fmt.Errorf("send chunk: %w", err)
				}
			}

			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				break
			}
		}
	}

	if !sentAny {
		// send sentinel EOF chunk to hint backend even when no artifact provided
		if err := stream.Send(&pluginreleasepb.HotReloadChunk{
			SessionId: sessionID,
			Sequence:  sequence,
			Eof:       true,
		}); err != nil {
			return fmt.Errorf("send empty chunk: %w", err)
		}
	}

	ack, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("close hot reload stream: %w", err)
	}

	fmt.Fprintf(out, "Hot reload applied (seq=%d status=%s)\n", ack.GetAppliedSequence(), ack.GetStatus())
	return nil
}

func resolveArtifactURI(explicitURI, artifactPath string) (string, error) {
	if strings.TrimSpace(explicitURI) != "" {
		return explicitURI, nil
	}
	if strings.TrimSpace(artifactPath) == "" {
		return "", nil
	}
	abs, err := filepath.Abs(artifactPath)
	if err != nil {
		return "", fmt.Errorf("resolve artifact path: %w", err)
	}
	return "file://" + filepath.ToSlash(abs), nil
}

func attachAuth(ctx context.Context, token string) context.Context {
	token = strings.TrimSpace(token)
	if token == "" {
		return ctx
	}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", token)
}

func printSessionSummary(cmd *cobra.Command, title string, session *pluginreleasepb.LocalInstallSession) {
	if session == nil {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title)
	fmt.Fprintf(cmd.OutOrStdout(), "  Session: %s\n", session.GetSessionId())
	fmt.Fprintf(cmd.OutOrStdout(), "  Status : %s\n", session.GetStatus())
	if session.GetLogUrl() != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Log URL: %s\n", session.GetLogUrl())
	}
}
