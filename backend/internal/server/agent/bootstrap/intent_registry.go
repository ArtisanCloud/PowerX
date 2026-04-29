// services/agent/bootstrap/intent_registry.go
package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"os"
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
	if !shouldPrintRouteDiag() {
		return
	}
	mgr := agent.GetAgentManager()
	specs := mgr.ListFlowRoutesByAgent(agentID)
	ctx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.intent_registry"})
	logger.InfoF(ctx, "=== ROUTES: %d ===", len(specs))
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
		logger.InfoF(ctx, "flow=%s matchers=%d group=%s weight=%.2f",
			sp.FlowID, nm, sp.Group, sp.Weight)
	}
}

func shouldPrintRouteDiag() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("POWERX_AGENT_ROUTE_DIAG")))
	switch v {
	case "1", "true", "yes", "on", "debug":
		return true
	default:
		return false
	}
}
