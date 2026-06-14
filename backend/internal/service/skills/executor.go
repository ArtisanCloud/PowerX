package skills

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ExecuteInput struct {
	TenantUUID   string
	TraceID      string
	SkillID      string
	Version      string
	Entrypoint   string
	Payload      map[string]interface{}
	Context      map[string]interface{}
	Manifest     map[string]interface{}
	Source       string
	CapabilityID string
}

type SkillExecutor interface {
	CanHandle(in ExecuteInput) bool
	Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error)
}

func pickExecutor(executors []SkillExecutor, in ExecuteInput) SkillExecutor {
	for _, executor := range executors {
		if executor != nil && executor.CanHandle(in) {
			return executor
		}
	}
	return nil
}

func validateEntrypointAllowed(manifest map[string]interface{}, entrypoint string) error {
	allowed := normalizeEntrypoints(manifest["entrypoints"])
	if len(allowed) == 0 {
		return nil
	}
	target := strings.TrimSpace(entrypoint)
	if target == "" {
		return nil
	}
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return nil
		}
	}
	sort.Strings(allowed)
	return fmt.Errorf("entrypoint %q not allowed in manifest, allowed: %s", target, strings.Join(allowed, ","))
}

var errNoExecutorMatched = errors.New("no executor matched for skill")
