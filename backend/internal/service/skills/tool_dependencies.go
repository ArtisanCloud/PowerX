package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
)

const ToolDependencySchema = "powerx.skill-tools/v1"

// ToolDependency 用稳定工具 key 与精确版本声明能力，不包含领域路由。
type ToolDependency struct {
	ToolKey      string `json:"tool_key"`
	Version      string `json:"version"`
	InputSchema  string `json:"input_schema"`
	OutputSchema string `json:"output_schema"`
	Permission   string `json:"permission"`
}
type ToolRequirements struct {
	Schema string           `json:"schema"`
	Tools  []ToolDependency `json:"tools"`
}
type ToolDependencyChecker func(context.Context, string, ToolDependency) error

func CalculatorDependency() ToolDependency {
	return ToolDependency{evidence.CalculatorKey, evidence.CalculatorVersion, evidence.DraftSchema, evidence.ReportSchema, "pure_calculation"}
}

// CheckToolDependencies 在发布、绑定及执行前复用。外部工具必须由受信任端口检查授权及版本。
func CheckToolDependencies(ctx context.Context, tenantUUID string, definition map[string]any, external ToolDependencyChecker) error {
	raw, exists := definition["tool_dependencies"]
	executor := nestedManifestMap(definition, "executor")
	if _, err := ManifestLLMParameters(nestedManifestMap(executor, "model_policy")); err != nil {
		return err
	}
	response := asStringInterface(executor["output_mode"]) == "response_envelope"
	if !exists {
		if response {
			return fmt.Errorf("skill.tool_dependencies_required")
		}
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var requirements ToolRequirements
	if err := evidence.Decode(b, &requirements); err != nil {
		return err
	}
	if requirements.Schema != ToolDependencySchema || requirements.Tools == nil {
		return fmt.Errorf("skill.tool_dependency_schema_invalid")
	}
	seen := map[string]bool{}
	for _, d := range requirements.Tools {
		if d.ToolKey == "" || d.Version == "" || d.InputSchema == "" || d.OutputSchema == "" || d.Permission == "" || seen[d.ToolKey] {
			return fmt.Errorf("skill.tool_dependency_invalid")
		}
		seen[d.ToolKey] = true
		if d.ToolKey == evidence.CalculatorKey {
			if d != CalculatorDependency() {
				return fmt.Errorf("skill.tool_dependency_version_or_contract_mismatch: %s", d.ToolKey)
			}
		} else {
			if external == nil {
				return fmt.Errorf("skill.tool_dependency_unavailable: %s", d.ToolKey)
			}
			if err := external(ctx, tenantUUID, d); err != nil {
				return err
			}
		}
	}
	if response {
		if asStringInterface(executor["response_contract"]) != evidence.ReportSchema {
			return fmt.Errorf("skill.response_contract_upgrade_required")
		}
		if !seen[evidence.CalculatorKey] {
			return fmt.Errorf("skill.calculation_dependency_required")
		}
		if len(evidenceSourcePointers(executor)) == 0 {
			return fmt.Errorf("skill.evidence_sources_required")
		}
		calculation, err := evidence.ReadCalculationPolicy(executor["calculation_policy"])
		if err != nil {
			return err
		}
		if _, err := evidence.ReadReviewPolicy(executor["review_policy"], calculation); err != nil {
			return err
		}
	}
	return nil
}

func evidenceSourcePointers(executor map[string]any) []string {
	var sources []string
	switch values := executor["evidence_sources"].(type) {
	case []string:
		sources = values
	case []any:
		for _, v := range values {
			if s, ok := v.(string); ok {
				sources = append(sources, s)
			} else {
				return nil
			}
		}
	}
	for _, source := range sources {
		if len(source) < 2 || source[0] != '/' {
			return nil
		}
	}
	return sources
}
