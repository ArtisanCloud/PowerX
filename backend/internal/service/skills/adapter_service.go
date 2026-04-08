package skills

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	intent "github.com/ArtisanCloud/PowerX/internal/server/agent/intent"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// AdapterService bridges unified invocation requests to skill invocation.
type AdapterService struct {
	invokeSvc            *InvokeService
	bindingRepo          *skillrepo.SkillCapabilityBindingRepository
	sourcePolicyResolver SourcePolicyResolver
}

type UnifiedInvokeRequest struct {
	TenantUUID        string
	Env               string
	CapabilityID      string
	PreferredProtocol string
	ToolGrantIDs      []string
	AgentID           uint64
	Context           map[string]interface{}
	Payload           map[string]interface{}
	TraceID           string
}

type UnifiedInvokeResult struct {
	TraceID         string
	Status          string
	ProtocolUsed    string
	FallbackUsed    bool
	Result          map[string]interface{}
	SkillID         string
	Version         string
	SkillCandidates []map[string]interface{}
}

func NewAdapterService(invokeSvc *InvokeService, bindingRepo *skillrepo.SkillCapabilityBindingRepository) *AdapterService {
	if invokeSvc == nil || bindingRepo == nil {
		return nil
	}
	return &AdapterService{
		invokeSvc:   invokeSvc,
		bindingRepo: bindingRepo,
	}
}

func (s *AdapterService) WithSourcePolicyResolver(resolver SourcePolicyResolver) *AdapterService {
	if s == nil {
		return nil
	}
	s.sourcePolicyResolver = resolver
	return s
}

func (s *AdapterService) InvokeUnified(ctx context.Context, req UnifiedInvokeRequest) (*UnifiedInvokeResult, error) {
	if s == nil {
		return nil, nil
	}
	if strings.TrimSpace(strings.ToLower(req.PreferredProtocol)) != "skill" {
		return nil, nil
	}
	agentID := req.AgentID
	if agentID == 0 {
		agentID = extractAgentIDFromContext(req.Context)
	}
	allowedSources := defaultAllowedSources()
	if s.sourcePolicyResolver != nil {
		resolved := s.sourcePolicyResolver.ResolveAllowedSources(ctx, SourcePolicyInput{
			TenantUUID: req.TenantUUID,
			Env:        req.Env,
			AgentID:    agentID,
			Context:    req.Context,
		})
		if len(resolved) > 0 {
			allowedSources = resolved
		}
	}
	candidates, err := s.bindingRepo.ListMatchCandidates(ctx, skillrepo.SkillMatchFilter{
		CapabilityID:   strings.TrimSpace(req.CapabilityID),
		AllowedSources: allowedSources,
		AllowedScopes:  allowedScopesByTenant(req.TenantUUID),
		Limit:          256,
	})
	if err != nil {
		return nil, err
	}
	filtered := hardFilterCandidates(candidates, req.ToolGrantIDs)
	if len(filtered) == 0 {
		return nil, skillrepo.ErrSkillNotFound
	}
	ranked := intent.RecallAndRerankSkills(extractQueryText(req.Payload), toIntentCandidates(filtered), 8)
	if len(ranked) == 0 {
		return nil, skillrepo.ErrSkillNotFound
	}
	selected := ranked[0]
	rankedCandidates := make([]map[string]interface{}, 0, len(ranked))
	for i := range ranked {
		item := ranked[i]
		rankedCandidates = append(rankedCandidates, map[string]interface{}{
			"rank":         i + 1,
			"skill_id":     item.SkillID,
			"version":      item.Version,
			"source":       item.Source,
			"score":        item.Score,
			"reason":       item.Reason,
			"match_tokens": item.MatchTokens,
		})
	}
	executed, err := s.invokeSvc.Execute(ctx, InvokeRequest{
		TenantUUID: req.TenantUUID,
		SkillID:    selected.SkillID,
		Version:    selected.Version,
		InvokePath: "tenant.invocations",
		TraceID:    req.TraceID,
	}, req.Payload, req.Context)
	if err != nil {
		return nil, err
	}
	return &UnifiedInvokeResult{
		TraceID:         executed.TraceID,
		Status:          executed.Status,
		ProtocolUsed:    executed.ProtocolUsed,
		FallbackUsed:    executed.FallbackUsed,
		Result:          executed.Result,
		SkillID:         executed.SkillID,
		Version:         executed.Version,
		SkillCandidates: rankedCandidates,
	}, nil
}

func defaultAllowedSources() []string {
	return []string{"builtin", "plugin", "third_party"}
}

func allowedScopesByTenant(tenantUUID string) []string {
	if strings.TrimSpace(tenantUUID) == "" {
		return []string{"public", "global", "system"}
	}
	return []string{"tenant", "public", "global", "system"}
}

func hardFilterCandidates(candidates []skillrepo.SkillMatchCandidate, requestedToolGrants []string) []skillrepo.SkillMatchCandidate {
	grantSet := make(map[string]struct{}, len(requestedToolGrants))
	for _, g := range requestedToolGrants {
		trimmed := strings.TrimSpace(strings.ToLower(g))
		if trimmed == "" {
			continue
		}
		grantSet[trimmed] = struct{}{}
	}

	out := make([]skillrepo.SkillMatchCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !toolGrantAllowed(candidate.ToolGrants, grantSet) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func toolGrantAllowed(raw []byte, grantSet map[string]struct{}) bool {
	var bindingGrants []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &bindingGrants)
	}
	if len(bindingGrants) == 0 {
		return true
	}
	if len(grantSet) == 0 {
		return false
	}
	for _, g := range bindingGrants {
		if _, ok := grantSet[strings.ToLower(strings.TrimSpace(g))]; ok {
			return true
		}
	}
	return false
}

func extractQueryText(payload map[string]interface{}) string {
	if len(payload) == 0 {
		return ""
	}
	for _, key := range []string{"query", "input", "prompt", "text"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func toIntentCandidates(candidates []skillrepo.SkillMatchCandidate) []intent.SkillCandidate {
	out := make([]intent.SkillCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, intent.SkillCandidate{
			SkillID:      c.SkillID,
			Version:      c.Version,
			Source:       c.Source,
			UpdatedAt:    c.UpdatedAt,
			IntentHints:  intent.DecodeSkillStringArray([]byte(c.IntentHints)),
			Tags:         intent.DecodeSkillStringArray([]byte(c.Tags)),
			SemanticText: strings.TrimSpace(c.SemanticText),
		})
	}
	return out
}

func extractAgentIDFromContext(ctxMap map[string]interface{}) uint64 {
	if len(ctxMap) == 0 {
		return 0
	}
	for _, key := range []string{"agent_id", "agentId"} {
		raw, ok := ctxMap[key]
		if !ok {
			continue
		}
		if id := parseUintFromAny(raw); id > 0 {
			return id
		}
	}
	return 0
}

func parseUintFromAny(v interface{}) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case uint:
		return uint64(x)
	case int:
		if x > 0 {
			return uint64(x)
		}
	case int64:
		if x > 0 {
			return uint64(x)
		}
	case float64:
		if x > 0 {
			return uint64(x)
		}
	case string:
		n, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		if err == nil {
			return n
		}
	}
	return 0
}
