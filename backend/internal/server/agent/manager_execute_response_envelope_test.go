package agent

import (
	"testing"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestResponseEnvelopeTaskRefsForTaskUsesOnlyTransitiveDependencies(t *testing.T) {
	plan := flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{
		{TaskID: "source_analysis"},
		{TaskID: "campaign_analysis", DependsOn: []string{"source_analysis"}},
		{TaskID: "knowledge_curation", DependsOn: []string{"source_analysis", "campaign_analysis"}},
		{TaskID: "review_summary", DependsOn: []string{"campaign_analysis", "knowledge_curation"}},
		{TaskID: "unrelated"},
	}}

	require.Equal(t,
		[]string{"campaign_analysis", "knowledge_curation", "source_analysis"},
		responseEnvelopeTaskRefsForTask(plan, "review_summary"),
	)
	require.Empty(t, responseEnvelopeTaskRefsForTask(plan, "source_analysis"))
}
