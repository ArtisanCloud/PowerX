package llm

import (
	"context"
)

type Candidate struct {
	FlowID string   `json:"flow_id"`
	Name   string   `json:"name,omitempty"`
	Hints  []string `json:"hints,omitempty"` // 可放 intent.examples.positive 前几条
}

type Result struct {
	FlowID     string  `json:"flow_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

type Classifier interface {
	Classify(ctx context.Context, question string, cands []Candidate) (Result, error)
	Name() string
}
