package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var deployOpts = struct {
	grpcAddr    string
	planID      string
	batchName   string
	finalAction string
	timeout     time.Duration
}{
	grpcAddr:    defaultPipelineGRPCAddr,
	finalAction: "promote",
	timeout:     60 * time.Second,
}

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Trigger or finalize plugin canary deployments",
	Long:  "Streams canary progress events via gRPC and optionally finalizes the deployment.",
	RunE:  runPublishDeploy,
}

func init() {
	Command.AddCommand(deployCmd)

	deployCmd.Flags().StringVar(&deployOpts.grpcAddr, "grpc-addr", deployOpts.grpcAddr, "Plugin release gRPC endpoint")
	deployCmd.Flags().StringVar(&deployOpts.planID, "plan-id", "", "Release plan identifier (required)")
	deployCmd.Flags().StringVar(&deployOpts.batchName, "batch-name", "", "Canary batch to trigger (required)")
	deployCmd.Flags().StringVar(&deployOpts.finalAction, "final-action", deployOpts.finalAction, "Finalize action after the canary (promote|rollback)")
	deployCmd.Flags().DurationVar(&deployOpts.timeout, "timeout", deployOpts.timeout, "Overall gRPC timeout")

	_ = deployCmd.MarkFlagRequired("plan-id")
	_ = deployCmd.MarkFlagRequired("batch-name")
}

func runPublishDeploy(cmd *cobra.Command, _ []string) error {
	if deployOpts.finalAction == "" {
		return errors.New("final-action must be provided")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), deployOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, deployOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	stream, err := client.TriggerCanary(ctx, &pluginreleasepb.TriggerCanaryRequest{
		PlanId:    deployOpts.planID,
		BatchName: deployOpts.batchName,
	})
	if err != nil {
		return fmt.Errorf("trigger canary: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Triggering canary batch %s for plan %s ...\n", deployOpts.batchName, deployOpts.planID)
	thresholdBreached := false
	for {
		progress, err := stream.Recv()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			if err == io.EOF {
				break
			}
			return fmt.Errorf("receive canary progress: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] phase=%s error_rate=%.4f threshold_breached=%v\n",
			progress.GetBatchName(),
			progress.GetPhase(),
			progress.GetErrorRate(),
			progress.GetThresholdBreached(),
		)
		if progress.GetThresholdBreached() {
			thresholdBreached = true
		}
	}

	action := strings.ToLower(strings.TrimSpace(deployOpts.finalAction))
	if thresholdBreached && action == "promote" {
		fmt.Fprintln(cmd.OutOrStdout(), "Threshold breached - overriding action to rollback.")
		action = "rollback"
	}
	finalResp, err := client.FinalizeDeployment(ctx, &pluginreleasepb.FinalizeDeploymentRequest{
		PlanId: deployOpts.planID,
		Action: action,
	})
	if err != nil {
		return fmt.Errorf("finalize deployment: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Plan %s finalized with state %s\n", finalResp.GetPlanId(), finalResp.GetFinalState())
	return nil
}
