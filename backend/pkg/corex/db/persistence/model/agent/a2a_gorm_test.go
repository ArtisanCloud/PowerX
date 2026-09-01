package agent

import "testing"

func TestParseTeamOrchestrationSpecRejectsCycle(t *testing.T) {
	_, err := ParseTeamOrchestrationSpec([]byte(`{
  "schema":"powerx.agent.team-orchestration/v1",
  "tasks":[
    {"task_id":"first","node_kind":"agent_handoff","assignee_role":"retriever","skill_id":"demo.first","stage":1,"depends_on":["second"]},
    {"task_id":"second","node_kind":"skill","assignee_role":"planner","skill_id":"demo.second","stage":2,"depends_on":["first"]}
  ]
}`))
	if err == nil {
		t.Fatal("expected cyclic orchestration to be rejected")
	}
}

func TestParseTeamOrchestrationSpecAcceptsDAG(t *testing.T) {
	_, err := ParseTeamOrchestrationSpec([]byte(`{
  "schema":"powerx.agent.team-orchestration/v1",
  "tasks":[
    {"task_id":"first","node_kind":"agent_handoff","assignee_role":"retriever","skill_id":"demo.first","stage":1},
    {"task_id":"second","node_kind":"skill","assignee_role":"planner","skill_id":"demo.second","stage":2,"depends_on":["first"]}
  ]
}`))
	if err != nil {
		t.Fatalf("expected valid DAG: %v", err)
	}
}
