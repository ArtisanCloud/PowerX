package agent

import (
	"context"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	agentschema "github.com/ArtisanCloud/PowerX/services/agent/schemas"
)

func (m *Manager) Dispatch(ctx context.Context, msg string, metaCtx flowschema.Context, mt agentschema.ExecutionMeta) (string, interface{}, error) {
	return "", nil, nil
}
