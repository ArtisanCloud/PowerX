package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type toolFixture struct {
	service *ToolService
	run     *ToolRun
	ctx     context.Context
	policy  ToolRunPolicy
	tool    ToolDependency
	a, b    ResolvedDatum
}

func newToolFixture(t *testing.T, input map[string]any) *toolFixture {
	t.Helper()
	svc := NewToolService(ToolServiceOptions{})
	tool, err := svc.CalculatorDependency("calculator")
	require.NoError(t, err)
	fields := map[string]SourceField{
		"a":    {FieldKey: "a", LabelKey: "a.label", DatumKind: "quantity", Unit: "人", Scope: "cohort:current"},
		"b":    {FieldKey: "b", LabelKey: "b.label", DatumKind: "quantity", Unit: "人", Scope: "cohort:current"},
		"rate": {FieldKey: "rate", LabelKey: "rate.label", DatumKind: "reported", Unit: "%", Scope: "cohort:current"},
	}
	policy := ToolRunPolicy{Tools: ToolRequirements{Schema: ToolRequirementsSchema, Tools: []ToolDependency{tool}}, Fields: fields,
		Calculations: map[string]CalculationPlan{"ratio": {Expression: "n/d", OperandFields: map[string]string{"n": "a", "d": "b"}, Precision: 2, ResultUnit: "1"}}}
	id := ToolRunIdentity{TenantUUID: uuid.NewString(), RevisionUUID: uuid.NewString(), RunID: "run_" + uuid.NewString(), TraceID: "trace_" + uuid.NewString()}
	ctx := reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), id.TenantUUID), uuid.NewString())
	run, err := svc.NewRun(ctx, id, input, policy)
	require.NoError(t, err)
	return &toolFixture{service: svc, run: run, ctx: ctx, policy: policy, tool: tool}
}

func (f *toolFixture) resolve(t *testing.T, fieldKey, pointer, text, literal string, start int) ResolvedDatum {
	t.Helper()
	datum, err := f.run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: f.run.InputObjectUUID(), Pointer: pointer, Quote: text, Literal: literal, Range: ByteRange{StartByte: start, EndByte: start + len(text)}}, f.policy.Fields[fieldKey])
	require.NoError(t, err)
	return datum
}
func (f *toolFixture) call() ToolCall {
	request := CalculationRequest{Schema: CalculationRequestSchema, Expression: "n/d", Operands: map[string]ArithmeticOperand{
		"n": {DatumUUID: f.a.DatumUUID, Value: f.a.Value, Unit: f.a.Unit, Scope: f.a.Scope}, "d": {DatumUUID: f.b.DatumUUID, Value: f.b.Value, Unit: f.b.Unit, Scope: f.b.Scope},
	}, Precision: 2, ResultUnit: "1", Rounding: HalfAwayFromZero}
	raw, _ := json.Marshal(request)
	return ToolCall{Tool: f.tool, NodeID: "ratio", Attempt: 1, Inputs: []ResolvedDatum{f.a, f.b}, Request: raw}
}
func numericFixture(t *testing.T, denominator string) *toolFixture {
	t.Helper()
	f := newToolFixture(t, map[string]any{"a": json.Number("30"), "b": json.Number(denominator)})
	f.a = f.resolve(t, "a", "/a", "30", "30", 0)
	f.b = f.resolve(t, "b", "/b", denominator, denominator, 0)
	return f
}

func TestTypedToolExecutesActualCalculationAndSealsArtifacts(t *testing.T) {
	f := numericFixture(t, "12")
	require.Equal(t, "user_supplied_unverified", f.a.InputState)
	receipt, err := f.run.Execute(f.ctx, f.call())
	require.NoError(t, err)
	require.Equal(t, "succeeded", receipt.Status)
	require.Nil(t, receipt.Error)
	require.True(t, canonicalUUID(receipt.Authorization.DecisionUUID))
	require.NotEqual(t, reqctx.GetUserUUID(f.ctx), receipt.Authorization.SubjectUUID)
	require.Equal(t, "runtime_internal", receipt.Authorization.Actor)
	raw, err := f.run.ReadArtifact(f.ctx, *receipt.Result)
	require.NoError(t, err)
	var result CalculationResult
	require.NoError(t, DecodeToolJSON(raw, &result))
	require.Equal(t, CalculationResult{Schema: CalculationResultSchema, Value: "2.5", Unit: "1", Precision: 2, Rounding: HalfAwayFromZero}, result)
	require.NoError(t, f.run.VerifyReceipt(f.ctx, *receipt))
	require.NoError(t, f.run.VerifyResult(f.ctx, *receipt, "/value", "2.5"))
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/value", "3"), "evidence.tool_result_invalid")
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/value/extra", "2.5"), "evidence.tool_result_invalid")
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/precision", "2.5"), "evidence.tool_result_invalid")
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/precision", "2"), "evidence.tool_result_invalid")
	require.NoError(t, f.run.VerifyResultReference(f.ctx, *receipt, ToolResultReference{Pointer: "/value", Value: "2.5", Unit: "1", Scope: f.a.Scope}))
	require.ErrorContains(t, f.run.VerifyResultReference(f.ctx, *receipt, ToolResultReference{Pointer: "/value", Value: "2.5", Unit: "人", Scope: f.a.Scope}), "evidence.tool_result_invalid")
	require.ErrorContains(t, f.run.VerifyResultReference(f.ctx, *receipt, ToolResultReference{Pointer: "/value", Value: "2.5", Unit: "1", Scope: "different cohort"}), "evidence.tool_result_invalid")
	raw[0] = '!'
	require.NoError(t, f.run.VerifyResult(f.ctx, *receipt, "/value", "2.5"))
	// 纯计算结果保留原输入的未核实状态，不把原始人数升级为事实。
	require.Equal(t, "user_supplied_unverified", receipt.Inputs[0].InputState)
	location := SourceLocation{Kind: "tool_output", ObjectType: "powerx.tool_output", ObjectUUID: receipt.Result.ArtifactUUID, ExecutionUUID: receipt.ExecutionUUID, Pointer: "/value", Quote: "2.5", Literal: "2.5", Range: ByteRange{EndByte: 3}}
	field := SourceField{FieldKey: "value", LabelKey: "calculation.value", DatumKind: "quantity", Unit: "1", Scope: f.a.Scope, NumericString: true}
	datum, err := f.run.ResolveDatum(f.ctx, location, field)
	require.NoError(t, err)
	require.Equal(t, "tool_result", datum.InputState)
	require.NoError(t, f.run.VerifyDatum(f.ctx, datum))
	field.Unit = "人"
	_, err = f.run.ResolveDatum(f.ctx, location, field)
	require.ErrorContains(t, err, "evidence.source_invalid")
}

func TestTypedToolRejectsReceiptForgeryAndReplayEvenWithValidNumbers(t *testing.T) {
	f := numericFixture(t, "12")
	receipt, err := f.run.Execute(f.ctx, f.call())
	require.NoError(t, err)
	for _, mutate := range []func(*ToolReceipt){
		func(r *ToolReceipt) { r.ExecutionUUID = uuid.NewString() }, func(r *ToolReceipt) { r.TenantUUID = uuid.NewString() },
		func(r *ToolReceipt) { r.RevisionUUID = uuid.NewString() }, func(r *ToolReceipt) { r.RunID = "other" }, func(r *ToolReceipt) { r.TraceID = "other" },
		func(r *ToolReceipt) { r.Tool.Version = "2.0.1" }, func(r *ToolReceipt) { r.Tool.Binding.ImplementationSHA256 = toolDigest([]byte("forged")) },
		func(r *ToolReceipt) { r.Tool.InputSchema.SHA256 = toolDigest([]byte("forged")) }, func(r *ToolReceipt) { r.Authorization.DecisionUUID = uuid.NewString() },
		func(r *ToolReceipt) { r.Request.SHA256 = toolDigest([]byte("forged")) }, func(r *ToolReceipt) { r.Result.SHA256 = toolDigest([]byte("forged")) },
		func(r *ToolReceipt) { r.Inputs[0].Value = "300" }, func(r *ToolReceipt) { r.Inputs[0].InputState = "authorized_record" },
	} {
		raw, err := json.Marshal(receipt)
		require.NoError(t, err)
		var forged ToolReceipt
		require.NoError(t, DecodeToolJSON(raw, &forged))
		mutate(&forged)
		require.ErrorContains(t, f.run.VerifyResult(f.ctx, forged, "/value", "2.5"), "evidence.tool_receipt_untrusted")
	}
	// 相同上下文和身份也不能把历史/子 run 回执导入新的 ledger。
	other, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{}, f.policy)
	require.NoError(t, err)
	require.ErrorContains(t, other.VerifyReceipt(f.ctx, *receipt), "evidence.tool_receipt_untrusted")
	foreign := reqctx.WithTenantUUID(f.ctx, uuid.NewString())
	require.ErrorContains(t, f.run.VerifyReceipt(foreign, *receipt), "evidence.source_access_denied")
	sameTenantOtherUser := reqctx.WithUserUUID(f.ctx, uuid.NewString())
	_, err = f.run.ReadArtifact(sameTenantOtherUser, *receipt.Result)
	require.ErrorContains(t, err, "evidence.source_access_denied")
	// 存储损坏时即使数值仍正确也不能验真。
	f.run.mu.Lock()
	f.run.artifacts[receipt.Result.ArtifactUUID] = []byte(`{"value":"2.5"}`)
	f.run.mu.Unlock()
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/value", "2.5"), "evidence.tool_receipt_untrusted")
}

func TestTypedToolChecksPublishedPlanAndResolvedOperandsBeforeExecution(t *testing.T) {
	for name, mutate := range map[string]func(*ToolCall){
		"value": func(c *ToolCall) {
			c.Request = []byte(strings.Replace(string(c.Request), `"value":"30"`, `"value":"300"`, 1))
		},
		"quantity_ref":  func(c *ToolCall) { c.Inputs[0].DatumUUID = uuid.NewString() },
		"trust_upgrade": func(c *ToolCall) { c.Inputs[0].InputState = "authorized_record" },
		"expression":    func(c *ToolCall) { c.Request = []byte(strings.Replace(string(c.Request), "n/d", "n*d", 1)) },
		"constant":      func(c *ToolCall) { c.Request = []byte(strings.Replace(string(c.Request), "n/d", "n/100", 1)) },
		"precision": func(c *ToolCall) {
			c.Request = []byte(strings.Replace(string(c.Request), `"precision":2`, `"precision":3`, 1))
		},
		"unit": func(c *ToolCall) {
			c.Request = []byte(strings.Replace(string(c.Request), `"result_unit":"1"`, `"result_unit":"人"`, 1))
		},
		"node":               func(c *ToolCall) { c.NodeID = "undeclared" },
		"tool":               func(c *ToolCall) { c.Tool.Version = "2.0.1" },
		"duplicate_json":     func(c *ToolCall) { c.Request = append([]byte(`{"percent":false,`), c.Request[1:]...) },
		"unknown_field":      func(c *ToolCall) { c.Request = append([]byte(`{"trusted":true,`), c.Request[1:]...) },
		"missing_zero_field": func(c *ToolCall) { c.Request = []byte(strings.Replace(string(c.Request), `"percent":false,`, "", 1)) },
	} {
		t.Run(name, func(t *testing.T) {
			f := numericFixture(t, "12")
			call := f.call()
			mutate(&call)
			receipt, err := f.run.Execute(f.ctx, call)
			require.Error(t, err)
			require.Nil(t, receipt)
			receipts, err := f.run.Receipts(f.ctx)
			require.NoError(t, err)
			require.Empty(t, receipts)
			require.Zero(t, f.run.calls)
		})
	}
}

func TestTypedToolDivisionByZeroHasFailedActualExecution(t *testing.T) {
	f := numericFixture(t, "0")
	receipt, err := f.run.Execute(f.ctx, f.call())
	require.ErrorContains(t, err, "evidence.calculation_failed")
	require.Equal(t, "failed", receipt.Status)
	require.Nil(t, receipt.Result)
	require.Equal(t, "tool_invoke", receipt.Error.Stage)
	require.NoError(t, f.run.VerifyReceipt(f.ctx, *receipt))
	require.ErrorContains(t, f.run.VerifyResult(f.ctx, *receipt, "/value", "0"), "evidence.tool_receipt_untrusted")
	receipts, err := f.run.Receipts(f.ctx)
	require.NoError(t, err)
	require.Len(t, receipts, 1)
}

func TestTypedCalculatorPercentRoundingAndNegativeZeroUseActualToolResult(t *testing.T) {
	for _, test := range []struct {
		a, b, expression, unit, value string
		precision                     int
		percent                       bool
	}{
		{"1", "8", "n/d", "%", "12.5", 2, true}, {"1", "8", "n/d", "1", "0.13", 2, false},
		{"-1", "8", "n/d", "1", "-0.13", 2, false}, {"-1", "1000", "n/d", "1", "0", 2, false},
	} {
		f := newToolFixture(t, map[string]any{"a": json.Number(test.a), "b": json.Number(test.b)})
		policy := f.policy
		policy.Calculations = map[string]CalculationPlan{"ratio": {Expression: test.expression, OperandFields: map[string]string{"n": "a", "d": "b"}, Precision: test.precision, Percent: test.percent, ResultUnit: test.unit}}
		run, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{"a": json.Number(test.a), "b": json.Number(test.b)}, policy)
		require.NoError(t, err)
		f.run = run
		f.a = f.resolve(t, "a", "/a", test.a, test.a, 0)
		f.b = f.resolve(t, "b", "/b", test.b, test.b, 0)
		call := f.call()
		var request CalculationRequest
		require.NoError(t, DecodeToolJSON(call.Request, &request))
		request.Percent = test.percent
		request.ResultUnit = test.unit
		request.Precision = test.precision
		call.Request, _ = json.Marshal(request)
		receipt, err := run.Execute(f.ctx, call)
		require.NoError(t, err)
		require.NoError(t, run.VerifyResultReference(f.ctx, *receipt, ToolResultReference{Pointer: "/value", Value: test.value, Unit: test.unit, Scope: f.a.Scope}))
		logToolContractFixture(t, "receipt", receipt)
	}
}

func TestTypedCalculatorRejectsScopeAndUnitMixing(t *testing.T) {
	for _, which := range []string{"scope", "unit"} {
		f := numericFixture(t, "12")
		policy := f.policy
		policy.Fields = map[string]SourceField{}
		for key, field := range f.policy.Fields {
			policy.Fields[key] = field
		}
		field := policy.Fields["b"]
		if which == "scope" {
			field.Scope = "cohort:other"
		} else {
			field.Unit = "元"
		}
		policy.Fields["b"] = field
		run, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{"a": 30, "b": 12}, policy)
		require.NoError(t, err)
		f.run = run
		f.policy = policy
		f.a = f.resolve(t, "a", "/a", "30", "30", 0)
		f.b = f.resolve(t, "b", "/b", "12", "12", 0)
		receipt, err := run.Execute(f.ctx, f.call())
		require.ErrorContains(t, err, "evidence.calculation_plan_invalid")
		require.Nil(t, receipt)
		require.Zero(t, run.calls)
	}
}

func TestTypedToolConcurrentReceiptsHaveIsolatedActualBytes(t *testing.T) {
	f := numericFixture(t, "12")
	var wait sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, err := f.run.Execute(f.ctx, f.call())
			if err == nil {
				err = f.run.VerifyResult(f.ctx, *receipt, "/value", "2.5")
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	receipts, err := f.run.Receipts(f.ctx)
	require.NoError(t, err)
	require.Len(t, receipts, 8)
}

// 本地 HTTP 只是正式 adapter 的受控替身，不表示生产插件 pinning/授权已装配。
func TestCapabilityEvidenceRecordsActualHTTPOutputAndFailsClosed(t *testing.T) {
	for _, mode := range []string{"success", "invalid_output", "duplicate_output", "version_drift", "revoked", "timeout", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := numericFixture(t, "12")
			tool := f.tool
			tool.DependencyKey = "inventory"
			tool.ToolKey = "example.inventory.evaluate"
			tool.Version = "1.0.0"
			tool.Binding = ToolBinding{Kind: "capability", CapabilityUUID: uuid.NewString(), ContractUUID: uuid.NewString(), ProviderVersion: "0.4.0", ImplementationSHA256: toolDigest([]byte("fixture-only")), Protocol: "http"}
			tool.Permission = ToolPermission{Actor: "admin_user", Code: "inventory:read"}
			tool.InputSchema.ID, tool.OutputSchema.ID = "example.request/v1", "example.result/v1"
			var calls atomic.Int32
			var liveGrant atomic.Bool
			liveGrant.Store(true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "invalid_output" {
					fmt.Fprint(w, `{"value":"7.25","receipt":{"status":"succeeded"}}`)
				} else if mode == "duplicate_output" {
					fmt.Fprint(w, `{"value":"0","value":"7.25"}`)
				} else {
					fmt.Fprint(w, `{"value":"7.25"}`)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			svc := NewToolService(ToolServiceOptions{Capability: func(ctx context.Context, identity ToolRunIdentity, declared ToolDependency) (*BoundTool, error) {
				if declared != tool || identity.TenantUUID != f.run.identity.TenantUUID {
					return nil, toolError("skill.tool_dependency_denied", "permission_check", nil)
				}
				return &BoundTool{Binding: ToolInvocationBinding{Kind: "capability", RegistrationVersion: 1, AdapterID: "fixture-http", CatalogSHA256: toolDigest([]byte("fixture-catalog"))}, Authorization: ToolAuthorization{DecisionUUID: uuid.NewString(), SubjectUUID: reqctx.GetUserUUID(ctx), Actor: "admin_user"},
					ValidateRequest: func(_ context.Context, raw []byte, inputs []ResolvedDatum) error {
						if string(raw) != `{"operation":"evaluate"}` || len(inputs) != 0 {
							return errors.New("request.invalid")
						}
						return nil
					},
					Invoke: func(ctx context.Context, raw []byte) ([]byte, error) {
						if mode == "timeout" {
							calls.Add(1)
							return nil, context.DeadlineExceeded
						}
						if mode == "cancelled" {
							calls.Add(1)
							cancel()
							return nil, context.Canceled
						}
						req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(string(raw)))
						if err != nil {
							return nil, err
						}
						response, err := server.Client().Do(req)
						if err != nil {
							return nil, err
						}
						defer response.Body.Close()
						var rawResult json.RawMessage
						err = json.NewDecoder(response.Body).Decode(&rawResult)
						return rawResult, err
					},
					ValidateResult: func(_ context.Context, _, raw []byte) error {
						var result struct {
							Value string `json:"value"`
						}
						if DecodeToolJSON(raw, &result) != nil || result.Value != "7.25" {
							return toolError("evidence.tool_result_invalid", "result_validation", nil)
						}
						return nil
					},
					Recheck: func(context.Context) error {
						if (mode == "revoked" && calls.Load() > 0) || !liveGrant.Load() {
							return toolError("skill.tool_dependency_denied", "permission_check", nil)
						}
						if mode == "version_drift" && calls.Load() > 0 {
							return toolError("skill.tool_dependency_version_or_contract_mismatch", "dependency_check", nil)
						}
						return nil
					},
					OutputFields: func([]byte, []byte) map[string]SourceField {
						return map[string]SourceField{"/value": {FieldKey: "value", LabelKey: "inventory.value", DatumKind: "quantity", Unit: "件", Scope: "stock:current", NumericString: true}}
					},
				}, nil
			}})
			run, err := svc.NewRun(ctx, f.run.Identity(), map[string]any{}, ToolRunPolicy{Tools: ToolRequirements{Schema: ToolRequirementsSchema, Tools: []ToolDependency{tool}}})
			require.NoError(t, err)
			receipt, err := run.Execute(ctx, ToolCall{Tool: tool, NodeID: "inventory", Attempt: 1, Inputs: []ResolvedDatum{}, Request: []byte(`{"operation":"evaluate"}`)})
			require.EqualValues(t, 1, calls.Load())
			require.NotNil(t, receipt)
			logToolContractFixture(t, "receipt", receipt)
			if mode == "success" {
				require.NoError(t, err)
				require.NoError(t, run.VerifyResult(ctx, *receipt, "/value", "7.25"))
				ref := ToolResultReference{Pointer: "/value", Value: "7.25", Unit: "件", Scope: "stock:current"}
				require.NoError(t, run.VerifyResultReference(ctx, *receipt, ref))
				liveGrant.Store(false)
				require.NoError(t, run.VerifyReceipt(ctx, *receipt)) // 执行事实仍存在，但不授予新访问权。
				require.ErrorContains(t, run.VerifyResultReference(ctx, *receipt, ref), "skill.tool_dependency_denied")
				return
			}
			require.Error(t, err)
			require.Nil(t, receipt.Result)
			require.NotNil(t, receipt.Error)
			if mode == "timeout" {
				require.Equal(t, "timed_out", receipt.Status)
				require.Equal(t, "skill.tool_timeout", receipt.Error.Code)
			} else if mode == "cancelled" {
				require.Equal(t, "cancelled", receipt.Status)
				require.Equal(t, "skill.tool_cancelled", receipt.Error.Code)
			} else {
				require.Equal(t, "failed", receipt.Status)
				require.Equal(t, "result_validation", receipt.Error.Stage)
			}
		})
	}
}

func TestCapabilityBoundaryRejectsWrongActorAndIncompletePortBeforeInvocation(t *testing.T) {
	for _, mode := range []string{"wrong_user", "service_as_user", "missing_validator", "unregistered"} {
		f := numericFixture(t, "12")
		tool := f.tool
		tool.ToolKey = "example.stock.read"
		tool.Binding = ToolBinding{Kind: "capability", CapabilityUUID: uuid.NewString(), ContractUUID: uuid.NewString(), ProviderVersion: "1.0.0", ImplementationSHA256: toolDigest([]byte("fixture")), Protocol: "http"}
		tool.Permission = ToolPermission{Actor: "admin_user", Code: "stock:read"}
		if mode == "service_as_user" {
			tool.Permission.Actor = "service_actor"
		}
		calls := 0
		options := ToolServiceOptions{Capability: func(ctx context.Context, _ ToolRunIdentity, _ ToolDependency) (*BoundTool, error) {
			bound := &BoundTool{Binding: ToolInvocationBinding{Kind: "capability", RegistrationVersion: 1, AdapterID: "fixture", CatalogSHA256: toolDigest([]byte("fixture"))}, Authorization: ToolAuthorization{DecisionUUID: uuid.NewString(), SubjectUUID: reqctx.GetUserUUID(ctx), Actor: tool.Permission.Actor}, ValidateRequest: func(context.Context, []byte, []ResolvedDatum) error { return nil }, Invoke: func(context.Context, []byte) ([]byte, error) { calls++; return []byte(`{}`), nil }, ValidateResult: func(context.Context, []byte, []byte) error { return nil }, Recheck: func(context.Context) error { return nil }, OutputFields: func([]byte, []byte) map[string]SourceField { return nil }}
			if mode == "wrong_user" {
				bound.Authorization.SubjectUUID = uuid.NewString()
			}
			if mode == "missing_validator" {
				bound.ValidateRequest = nil
			}
			return bound, nil
		}}
		if mode == "unregistered" {
			options.Capability = nil
		}
		run, err := NewToolService(options).NewRun(f.ctx, f.run.Identity(), map[string]any{}, ToolRunPolicy{Tools: ToolRequirements{Schema: ToolRequirementsSchema, Tools: []ToolDependency{tool}}})
		require.NoError(t, err)
		receipt, err := run.Execute(f.ctx, ToolCall{Tool: tool, NodeID: "stock", Attempt: 1, Request: []byte(`{}`)})
		require.Error(t, err)
		require.Nil(t, receipt)
		require.Zero(t, calls)
	}
}

func TestToolContractEmbeddedBytesAndDTOShapeMatchFrozenContract(t *testing.T) {
	raw, err := os.ReadFile("../../../../../specs/024-ai-engineering-skills/contracts/tool-evidence.v1.schema.json")
	require.NoError(t, err)
	require.Equal(t, raw, ToolContractBytes())
	copy := ToolContractBytes()
	copy[0] = '!'
	require.Equal(t, raw, ToolContractBytes())
	// 每种新 wire DTO 的样本都能经过 strict decode；正式 Schema 外部校验另见验证脚本。
	f := numericFixture(t, "12")
	receipt, err := f.run.Execute(f.ctx, f.call())
	require.NoError(t, err)
	bytes, err := json.Marshal(receipt)
	require.NoError(t, err)
	var decoded ToolReceipt
	require.NoError(t, DecodeToolJSON(bytes, &decoded))
	require.Equal(t, *receipt, decoded)
	logToolContractFixture(t, "receipt", receipt)
	logToolContractFixture(t, "datum", f.a)
	requestRaw, err := f.run.ReadArtifact(f.ctx, receipt.Request)
	require.NoError(t, err)
	resultRaw, err := f.run.ReadArtifact(f.ctx, *receipt.Result)
	require.NoError(t, err)
	logToolContractFixture(t, "calculation_request", json.RawMessage(requestRaw))
	logToolContractFixture(t, "calculation_result", json.RawMessage(resultRaw))
	logToolContractFixture(t, "tool_requirements", f.policy.Tools)
	for _, raw := range []string{`{"schema":"powerx.calculation.result/v1","value":"2.5","unit":"1","precision":2,"rounding":"half_away_from_zero"}`, `{"schema":"powerx.calculation.result/v1","schema":"x","value":"2.5","unit":"1","precision":2,"percent":false,"rounding":"half_away_from_zero"}`} {
		var result CalculationResult
		require.Error(t, DecodeToolJSON([]byte(raw), &result))
	}
}

func TestV4ModelResponseRejectsDuplicateFieldsAndInvalidUTF8(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"schema":"powerx.agent.evidence-notes/v1","hypotheses":[],"hypotheses":["forged"],"gaps":[],"actions":[]}`),
		append([]byte(`{"schema":"powerx.agent.evidence-notes/v1","hypotheses":["`), append([]byte{0xff}, []byte(`"],"gaps":[],"actions":[]}`)...)...),
	} {
		var notes Notes
		require.ErrorContains(t, Decode(raw, &notes), "agent.response_contract_invalid")
	}
}

func logToolContractFixture(t *testing.T, definition string, value any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"definition": definition, "value": value})
	require.NoError(t, err)
	t.Logf("tool_contract_fixture %s", raw)
}
