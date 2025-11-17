package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"google.golang.org/grpc"
)

func main() {
	endpoint := flag.String("endpoint", "127.0.0.1:9001", "KnowledgeSpace gRPC endpoint")
	cmd := flag.String("cmd", "upsert", "Command: upsert|publish|promote|rollback")
	matrixPath := flag.String("matrix", "configs/knowledge/tenant_release_matrix.yaml", "Matrix file for upsert")
	policyID := flag.Uint64("policy", 0, "Policy ID")
	versionID := flag.String("version", "", "Knowledge version ID")
	batchToken := flag.String("batch", "", "Batch token")
	alerts := flag.String("alerts", "", "Comma separated alert codes")
	reason := flag.String("reason", "", "Rollback reason")
	requestedBy := flag.String("by", "cli@powerx.io", "Requested by")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *endpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("dial gRPC failed: %v", err)
	}
	defer conn.Close()

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)

	switch strings.ToLower(*cmd) {
	case "upsert":
		req := buildUpsertRequest(*matrixPath)
		resp, err := client.UpsertReleasePolicy(ctx, req)
		exitOnErr(err)
		fmt.Printf("Policy saved: id=%s status=%s\n", resp.GetPolicyId(), resp.GetStatus())
	case "publish":
		ensure(*policyID > 0, "missing --policy")
		ensure(strings.TrimSpace(*versionID) != "", "missing --version")
		resp, err := client.PublishRelease(ctx, &knowledgev1.PublishReleaseRequest{
			PolicyId:    fmt.Sprintf("%d", *policyID),
			VersionId:   *versionID,
			RequestedBy: *requestedBy,
		})
		exitOnErr(err)
		fmt.Printf("Release published: releaseId=%s batchToken=%s tenants=%v\n", resp.GetReleaseId(), resp.GetBatchToken(), resp.GetTenants())
	case "promote":
		ensure(*policyID > 0, "missing --policy")
		ensure(strings.TrimSpace(*versionID) != "", "missing --version")
		ensure(strings.TrimSpace(*batchToken) != "", "missing --batch")
		resp, err := client.PromoteRelease(ctx, &knowledgev1.PromoteReleaseRequest{
			PolicyId:    fmt.Sprintf("%d", *policyID),
			VersionId:   *versionID,
			BatchToken:  *batchToken,
			Alerts:      splitAlerts(*alerts),
			RequestedBy: *requestedBy,
		})
		exitOnErr(err)
		fmt.Printf("Batch promoted: state=%s nextToken=%s tenants=%v coverage=%.2f\n", resp.GetState(), resp.GetNextBatchToken(), resp.GetTenants(), resp.GetTenantCoverage())
	case "rollback":
		ensure(*policyID > 0, "missing --policy")
		ensure(strings.TrimSpace(*versionID) != "", "missing --version")
		resp, err := client.RollbackRelease(ctx, &knowledgev1.RollbackReleaseRequest{
			PolicyId:    fmt.Sprintf("%d", *policyID),
			VersionId:   *versionID,
			Reason:      *reason,
			RequestedBy: *requestedBy,
		})
		exitOnErr(err)
		fmt.Printf("Rollback completed: status=%s\n", resp.GetStatus())
	default:
		log.Fatalf("unknown cmd %s", *cmd)
	}
}

func buildUpsertRequest(matrixPath string) *knowledgev1.UpsertReleasePolicyRequest {
	data, err := os.ReadFile(matrixPath)
	if err != nil {
		log.Fatalf("read matrix failed: %v", err)
	}
	var payload struct {
		MatrixVersion string   `json:"matrixVersion"`
		PilotTenants  []string `json:"pilotTenants"`
		Batches       []struct {
			Name    string   `json:"name"`
			Tenants []string `json:"tenants"`
		} `json:"batches"`
		Guardrails map[string]string `json:"guardrails"`
		ApprovedBy string            `json:"approvedBy"`
		CreatedBy  string            `json:"createdBy"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Fatalf("parse matrix failed: %v", err)
	}
	req := &knowledgev1.UpsertReleasePolicyRequest{
		MatrixVersion: payload.MatrixVersion,
		PilotTenants:  payload.PilotTenants,
		Guardrails:    payload.Guardrails,
		ApprovedBy:    payload.ApprovedBy,
		CreatedBy:     payload.CreatedBy,
	}
	for _, batch := range payload.Batches {
		req.Batches = append(req.Batches, &knowledgev1.ReleaseBatch{Name: batch.Name, Tenants: batch.Tenants})
	}
	return req
}

func splitAlerts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	tokens := strings.Split(raw, ",")
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func ensure(ok bool, msg string) {
	if !ok {
		log.Fatalf(msg)
	}
}

func exitOnErr(err error) {
	if err != nil {
		log.Fatalf("command failed: %v", err)
	}
}
