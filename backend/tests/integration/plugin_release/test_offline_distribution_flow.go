package pluginreleaseintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	adminhandler "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	httpopenapi "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestOfflineDistributionFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := newPluginReleaseEnv(t)

	listener := bufconn.Listen(bufSize)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	grpcserver.RegisterServer(server, env.Deps)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	candidateResp, err := client.CreateReleaseCandidate(ctx, &pluginreleasepb.CreateReleaseCandidateRequest{
		TenantId:         "tenant-integration",
		PluginId:         "px.integration",
		Version:          "v4.0.0",
		BuildArtifactUri: "s3://bucket/releases/v4.0.0.zip",
		CommitHash:       "commit-int-4",
		ReleaseNotes:     "Offline distribution integration test",
	})
	require.NoError(t, err)

	_, err = client.RunQualityGates(ctx, &pluginreleasepb.RunQualityGatesRequest{
		CandidateId: candidateResp.GetCandidateId(),
	})
	require.NoError(t, err)

	content := []byte("offline bundle bytes for integration flow")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	stream, err := client.UploadOfflinePackage(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&pluginreleasepb.UploadOfflinePackageRequest{
		CandidateId: candidateResp.GetCandidateId(),
		Chunk:       content[:20],
	}))
	require.NoError(t, stream.Send(&pluginreleasepb.UploadOfflinePackageRequest{
		CandidateId: candidateResp.GetCandidateId(),
		Chunk:       content[20:],
		Checksum:    checksum,
		Eof:         true,
	}))
	uploadResp, err := stream.CloseAndRecv()
	require.NoError(t, err)

	router := gin.New()
	admin := router.Group("/api/admin")
	admin.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	adminhandler.RegisterAPIRoutes(nil, admin, env.Deps)

	openapi := router.Group("/api")
	openapi.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	httpopenapi.RegisterTenantRoutes(openapi, env.Deps)

	offlinePackageID, err := strconv.ParseUint(uploadResp.GetOfflinePackageId(), 10, 64)
	require.NoError(t, err)

	listingPayload := map[string]any{
		"offlinePackageId": offlinePackageID,
		"channel":          "online",
		"pricing": map[string]any{
			"tier": "enterprise",
		},
		"supportPolicy": map[string]any{
			"sla": "24x7",
		},
		"submissionForm": map[string]any{
			"notes": "integration listing",
		},
	}
	listingBody, _ := json.Marshal(listingPayload)
	listingReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/marketplace/listings", bytes.NewReader(listingBody))
	listingReq.Header.Set("Authorization", "Bearer admin")
	listingResp := httptest.NewRecorder()
	router.ServeHTTP(listingResp, listingReq)
	require.Equal(t, http.StatusCreated, listingResp.Code)

	var listingData struct {
		Code int `json:"code"`
		Data struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listingResp.Body.Bytes(), &listingData))

	reviewBody, _ := json.Marshal(map[string]string{"decision": "approved"})
	reviewReq := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/admin/plugin-release/marketplace/listings/%d/reviews", listingData.Data.ID),
		bytes.NewReader(reviewBody),
	)
	reviewReq.Header.Set("Authorization", "Bearer admin")
	reviewResp := httptest.NewRecorder()
	router.ServeHTTP(reviewResp, reviewReq)
	require.Equal(t, http.StatusOK, reviewResp.Code)

	importPayload := map[string]any{
		"tenantId":        "enterprise-tenant",
		"packageUri":      uploadResp.GetPackageUri(),
		"checksum":        checksum,
		"licenseAccepted": true,
	}
	importBody, _ := json.Marshal(importPayload)
	importReq := httptest.NewRequest(http.MethodPost, "/api/tenant/offline-imports", bytes.NewReader(importBody))
	importReq.Header.Set("Authorization", "Bearer tenant")
	importResp := httptest.NewRecorder()
	router.ServeHTTP(importResp, importReq)
	require.Equal(t, http.StatusAccepted, importResp.Code)

	var job struct {
		Code int `json:"code"`
		Data struct {
			JobID  string `json:"jobId"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(importResp.Body.Bytes(), &job))
	require.NotEmpty(t, job.Data.JobID)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/tenant/offline-imports/"+job.Data.JobID, nil)
	statusReq.Header.Set("Authorization", "Bearer tenant")
	statusResp := httptest.NewRecorder()
	router.ServeHTTP(statusResp, statusReq)
	require.Equal(t, http.StatusOK, statusResp.Code)

	var statusData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusResp.Body.Bytes(), &statusData))
	require.Equal(t, "completed", statusData.Data.Status)
}
