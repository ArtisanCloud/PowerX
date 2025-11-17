package knowledge_space_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	vectorstorepkg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
)

func TestQAReasoningFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("qa-reasoning-flow", "v1")
	spaceA := env.CreateSpaceFixture("qa-reasoning-alpha", tpl)
	spaceB := env.CreateSpaceFixture("qa-reasoning-beta", tpl)
	require.NoError(t, env.ActivateSpace(spaceA.UUID))
	require.NoError(t, env.ActivateSpace(spaceB.UUID))

	env.VectorStore.SetQueryResponse(spaceA.UUID, vectorstorepkg.QueryResponse{
		Matches: []vectorstorepkg.QueryMatch{
			{ChunkID: spaceA.UUID, Score: 0.91},
		},
	})
	env.VectorStore.SetQueryResponse(spaceB.UUID, vectorstorepkg.QueryResponse{
		Matches: []vectorstorepkg.QueryMatch{
			{ChunkID: spaceB.UUID, Score: 0.77},
		},
	})

	engine := env.Engine()
	body := map[string]any{
		"tenantId":        env.TenantID().String(),
		"intent":          "供应商是否超限",
		"domainTags":      []string{"finance", "policy"},
		"latencyBudgetMs": 1600,
		"sessionId":       "integration-session",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/retrieval-plan", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	lastQuery := env.VectorStore.LastQuery()
	expectedSpaces := []uuid.UUID{spaceA.UUID, spaceB.UUID}
	require.Contains(t, expectedSpaces, lastQuery.SpaceID)

	// Start gRPC client to verify retrieval is also exposed there and memory snapshot persists state.
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })
	server := env.GRPCServer()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceQABridgeServiceClient(conn)

	planResp, err := client.PlanRetrieval(ctx, &knowledgev1.QARetrievalPlanRequest{
		TenantId:        env.TenantID().String(),
		Intent:          "供应商是否超限",
		DomainTags:      []string{"finance"},
		SessionId:       "integration-session",
		LatencyBudgetMs: 1600,
	})
	require.NoError(t, err)
	require.Len(t, planResp.GetCandidateSpaces(), 2)

	_, err = client.UpsertMemorySnapshot(ctx, &knowledgev1.QAMemorySnapshotRequest{
		TenantId:  env.TenantID().String(),
		SessionId: "integration-session",
		Updates: []*knowledgev1.QAMemoryUpdate{
			{
				ChunkId:    "chunk-integration-1",
				SpaceId:    spaceA.UUID.String(),
				Citations:  []string{"doc#1"},
				Status:     "answered",
				SourceType: "pdf",
				Confidence: 0.95,
			},
		},
	})
	require.NoError(t, err)

	// Fetch through HTTP to ensure shared store.
	body = map[string]any{
		"tenantId":  env.TenantID().String(),
		"sessionId": "integration-session",
	}
	payload, _ = json.Marshal(body)
	req = httptest.NewRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/memory-snapshot", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	var snapshotResp struct {
		Code int `json:"code"`
		Data struct {
			Citations []struct {
				ChunkID string `json:"chunkId"`
			} `json:"citations"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &snapshotResp))
	require.Equal(t, http.StatusOK, snapshotResp.Code)
	require.Len(t, snapshotResp.Data.Citations, 1)
	require.Equal(t, "chunk-integration-1", snapshotResp.Data.Citations[0].ChunkID)
}
