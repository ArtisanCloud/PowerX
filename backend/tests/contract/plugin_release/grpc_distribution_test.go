//go:build ignore

package plugin_release

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestPluginReleaseGRPC_DistributionLifecycle(t *testing.T) {
	deps, db := setupPluginReleaseDeps(t)

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	grpcserver.RegisterServer(server, deps)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	baseCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(baseCtx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	adminCtx := pluginReleaseGRPCContext(t, baseCtx, distributionContractTenantUUID)
	createResp, err := client.CreateReleaseCandidate(adminCtx, &pluginreleasepb.CreateReleaseCandidateRequest{
		TenantUuid:       distributionContractTenantUUID,
		PluginId:         "px.demo",
		Version:          "v3.0.0",
		BuildArtifactUri: "s3://bucket/releases/v3.0.0.zip",
		CommitHash:       "commit-sha-xyz",
		ReleaseNotes:     "Distribution flow smoke test",
		Labels: map[string]string{
			"coverage": "95",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.GetCandidateId())
	assertNoPluginReleaseTenantLeakProto(t, createResp)

	gateResp, err := client.RunQualityGates(adminCtx, &pluginreleasepb.RunQualityGatesRequest{
		CandidateId: createResp.GetCandidateId(),
	})
	require.NoError(t, err)
	assertNoPluginReleaseTenantLeakProto(t, gateResp)

	bundleContent := []byte("dummy offline bundle content for grpc test")
	checksum := fmt.Sprintf("%x", sha256.Sum256(bundleContent))
	uploadCtx := pluginReleaseGRPCContext(t, baseCtx, distributionContractTenantUUID)
	uploadStream, err := client.UploadOfflinePackage(uploadCtx)
	require.NoError(t, err)
	require.NoError(t, uploadStream.Send(&pluginreleasepb.UploadOfflinePackageRequest{
		CandidateId: createResp.GetCandidateId(),
		Chunk:       bundleContent[:25],
	}))
	require.NoError(t, uploadStream.Send(&pluginreleasepb.UploadOfflinePackageRequest{
		CandidateId: createResp.GetCandidateId(),
		Chunk:       bundleContent[25:],
		Checksum:    checksum,
		Eof:         true,
	}))
	uploadResp, err := uploadStream.CloseAndRecv()
	require.NoError(t, err)
	require.NotEmpty(t, uploadResp.GetOfflinePackageId())
	require.NotEmpty(t, uploadResp.GetPackageUri())
	assertNoPluginReleaseTenantLeakProto(t, uploadResp)

	packageID, err := strconv.ParseUint(uploadResp.GetOfflinePackageId(), 10, 64)
	require.NoError(t, err)

	var stored models.OfflineDistributionPackage
	require.NoError(t, db.WithContext(context.Background()).
		Where("id = ?", packageID).
		Take(&stored).Error)
	require.Equal(t, checksum, strings.ToLower(stored.Checksum))
	require.Equal(t, models.OfflinePackageStatusSubmitted, stored.Status)

	listingResp, err := client.SubmitMarketplaceListing(adminCtx, &pluginreleasepb.SubmitMarketplaceListingRequest{
		OfflinePackageId: uploadResp.GetOfflinePackageId(),
		Channel:          "online",
		PricingJson:      `{"tier":"enterprise"}`,
		SupportPolicyJson: `{
			"sla":"24x7"
		}`,
	})
	require.NoError(t, err)
	require.NotEmpty(t, listingResp.GetListingId())
	require.Equal(t, "pending", listingResp.GetReviewStatus())
	assertNoPluginReleaseTenantLeakProto(t, listingResp)

	importCtx := pluginReleaseGRPCContext(t, baseCtx, distributionImportTenantUUID)
	importResp, err := client.ImportOfflinePackage(importCtx, &pluginreleasepb.ImportOfflinePackageRequest{
		TenantUuid: distributionImportTenantUUID,
		PackageUri: uploadResp.GetPackageUri(),
		Checksum:   checksum,
		DryRun:     true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, importResp.GetJobId())
	require.Equal(t, "completed", strings.ToLower(importResp.GetStatus()))
	assertNoPluginReleaseTenantLeakProto(t, importResp)
}
