package publish

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var packageOpts = struct {
	grpcAddr    string
	candidateID string
	artifact    string
	checksum    string
	offline     bool
	timeout     time.Duration
}{
	grpcAddr: defaultPipelineGRPCAddr,
	timeout:  60 * time.Second,
}

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Manage plugin release packages",
	Long:  "Uploads offline plugin packages to the release service.",
	RunE:  runPublishPackage,
}

func init() {
	Command.AddCommand(packageCmd)

	packageCmd.Flags().StringVar(&packageOpts.grpcAddr, "grpc-addr", packageOpts.grpcAddr, "Plugin release gRPC endpoint")
	packageCmd.Flags().StringVar(&packageOpts.candidateID, "candidate-id", "", "Release candidate identifier (required)")
	packageCmd.Flags().StringVar(&packageOpts.artifact, "artifact", "", "Path to the offline package archive (required)")
	packageCmd.Flags().StringVar(&packageOpts.checksum, "checksum", "", "Pre-computed SHA256 checksum (optional)")
	packageCmd.Flags().BoolVar(&packageOpts.offline, "offline", false, "Upload the artifact as an offline package")
	packageCmd.Flags().DurationVar(&packageOpts.timeout, "timeout", packageOpts.timeout, "RPC timeout")

	_ = packageCmd.MarkFlagRequired("candidate-id")
	_ = packageCmd.MarkFlagRequired("artifact")
}

func runPublishPackage(cmd *cobra.Command, _ []string) error {
	if !packageOpts.offline {
		return errors.New("only offline package uploads are supported; pass --offline")
	}
	file, err := os.Open(packageOpts.artifact)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), packageOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, packageOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	stream, err := client.UploadOfflinePackage(ctx)
	if err != nil {
		return fmt.Errorf("open upload stream: %w", err)
	}

	reader := bufio.NewReader(file)
	hash := sha256.New()
	buf := make([]byte, 512*1024)

	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := &pluginreleasepb.UploadOfflinePackageRequest{
				CandidateId: strings.TrimSpace(packageOpts.candidateID),
				Chunk:       buf[:n],
			}
			hash.Write(buf[:n])
			if err := stream.Send(chunk); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read artifact: %w", readErr)
		}
	}

	checksum := strings.TrimSpace(packageOpts.checksum)
	if checksum == "" {
		checksum = hex.EncodeToString(hash.Sum(nil))
	}
	if err := stream.Send(&pluginreleasepb.UploadOfflinePackageRequest{
		CandidateId: strings.TrimSpace(packageOpts.candidateID),
		Checksum:    checksum,
		Eof:         true,
	}); err != nil {
		return fmt.Errorf("send final chunk: %w", err)
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("close upload stream: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Offline package %s stored at %s\n", resp.GetOfflinePackageId(), resp.GetPackageUri())
	return nil
}
