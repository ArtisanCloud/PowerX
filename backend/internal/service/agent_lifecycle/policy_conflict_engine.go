package agent_lifecycle

import (
	"context"
	"strings"
)

// PolicyConflictEngine 用于检测租户自助表单的策略冲突。
type PolicyConflictEngine interface {
	Evaluate(ctx context.Context, in PolicyConflictInput) ([]PolicyConflict, error)
}

// PolicyEngineOptions 控制策略基线。
type PolicyEngineOptions struct {
	MaxRateLimit    int32
	ReservedAliases []string
}

type defaultPolicyConflictEngine struct {
	maxRateLimit    int32
	reservedAliases []string
}

// NewDefaultPolicyConflictEngine 构造基础策略引擎。
func NewDefaultPolicyConflictEngine(opts PolicyEngineOptions) PolicyConflictEngine {
	engine := &defaultPolicyConflictEngine{
		maxRateLimit:    opts.MaxRateLimit,
		reservedAliases: opts.ReservedAliases,
	}
	if engine.maxRateLimit == 0 {
		engine.maxRateLimit = 2000
	}
	if len(engine.reservedAliases) == 0 {
		engine.reservedAliases = []string{"root", "super", "global"}
	}
	return engine
}

func (e *defaultPolicyConflictEngine) Evaluate(_ context.Context, in PolicyConflictInput) ([]PolicyConflict, error) {
	var conflicts []PolicyConflict
	aliasNormalized := strings.ToLower(strings.TrimSpace(in.Alias))
	for _, reserved := range e.reservedAliases {
		if aliasNormalized == reserved || strings.Contains(aliasNormalized, reserved+"-") {
			conflicts = append(conflicts, PolicyConflict{
				Field:   "alias",
				Code:    "RESERVED_ALIAS",
				Message: "alias uses reserved keyword",
			})
			break
		}
	}
	if in.RateLimit > e.maxRateLimit {
		conflicts = append(conflicts, PolicyConflict{
			Field:   "rate_limit",
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "requested rate limit exceeds platform allowance",
		})
	}
	seen := make(map[string]struct{})
	for _, perm := range in.Permissions {
		perm = strings.TrimSpace(strings.ToLower(perm))
		if perm == "" {
			continue
		}
		if _, ok := seen[perm]; ok {
			conflicts = append(conflicts, PolicyConflict{
				Field:   "permissions",
				Code:    "DUPLICATED_PERMISSION",
				Message: "permission duplicated: " + perm,
			})
			break
		}
		seen[perm] = struct{}{}
	}
	return conflicts, nil
}
