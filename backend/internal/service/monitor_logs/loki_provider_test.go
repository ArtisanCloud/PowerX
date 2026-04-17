package monitorlogs

import (
	"strings"
	"testing"
)

func TestLokiProvider_BuildQuery_UsesFlexibleNumericJSONFilter(t *testing.T) {
	p := &LokiProvider{jobName: "powerx"}
	query := p.buildQuery(QueryRequest{
		JobID:    42,
		PolicyID: 7,
	})

	if !strings.Contains(query, `|~ "\\\"job_id\\\"\\s*:\\s*42"`) {
		t.Fatalf("job_id filter mismatch: %s", query)
	}
	if !strings.Contains(query, `|~ "\\\"policy_id\\\"\\s*:\\s*7"`) {
		t.Fatalf("policy_id filter mismatch: %s", query)
	}
}
