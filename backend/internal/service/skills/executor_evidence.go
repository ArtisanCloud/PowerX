package skills

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
)

//go:embed locales/evidence.*.json
var evidenceLocales embed.FS

type evidenceInstructions struct {
	Extraction string `json:"extraction"`
	Facts      string `json:"facts"`
	Notes      string `json:"notes"`
}

type EvidenceValidationError struct {
	Draft   evidence.Draft
	Details map[string]any
	Cause   error
}

func (e *EvidenceValidationError) Error() string { return e.Cause.Error() }
func (e *EvidenceValidationError) Unwrap() error { return e.Cause }

func (e *ManifestExecutor) executeEvidenceReport(ctx context.Context, in ExecuteInput, executor map[string]any, _ string) (map[string]any, error) {
	sources := evidenceSourcePointers(executor)
	locale := asStringInterface(in.Context["locale"])
	if locale != "zh-CN" && locale != "en-US" {
		return nil, fmt.Errorf("skill.executor_locale_not_supported")
	}
	raw, err := evidenceLocales.ReadFile("locales/evidence." + locale + ".json")
	if err != nil {
		return nil, err
	}
	var instructions evidenceInstructions
	if err := json.Unmarshal(raw, &instructions); err != nil {
		return nil, err
	}
	invoke := func(stage, instruction string, schema, payload map[string]any) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		result, err := e.llm(ctx, ManifestLLMInvocation{TenantUUID: in.TenantUUID, TraceID: in.TraceID, SkillID: in.SkillID, Version: in.Version, Entrypoint: in.Entrypoint, PromptTemplate: instruction, ResponseSchema: schema, ModelPolicy: nestedManifestMap(executor, "model_policy"), Payload: payload, Context: in.Context})
		if err != nil {
			return "", fmt.Errorf("skill.evidence.%s: %w", stage, err)
		}
		return result, nil
	}
	policy, err := evidence.ReadCalculationPolicy(executor["calculation_policy"])
	if err != nil {
		return nil, err
	}
	genericTokens, err := evidence.TokenizeGenericSources(in.Payload, sources)
	if err != nil {
		return nil, fmt.Errorf("skill.evidence.facts: %w", err)
	}
	text, err := invoke("facts", instructions.Facts, evidence.GenericFactsJSONSchema(genericTokens), map[string]any{"input": in.Payload, "numeric_tokens": genericTokens})
	if err != nil {
		return nil, err
	}
	var genericSelection evidence.GenericFactSelection
	if err := evidence.Decode([]byte(text), &genericSelection); err != nil {
		return nil, &EvidenceValidationError{Details: map[string]any{"stage": "facts_decode"}, Cause: fmt.Errorf("skill.evidence.facts: %w", err)}
	}
	genericFacts, err := evidence.ResolveGenericFacts(genericSelection, genericTokens)
	if err != nil {
		return nil, &EvidenceValidationError{Details: map[string]any{"stage": "facts_selection", "selection": genericSelection}, Cause: fmt.Errorf("skill.evidence.facts: %w", err)}
	}

	activeProfiles, profileErr := policy.DetectProfiles(in.Payload, sources, locale)
	if profileErr != nil && !errors.Is(profileErr, evidence.ErrActivityProfileUnmatched) {
		return nil, fmt.Errorf("skill.evidence.profile: %w", profileErr)
	}
	if profileErr != nil {
		activeProfiles = []string{}
	}
	descriptions, err := policy.InputDescriptions(locale, activeProfiles)
	if err != nil {
		return nil, err
	}
	tokens, err := evidence.TokenizeSources(in.Payload, sources, policy)
	if err != nil {
		return nil, fmt.Errorf("skill.evidence.source: %w", err)
	}
	text, err = invoke("source", instructions.Extraction, evidence.SelectionJSONSchema(policy, tokens, activeProfiles, locale), map[string]any{"input": in.Payload, "activity_profiles": activeProfiles, "fields": descriptions, "numeric_tokens": tokens})
	if err != nil {
		return nil, err
	}
	var selection evidence.SourceSelection
	if err := evidence.Decode([]byte(text), &selection); err != nil {
		return nil, &EvidenceValidationError{Details: map[string]any{"stage": "source_decode"}, Cause: fmt.Errorf("skill.evidence.source: %w", err)}
	}
	extracted, err := evidence.ResolveSelection(selection, tokens, policy, activeProfiles, locale)
	if err != nil {
		return nil, &EvidenceValidationError{Details: map[string]any{"stage": "source_selection", "profiles": activeProfiles, "selection": selection}, Cause: fmt.Errorf("skill.evidence.source: %w", err)}
	}
	plan, missing, err := policy.BuildPlan(extracted, activeProfiles, locale)
	if err != nil {
		return nil, &EvidenceValidationError{Details: map[string]any{"stage": "plan", "profiles": activeProfiles, "selection": selection}, Cause: fmt.Errorf("skill.evidence.plan: %w", err)}
	}
	plan.Data = mergeGenericFacts(plan.Data, genericFacts)
	prepared, err := evidence.ExecutePlan(ctx, plan, in.Payload, sources, in.TenantUUID, in.Version, in.TraceID)
	if err != nil {
		return nil, &EvidenceValidationError{Draft: evidence.Draft{Schema: evidence.DraftSchema, Kind: plan.Kind, Data: plan.Data, Calculations: plan.Calculations}, Cause: fmt.Errorf("skill.evidence.calculate: %w", err)}
	}
	// 第二阶段只解释已经执行的结果，不再承担算式、数值、原文提取或格式排版。
	notesContext, err := evidence.NotesContext(prepared, missing)
	if err != nil {
		return nil, err
	}
	text, err = invoke("notes", instructions.Notes, evidence.NotesJSONSchema(), notesContext)
	if err != nil {
		return nil, err
	}
	var notes evidence.Notes
	if err := evidence.Decode([]byte(text), &notes); err != nil {
		return nil, fmt.Errorf("skill.evidence.notes: %w", err)
	}
	if len(missing) > 0 && len(notes.Gaps) == 0 {
		return nil, fmt.Errorf("skill.evidence.notes: evidence.missing_inputs_unacknowledged")
	}
	report, err := evidence.AttachNotes(ctx, prepared, notes)
	if err != nil {
		return nil, &EvidenceValidationError{Draft: evidence.Draft{Schema: evidence.DraftSchema, Kind: plan.Kind, Data: plan.Data, Calculations: plan.Calculations, Hypotheses: notes.Hypotheses, Gaps: notes.Gaps, Actions: notes.Actions}, Cause: fmt.Errorf("skill.evidence.notes: %w", err)}
	}
	return map[string]any{"response_envelope": report}, nil
}

func mergeGenericFacts(declared []evidence.DraftDatum, generic []evidence.DraftDatum) []evidence.DraftDatum {
	seen := map[string]bool{}
	for _, datum := range declared {
		seen[datum.Source.Pointer+"\x00"+datum.Source.Literal+"\x00"+datum.Unit] = true
	}
	for _, datum := range generic {
		identity := datum.Source.Pointer + "\x00" + datum.Source.Literal + "\x00" + datum.Unit
		if seen[identity] {
			continue
		}
		seen[identity] = true
		datum.Key = fmt.Sprintf("reported_fact_%d", len(declared)+1)
		declared = append(declared, datum)
	}
	return declared
}
