//go:build ignore

package plugin_release

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	adminhandler "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAdminPublishAPIContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps, db := setupPluginReleaseDeps(t)

	engine := gin.New()
	protected := engine.Group("/api/admin")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx := reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{
			IsRoot: true,
			Roles:  []string{"system_admin"},
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminhandler.RegisterAPIRoutes(nil, protected, deps)

	createPayload := map[string]any{
		"tenantId":         "tenant-cli",
		"pluginId":         "com.powerx.helloworld",
		"version":          "0.1.0",
		"buildArtifactUri": "s3://objects/plugins/com.powerx.helloworld/0.1.0/package.tar.gz",
		"commitHash":       "8c4ad9d90c7a41d7817d615e42abf0a77610ff21",
		"releaseNotes":     "Release candidate via CLI publish flow with smoke validated artifacts.",
		"labels": map[string]string{
			"channel": "dev",
			"source":  "cli",
		},
		"metadata": map[string]any{
			"entry":  "/plugins/admin",
			"bundle": "web",
		},
	}
	body, err := json.Marshal(createPayload)
	require.NoError(t, err)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/internal/plugins/releases", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer admin")
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	engine.ServeHTTP(createResp, createReq)
	require.Equal(t, http.StatusCreated, createResp.Code)

	var createData struct {
		Code int `json:"code"`
		Data struct {
			CandidateID    string            `json:"candidateId"`
			ApprovalStatus string            `json:"approvalStatus"`
			GateStatus     string            `json:"gateStatus"`
			TenantID       string            `json:"tenantId"`
			PluginID       string            `json:"pluginId"`
			Version        string            `json:"version"`
			Labels         map[string]string `json:"labels"`
			ReleaseNotes   string            `json:"releaseNotes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createData))
	require.NotEmpty(t, createData.Data.CandidateID)
	require.Equal(t, "tenant-cli", createData.Data.TenantID)
	require.Equal(t, "com.powerx.helloworld", createData.Data.PluginID)
	require.Equal(t, "0.1.0", createData.Data.Version)
	require.Equal(t, "submitted", createData.Data.ApprovalStatus)
	require.Equal(t, "pending", createData.Data.GateStatus)

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/internal/plugins/releases/"+createData.Data.CandidateID, nil)
	getReq.Header.Set("Authorization", "Bearer admin")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getData struct {
		Code int `json:"code"`
		Data struct {
			CandidateID  string            `json:"candidateId"`
			ReleaseNotes string            `json:"releaseNotes"`
			Labels       map[string]string `json:"labels"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getData))
	require.Equal(t, createData.Data.CandidateID, getData.Data.CandidateID)
	require.Equal(t, createPayload["releaseNotes"], getData.Data.ReleaseNotes)
	require.Equal(t, createPayload["labels"], getData.Data.Labels)

	patchPayload := map[string]any{
		"releaseNotes":     "Release candidate updated after smoke verification and checklist review.",
		"buildArtifactUri": "s3://objects/plugins/com.powerx.helloworld/0.1.0/package-repacked.tar.gz",
		"labels": map[string]string{
			"channel": "dev",
			"source":  "cli",
			"qa":      "pending",
		},
	}
	patchBody, err := json.Marshal(patchPayload)
	require.NoError(t, err)

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/admin/internal/plugins/releases/"+createData.Data.CandidateID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer admin")
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp := httptest.NewRecorder()
	engine.ServeHTTP(patchResp, patchReq)
	require.Equal(t, http.StatusOK, patchResp.Code)

	var patchData struct {
		Code int `json:"code"`
		Data struct {
			CandidateID  string            `json:"candidateId"`
			ReleaseNotes string            `json:"releaseNotes"`
			Labels       map[string]string `json:"labels"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(patchResp.Body.Bytes(), &patchData))
	require.Equal(t, patchPayload["releaseNotes"], patchData.Data.ReleaseNotes)
	require.Equal(t, patchPayload["labels"], patchData.Data.Labels)

	content := []byte("package-content-from-cli")
	sum := sha256.Sum256(content)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	file, err := writer.CreateFormFile("artifact", "package.tar.gz")
	require.NoError(t, err)
	_, err = file.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("packageUri", "s3://objects/plugins/com.powerx.helloworld/0.1.0/package.tar.gz"))
	require.NoError(t, writer.WriteField("checksum", fmt.Sprintf("sha256:%x", sum)))
	require.NoError(t, writer.WriteField("signature", "sig-cli-publish"))
	require.NoError(t, writer.WriteField("dependencies", `["powerx>=1.0.0"]`))
	require.NoError(t, writer.WriteField("licenseReport", `{"status":"pending"}`))
	require.NoError(t, writer.Close())

	artifactReq := httptest.NewRequest(http.MethodPost, "/api/admin/internal/plugins/releases/"+createData.Data.CandidateID+"/artifacts", &buf)
	artifactReq.Header.Set("Authorization", "Bearer admin")
	artifactReq.Header.Set("Content-Type", writer.FormDataContentType())
	artifactResp := httptest.NewRecorder()
	engine.ServeHTTP(artifactResp, artifactReq)
	require.Equal(t, http.StatusCreated, artifactResp.Code)

	var artifactData struct {
		Code int `json:"code"`
		Data struct {
			OfflinePackageID uint64 `json:"offlinePackageId"`
			PackageURI       string `json:"packageUri"`
			Status           string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(artifactResp.Body.Bytes(), &artifactData))
	require.NotZero(t, artifactData.Data.OfflinePackageID)
	require.Equal(t, "s3://objects/plugins/com.powerx.helloworld/0.1.0/package.tar.gz", artifactData.Data.PackageURI)

	candidateUUID, err := uuid.Parse(createData.Data.CandidateID)
	require.NoError(t, err)
	var candidate models.PluginReleaseCandidate
	require.NoError(t, db.Where("uuid = ?", candidateUUID).Take(&candidate).Error)
	require.Equal(t, patchPayload["buildArtifactUri"], candidate.BuildArtifactURI)

	var offlinePackage models.OfflineDistributionPackage
	require.NoError(t, db.Where("release_candidate_id = ?", candidate.ID).Take(&offlinePackage).Error)
	require.Equal(t, artifactData.Data.OfflinePackageID, offlinePackage.ID)
	require.Equal(t, "sig-cli-publish", offlinePackage.SignatureFingerprint)

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/internal/plugins/releases?page=1&size=10&pluginId=com.powerx.helloworld", nil)
	listReq.Header.Set("Authorization", "Bearer admin")
	listResp := httptest.NewRecorder()
	engine.ServeHTTP(listResp, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listData struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				CandidateID          string `json:"candidateId"`
				PluginID             string `json:"pluginId"`
				PlanStatus           string `json:"planStatus"`
				OfflinePackageStatus string `json:"offlinePackageStatus"`
				OfflinePackageCount  int64  `json:"offlinePackageCount"`
			} `json:"items"`
			Pagination struct {
				Total int64 `json:"total"`
				Page  int   `json:"page"`
			} `json:"pagination"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listData))
	require.GreaterOrEqual(t, listData.Data.Pagination.Total, int64(1))
	found := false
	for _, item := range listData.Data.Items {
		if item.CandidateID == createData.Data.CandidateID {
			found = true
			require.Equal(t, "com.powerx.helloworld", item.PluginID)
			require.Equal(t, int64(1), item.OfflinePackageCount)
		}
	}
	require.True(t, found, "created candidate should be listed")
}
