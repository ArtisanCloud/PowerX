package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
)

// executeCapabilityV5 只执行单个固定的 typed 工具；不在 Core 识别领域算法或工具 key。
func (e *ManifestExecutor) executeCapabilityV5(ctx context.Context, in ExecuteInput) (_ map[string]any, executeErr error) {
	input, err := evidence.FreezeEvidenceInput(in.Payload)
	if err != nil {
		return nil, err
	}
	in.Payload = input
	var requirements evidence.ToolRequirements
	raw, err := json.Marshal(in.Manifest["tool_dependencies"])
	if err != nil || evidence.DecodeToolJSON(raw, &requirements) != nil || len(requirements.Tools) != 1 {
		return nil, fmt.Errorf("skill.tool_dependency_invalid")
	}
	tool := requirements.Tools[0]
	runID, _ := evidence.InvocationScope(ctx)
	if runID == "" {
		runID = uuid.NewString()
	}
	id := evidence.ToolRunIdentity{TenantUUID: in.TenantUUID, RevisionUUID: in.Version, RunID: runID, TraceID: in.TraceID}
	description, err := e.tools.DescribeTool(ctx, id, tool)
	if err != nil {
		return nil, err
	}
	if !canonicalDependencyUUID(description.AgentUUID) || len(description.InputFields) == 0 || len(description.ResultFields) == 0 {
		return nil, fmt.Errorf("skill.tool_contract_unsupported")
	}
	source, err := evidence.TrustedToolInputValues(ctx)
	if err != nil {
		return nil, err
	}
	selected := map[string]any{}
	for pointer := range description.InputFields {
		key := pointer[1:]
		value, ok := source[key]
		if !ok {
			return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.source_invalid", Stage: "source_resolution"}}
		}
		if candidate, present := in.Payload[key]; present {
			a, _ := json.Marshal(candidate)
			b, _ := json.Marshal(value)
			if string(a) != string(b) {
				return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.source_invalid", Stage: "source_resolution"}}
			}
		}
		selected[key] = value
	}
	in.Payload = selected
	id.AgentUUID = description.AgentUUID
	policy := evidence.ToolRunPolicy{Tools: requirements, Fields: map[string]evidence.SourceField{}, Calculations: map[string]evidence.CalculationPlan{}}
	for _, field := range description.InputFields {
		policy.Fields[field.FieldKey] = field
	}
	run, err := e.tools.NewRun(ctx, id, in.Payload, policy)
	if err != nil {
		return nil, err
	}
	state := &v5Execution{run: run, labels: evidence.ResponseLabels{Schema: "powerx.response-labels/v1", RevisionUUID: in.Version, Locales: description.Labels}, report: evidence.ReportV5{Schema: evidence.ReportSchemaV5, Kind: "tool_result", TenantUUID: in.TenantUUID, RevisionUUID: in.Version, RunID: runID, TraceID: in.TraceID, Receipts: []evidence.ToolReceipt{}, Presentation: evidence.PresentationV5{Reported: []evidence.ResolvedDatum{}, Computed: []evidence.ComputedDatum{}, Conflicts: []evidence.EvidenceConflict{}, Gaps: []evidence.EvidenceGap{}, Hypotheses: []string{}, Actions: []string{}}}}
	defer func() {
		if executeErr != nil {
			e.saveV5Failure(ctx, state, executeErr)
		}
	}()
	for _, pointer := range sortedToolPointers(description.InputFields) {
		// 支持集合已由固定合同限定为闭合顶层对象；不是自由 Pointer/URL resolver。
		value := in.Payload[pointer[1:]]
		number, ok := value.(json.Number)
		if !ok {
			return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.source_invalid", Stage: "input_validation"}}
		}
		literal := number.String()
		datum, err := run.ResolveDatum(ctx, evidence.SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: pointer, Quote: literal, Literal: literal, Range: evidence.ByteRange{EndByte: len(literal)}}, description.InputFields[pointer])
		if err != nil {
			return nil, err
		}
		state.report.Presentation.Reported = append(state.report.Presentation.Reported, datum)
	}
	raw, err = json.Marshal(in.Payload)
	if err != nil {
		return nil, err
	}
	receipt, err := run.Execute(ctx, evidence.ToolCall{Tool: tool, NodeID: tool.DependencyKey, Attempt: 1, Inputs: state.report.Presentation.Reported, Request: raw})
	if err != nil {
		return nil, err
	}
	result, err := run.ReadArtifact(ctx, *receipt.Result)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if evidence.DecodeToolJSON(result, &values) != nil {
		return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.tool_result_invalid", Stage: "result_validation"}}
	}
	state.report.Receipts = []evidence.ToolReceipt{*receipt}
	for _, pointer := range sortedToolPointers(description.ResultFields) {
		field := description.ResultFields[pointer]
		state.report.Presentation.Computed = append(state.report.Presentation.Computed, evidence.ComputedDatum{DatumUUID: uuid.NewString(), FieldKey: field.FieldKey, LabelKey: field.LabelKey, ReceiptUUID: receipt.ExecutionUUID, OutputPointer: pointer, Value: values[pointer[1:]], Unit: field.Unit, Scope: field.Scope, Derivation: evidence.Derivation{Algorithm: &evidence.AlgorithmDerivation{Kind: "domain_algorithm", AlgorithmID: tool.ToolKey, ParametersSHA256: receipt.Request.SHA256}}})
	}
	return e.finishV5(ctx, state, evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
}
