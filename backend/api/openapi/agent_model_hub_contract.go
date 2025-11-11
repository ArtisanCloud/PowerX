package docs

import _ "embed"

// AgentModelHubContract holds the OpenAPI stub for Agent Model Hub HTTP routes.
//
//go:embed agent_model_hub.http.yaml
var agentModelHubContract []byte

var AgentModelHubContract = agentModelHubContract
