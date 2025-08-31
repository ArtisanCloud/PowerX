// services/agent/bootstrap/intent_registry.go
package bootstrap

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	"strings"
)

func RegisterIntentsForAgent(agentID string, blueprintDirs ...string) error {
	mgr := agent.GetAgentManager()
	for _, dir := range blueprintDirs {
		specs, err := loader.ParseIntentSpecsFromDir(dir)
		if err != nil {
			return err
		}
		for _, spec := range specs {
			if err := mgr.RegisterFlowRoute(agentID, spec.FlowID, spec); err != nil {
				return err
			}
		}
	}
	diagRoutesOnce(agentID)
	return nil
}

func diagRoutesOnce(agentID string) {
	mgr := agent.GetAgentManager()
	specs := mgr.ListFlowRoutesByAgent(agentID)
	fmt.Printf("=== ROUTES: %d ===\n", len(specs))
	for _, sp := range specs {
		nm := 0
		for _, mt := range sp.Matchers {
			if strings.ToLower(string(mt.Type)) == "keyword" && len(mt.Any) > 0 {
				nm += len(mt.Any)
			}
			if strings.ToLower(string(mt.Type)) == "regex" || strings.ToLower(string(mt.Type)) == "pattern" {
				if mt.RegexValue() != "" {
					nm++
				}
			}
		}
		fmt.Printf("flow=%s matchers=%d group=%s weight=%.2f\n",
			sp.FlowID, nm, sp.Group, sp.Weight)
	}
}
