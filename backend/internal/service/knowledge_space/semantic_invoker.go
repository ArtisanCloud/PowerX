package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
)

func invokeSemanticRetrieval(ctx context.Context, service *SemanticRuntime, tenant string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	var op struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(raw, &op) != nil {
		return nil, KnowledgeInvalidArgumentError(errors.New("operation required"))
	}
	switch op.Operation {
	case "query":
		var in struct {
			Operation string        `json:"operation"`
			Query     SemanticQuery `json:"query"`
		}
		if strictJSON(raw, &in) != nil {
			return nil, KnowledgeInvalidArgumentError(errors.New("expected typed semantic query"))
		}
		result, err := service.Query(ctx, tenant, in.Query)
		return map[string]any{"result": result}, err
	case "capabilities":
		var in struct {
			Operation string `json:"operation"`
			SpaceUUID string `json:"space_uuid"`
		}
		if strictJSON(raw, &in) != nil {
			return nil, KnowledgeInvalidArgumentError(errors.New("expected capabilities operation"))
		}
		result, err := service.Capabilities(ctx, tenant, in.SpaceUUID)
		return map[string]any{"capabilities": result}, err
	default:
		return nil, KnowledgeInvalidArgumentError(errors.New("unsupported retrieval operation"))
	}
}
