package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
)

type v5Execution struct {
	run     *evidence.ToolRun
	report  evidence.ReportV5
	labels  evidence.ResponseLabels
	missing []evidence.MissingOperands
}

// prepareV5 不经旧计算器生成中间 V4；数值只来自本次 typed 执行和私有 ledger。
func (e *ManifestExecutor) prepareV5(ctx context.Context, in ExecuteInput, policy evidence.CalculationPolicy, plan evidence.Plan, missing []evidence.MissingOperands, locale string) (_ *v5Execution, executeErr error) {
	if e.evidenceStore == nil {
		return nil, fmt.Errorf("evidence.storage_unavailable")
	}
	if err := e.evidenceStore.Ready(ctx); err != nil {
		return nil, err
	}
	var requirements evidence.ToolRequirements
	raw, err := json.Marshal(in.Manifest["tool_dependencies"])
	if err != nil {
		return nil, err
	}
	if err := evidence.DecodeToolJSON(raw, &requirements); err != nil {
		return nil, err
	}
	runID, _ := evidence.InvocationScope(ctx)
	if runID == "" {
		runID = uuid.NewString()
	}
	labels := evidence.ResponseLabels{Schema: "powerx.response-labels/v1", RevisionUUID: in.Version, Locales: map[string]map[string]string{}}
	addLabels := func(key string, translations map[string]string) {
		for loc, label := range translations {
			if labels.Locales[loc] == nil {
				labels.Locales[loc] = map[string]string{}
			}
			labels.Locales[loc][key] = label
		}
	}
	for _, f := range policy.InputFields {
		addLabels("skill.fields."+f.Key, f.LabelI18n)
	}
	for _, f := range policy.Formulas {
		addLabels("skill.formulas."+f.Key, f.LabelI18n)
	}
	compiled := evidence.ToolRunPolicy{Tools: requirements, Fields: map[string]evidence.SourceField{}, Calculations: map[string]evidence.CalculationPlan{}}
	declared := map[string]evidence.InputField{}
	for _, field := range policy.InputFields {
		declared[field.Key] = field
	}
	formulas := map[string]evidence.Formula{}
	for _, formula := range policy.Formulas {
		formulas[formula.Key] = formula
		compiled.Calculations[formula.Key] = evidence.CalculationPlan{Expression: formula.Expression, OperandFields: formula.Bindings, Precision: formula.Precision, Percent: formula.Percent, ResultUnit: formula.ResultUnit}
	}
	for i := range plan.Data {
		datum := &plan.Data[i]
		unit := datum.Unit
		if unit == "" {
			unit = "1"
		}
		label := "skill.fields." + datum.Key
		if field, ok := declared[datum.Key]; ok {
			if field.ScopeI18n[locale] == "" || field.ScopeI18n[locale] != datum.Scope {
				return nil, fmt.Errorf("evidence.source_invalid")
			}
		} else {
			datum.Kind = "reported"
			datum.Scope = "run_input"
			label = "agent.response.reported"
		}
		compiled.Fields[datum.Key] = evidence.SourceField{FieldKey: datum.Key, LabelKey: label, DatumKind: datum.Kind, Unit: unit, Scope: datum.Scope}
	}
	run, err := e.tools.NewRun(ctx, evidence.ToolRunIdentity{TenantUUID: in.TenantUUID, RevisionUUID: in.Version, RunID: runID, TraceID: in.TraceID}, in.Payload, compiled)
	if err != nil {
		return nil, err
	}
	state := &v5Execution{run: run, labels: labels, missing: append([]evidence.MissingOperands{}, missing...), report: evidence.ReportV5{Schema: evidence.ReportSchemaV5, Kind: plan.Kind, TenantUUID: in.TenantUUID, RevisionUUID: in.Version, RunID: runID, TraceID: in.TraceID, Receipts: []evidence.ToolReceipt{}, Presentation: evidence.PresentationV5{Reported: []evidence.ResolvedDatum{}, Computed: []evidence.ComputedDatum{}, Conflicts: []evidence.EvidenceConflict{}, Gaps: []evidence.EvidenceGap{}, Hypotheses: []string{}, Actions: []string{}}}}
	// 即使工具或最终校验失败，也保存实际失败调用；不会生成成功报告或重试。
	success := false
	defer func() {
		if !success {
			e.saveV5Failure(ctx, state, executeErr)
		}
	}()
	byField := map[string]evidence.ResolvedDatum{}
	for _, datum := range plan.Data {
		text, err := jsonSourceText(in.Payload, datum.Source.Pointer)
		if err != nil {
			return nil, err
		}
		start := strings.Index(text, datum.Source.Quote)
		if start < 0 {
			return nil, fmt.Errorf("evidence.source_invalid")
		}
		resolved, err := run.ResolveDatum(ctx, evidence.SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: datum.Source.Pointer, Quote: datum.Source.Quote, Literal: datum.Source.Literal, Range: evidence.ByteRange{StartByte: start, EndByte: start + len(datum.Source.Quote)}}, compiled.Fields[datum.Key])
		if err != nil {
			return nil, err
		}
		state.report.Presentation.Reported = append(state.report.Presentation.Reported, resolved)
		byField[datum.Key] = resolved
	}
	for _, gap := range missing {
		formula := formulas[gap.CalculationKey]
		fields := []string{}
		for _, field := range policy.InputFields {
			for _, key := range formula.Bindings {
				if key == field.Key {
					if _, ok := byField[key]; !ok {
						fields = append(fields, key)
					}
					break
				}
			}
		}
		state.report.Presentation.Gaps = append(state.report.Presentation.Gaps, evidence.EvidenceGap{Key: gap.CalculationKey, LabelKey: "skill.formulas." + gap.CalculationKey, MissingFields: fields})
	}
	for _, calculation := range plan.Calculations {
		formula := formulas[calculation.Key]
		request := evidence.CalculationRequest{Schema: evidence.CalculationRequestSchema, Expression: formula.Expression, Operands: map[string]evidence.ArithmeticOperand{}, Precision: formula.Precision, Percent: formula.Percent, Rounding: evidence.HalfAwayFromZero, ResultUnit: formula.ResultUnit}
		inputs := []evidence.ResolvedDatum{}
		seen := map[string]bool{}
		bindings := map[string]string{}
		scope, unit := "", ""
		incompatible := false
		for variable, field := range formula.Bindings {
			d := byField[field]
			if scope != "" && (scope != d.Scope || unit != d.Unit) {
				incompatible = true
			}
			scope, unit = d.Scope, d.Unit
			request.Operands[variable] = evidence.ArithmeticOperand{DatumUUID: d.DatumUUID, Value: d.Value, Unit: d.Unit, Scope: d.Scope}
			bindings[variable] = d.DatumUUID
			if !seen[d.DatumUUID] {
				inputs = append(inputs, d)
				seen[d.DatumUUID] = true
			}
		}
		if incompatible {
			fields := []string{}
			for _, field := range policy.InputFields {
				for _, key := range formula.Bindings {
					if key == field.Key {
						fields = append(fields, key)
						break
					}
				}
			}
			state.report.Presentation.Gaps = append(state.report.Presentation.Gaps, evidence.EvidenceGap{Key: formula.Key, LabelKey: "skill.formulas." + formula.Key, MissingFields: fields})
			inputLabels := []string{}
			for _, key := range fields {
				inputLabels = append(inputLabels, declared[key].LabelI18n[locale])
			}
			state.missing = append(state.missing, evidence.MissingOperands{CalculationKey: formula.Key, Label: formula.LabelI18n[locale], InputLabels: inputLabels})
			continue
		}
		raw, _ := json.Marshal(request)
		var tool evidence.ToolDependency
		for _, dep := range requirements.Tools {
			if dep.ToolKey == evidence.CalculatorKey {
				tool = dep
				break
			}
		}
		receipt, err := run.Execute(ctx, evidence.ToolCall{Tool: tool, NodeID: formula.Key, Attempt: 1, Inputs: inputs, Request: raw})
		if err != nil {
			return nil, err
		}
		resultRaw, err := run.ReadArtifact(ctx, *receipt.Result)
		if err != nil {
			return nil, err
		}
		var result evidence.CalculationResult
		if err := evidence.DecodeToolJSON(resultRaw, &result); err != nil {
			return nil, err
		}
		computed := evidence.ComputedDatum{DatumUUID: uuid.NewString(), FieldKey: formula.Key, LabelKey: "skill.formulas." + formula.Key, ReceiptUUID: receipt.ExecutionUUID, OutputPointer: "/value", Value: result.Value, Unit: result.Unit, Scope: scope, Derivation: evidence.Derivation{Expression: &evidence.ExpressionDerivation{Kind: "expression", Expression: request.Expression, OperandBindings: bindings, Precision: request.Precision, Percent: request.Percent, Rounding: request.Rounding}}}
		state.report.Presentation.Computed = append(state.report.Presentation.Computed, computed)
		if original, ok := byField[calculation.CompareTo]; ok {
			reason := evidence.CompareEvidenceValues(original, computed)
			if reason != "" {
				state.report.Presentation.Conflicts = append(state.report.Presentation.Conflicts, evidence.EvidenceConflict{ConflictUUID: uuid.NewString(), ReportedDatumUUID: original.DatumUUID, ComputedDatumUUID: computed.DatumUUID, Reason: reason})
			}
		}
	}
	state.report.Receipts, err = run.Receipts(ctx)
	if err != nil {
		return nil, err
	}
	if len(state.report.Presentation.Reported)+len(state.report.Presentation.Computed)+len(state.report.Presentation.Gaps) == 0 {
		state.report.Presentation.Gaps = append(state.report.Presentation.Gaps, evidence.EvidenceGap{Key: "source_required", LabelKey: "agent.response.sourceRequired", MissingFields: []string{"source"}})
	}
	success = true
	return state, nil
}

func jsonSourceText(payload map[string]any, pointer string) (string, error) {
	var value any = payload
	for _, key := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		object, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("evidence.source_invalid")
		}
		value = object[strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")]
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("evidence.source_invalid")
	}
	return text, nil
}

func (e *ManifestExecutor) finishV5(ctx context.Context, state *v5Execution, notes evidence.Notes) (map[string]any, error) {
	state.report.Presentation.Hypotheses = notes.Hypotheses
	state.report.Presentation.Actions = notes.Actions
	report, err := state.run.SealReportV5(ctx, state.report)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(report)
	var typed evidence.ReportV5
	if err := evidence.DecodeToolJSON(raw, &typed); err != nil {
		return nil, err
	}
	archive, err := state.run.ExportArchive(ctx, &typed, state.labels)
	if err != nil {
		return nil, err
	}
	ref, err := e.evidenceStore.Save(ctx, archive)
	if err != nil {
		return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.storage_unavailable", Stage: "evidence_validation"}, Cause: err}
	}
	return map[string]any{"response_envelope": report, "response_labels": state.labels, "response_evidence": ref}, nil
}

func (e *ManifestExecutor) saveV5Failure(ctx context.Context, state *v5Execution, err error) {
	failure, ok := evidence.FailureDetails(err)
	if !ok {
		failure = evidence.ToolFailure{Code: "agent.response_contract_invalid", Stage: "evidence_validation"}
	}
	archive, exportErr := state.run.ExportFailureArchive(context.WithoutCancel(ctx), failure, state.labels)
	if exportErr == nil {
		_, _ = e.evidenceStore.Save(context.WithoutCancel(ctx), archive)
	}
}
