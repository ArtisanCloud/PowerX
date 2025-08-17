package contract

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// IntentStrategy 所有意图识别策略必须实现这个接口
type IntentStrategy interface {
	Name() string
	Match(ctx context.Context, text string) (*schemas.IntentResult, error)
}

type LLMClassifier interface {
	Classify(ctx context.Context, question string, candidates []string) (flowID string, confidence float64, err error)
}
