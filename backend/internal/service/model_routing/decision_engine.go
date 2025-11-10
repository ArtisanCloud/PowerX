package model_routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"gorm.io/datatypes"
)

type DecisionEngine struct {
	signals ProviderSignalSource
	clock   func() time.Time
}

type DecisionEngineOptions struct {
	Signals ProviderSignalSource
	Clock   func() time.Time
}

type DecisionInput struct {
	TenantScope     string
	TaskContext     map[string]any
	SafeModeEnabled bool
	Budget          float64
}

type DecisionResult struct {
	PolicyVersion      uint32
	PrimaryProviderID  string
	FallbackChain      []string
	MatchedRulePattern string
	FallbackUsed       bool
	Reason             string
	TraceID            string
	SafeMode           bool
}

func NewDecisionEngine(opts DecisionEngineOptions) *DecisionEngine {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &DecisionEngine{
		signals: opts.Signals,
		clock:   clock,
	}
}

func (e *DecisionEngine) Decide(ctx context.Context, policy *model.RoutingPolicy, input DecisionInput) (*DecisionResult, error) {
	if policy == nil {
		return nil, errors.New("policy required")
	}
	rules, err := decodePolicyRules(policy.Rules)
	if err != nil {
		return nil, err
	}
	fallback := decodeFallback(policy.FallbackChain)

	res := &DecisionResult{
		PolicyVersion: policy.Version,
		FallbackChain: fallback,
	}

	if input.SafeModeEnabled {
		res.PrimaryProviderID = firstNonEmpty(fallback)
		if res.PrimaryProviderID == "" {
			res.PrimaryProviderID, res.FallbackUsed, res.Reason = e.bestEffortCandidate(ctx, rules, input, fallback)
			if !res.FallbackUsed {
				res.MatchedRulePattern = res.Reason
				res.Reason = "safe_mode"
			}
		} else {
			res.FallbackUsed = true
			res.Reason = "safe_mode"
		}
	} else {
		res.PrimaryProviderID, res.FallbackUsed, res.Reason = e.bestEffortCandidate(ctx, rules, input, fallback)
		if !res.FallbackUsed {
			res.MatchedRulePattern = res.Reason
			res.Reason = ""
		}
	}

	if res.PrimaryProviderID == "" {
		return nil, errors.New("no eligible providers")
	}
	return res, nil
}

func (e *DecisionEngine) bestEffortCandidate(ctx context.Context, rules []policyRule, input DecisionInput, fallback []string) (string, bool, string) {
	taskType := strings.TrimSpace(toString(input.TaskContext["taskType"]))
	var matched *policyRule
	for i := range rules {
		if matchesPattern(rules[i].TaskPattern, taskType) {
			matched = &rules[i]
			break
		}
	}
	if matched == nil && len(rules) > 0 {
		matched = &rules[0]
	}
	if matched == nil {
		return firstNonEmpty(fallback), true, "no_rules"
	}
	scores := make([]candidateScore, 0, len(matched.Candidates))
	for _, cand := range matched.Candidates {
		providerID := strings.TrimSpace(cand.ProviderID)
		if providerID == "" {
			continue
		}
		score := cand.Weight
		if score <= 0 {
			score = 0.1
		}
		signal := e.fetchSignal(ctx, providerID)
		health := normalizeHealth(signal.HealthScore)
		score *= health
		costPenalty := costFactor(signal.CostPerCall, matched.SLA.CostCeiling, input.Budget)
		score *= costPenalty
		if score <= 0 {
			continue
		}
		scores = append(scores, candidateScore{
			ProviderID: providerID,
			Score:      score,
		})
	}
	if len(scores) == 0 {
		return firstNonEmpty(fallback), true, "no_candidates"
	}
	best := scores[0]
	for _, c := range scores[1:] {
		if c.Score > best.Score {
			best = c
		}
	}
	return best.ProviderID, false, matched.TaskPattern
}

func (e *DecisionEngine) fetchSignal(ctx context.Context, providerID string) ProviderSignals {
	if e.signals == nil {
		return ProviderSignals{ProviderID: providerID}
	}
	return e.signals.Fetch(ctx, providerID)
}

type candidateScore struct {
	ProviderID string
	Score      float64
}

type policyRule struct {
	TaskPattern string            `json:"taskPattern"`
	Candidates  []policyCandidate `json:"candidates"`
	SLA         policyRuleSLA     `json:"sla"`
}

type policyCandidate struct {
	ProviderID string  `json:"providerId"`
	Weight     float64 `json:"weight"`
}

type policyRuleSLA struct {
	LatencyMs   float64 `json:"latencyMs"`
	CostCeiling float64 `json:"costCeiling"`
}

func decodePolicyRules(raw datatypes.JSON) ([]policyRule, error) {
	if len(raw) == 0 {
		return nil, errors.New("policy rules empty")
	}
	var rules []policyRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("decode policy rules: %w", err)
	}
	return rules, nil
}

func decodeFallback(raw datatypes.JSON) []string {
	var fallback []string
	if len(raw) == 0 {
		return fallback
	}
	_ = json.Unmarshal(raw, &fallback)
	return fallback
}

func firstNonEmpty(items []string) string {
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func matchesPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(pattern, "*") {
		ok, _ := path.Match(pattern, value)
		return ok
	}
	return strings.EqualFold(pattern, value)
}

func normalizeHealth(score float64) float64 {
	if score <= 0 {
		return 0.5
	}
	if score > 1 {
		return 1
	}
	return score
}

func costFactor(providerCost, ceiling, budget float64) float64 {
	if budget <= 0 && ceiling <= 0 {
		return 1
	}
	target := ceiling
	if providerCost > 0 {
		target = providerCost
	}
	if budget <= 0 || target <= 0 {
		return 1
	}
	if budget >= target {
		return 1
	}
	return budget / target
}

func toString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}
