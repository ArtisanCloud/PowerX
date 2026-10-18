package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSourceSnapshotRejectsInventedCountsAndPreservesPercentageOnly(t *testing.T) {
	text := "仅有复购率 27.8%，没有客户人数。"
	f := newToolFixture(t, map[string]any{"message": text})
	start := strings.Index(text, "27.8")
	datum := f.resolve(t, "rate", "/message", "27.8%", "27.8", start)
	require.Equal(t, "reported", datum.DatumKind)
	require.Equal(t, "extracted_unverified", datum.InputState)
	require.Equal(t, "27.8", datum.Value)
	for _, attempt := range []struct {
		quote, literal, unit, kind string
		end                        int
	}{
		{"27.8%", "278", "人", "quantity", start + len("27.8%")},
		{"27.8", "27.8", "人", "quantity", start + len("27.8")},
		{"27.8", "27.8", "1", "quantity", start + len("27.8")},
		{"27.8%", "27.8", "%", "quantity", start + len("27.8%")},
	} {
		field := f.policy.Fields["rate"]
		field.Unit = attempt.unit
		field.DatumKind = attempt.kind
		_, err := f.run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: f.run.InputObjectUUID(), Pointer: "/message", Quote: attempt.quote, Literal: attempt.literal, Range: ByteRange{StartByte: start, EndByte: attempt.end}}, field)
		require.ErrorContains(t, err, "evidence.source_invalid")
	}
	receipts, err := f.run.Receipts(f.ctx)
	require.NoError(t, err)
	require.Empty(t, receipts)
	require.Zero(t, f.run.calls)
	// 即使发布者错误地把 rate 声明成 quantity，原文百分号边界仍阻止改写人数。
	for _, unit := range []string{"1", "人", "%"} {
		field := f.policy.Fields["rate"]
		field.Unit = unit
		field.DatumKind = "quantity"
		policy := f.policy
		policy.Fields = map[string]SourceField{"rate": field}
		run, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{"message": text}, policy)
		require.NoError(t, err)
		quote := "27.8"
		if unit == "%" {
			quote += "%"
		}
		_, err = run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: "/message", Quote: quote, Literal: "27.8", Range: ByteRange{StartByte: start, EndByte: start + len(quote)}}, field)
		require.ErrorContains(t, err, "evidence.source_invalid")
	}
}

func TestSourceSnapshotIsImmutableAndRejectsForgedRefsRangesAndStates(t *testing.T) {
	input := map[string]any{"values": []any{map[string]any{"a/b~c": "客户 30人"}}}
	f := newToolFixture(t, input)
	datum := f.resolve(t, "a", "/values/0/a~1b~0c", "30人", "30", len("客户 "))
	require.Equal(t, "extracted_unverified", datum.InputState)
	input["values"].([]any)[0].(map[string]any)["a/b~c"] = "300人"
	require.NoError(t, f.run.VerifyDatum(f.ctx, datum))
	for _, mutate := range []func(*ResolvedDatum){
		func(d *ResolvedDatum) { d.DatumUUID = uuid.NewString() }, func(d *ResolvedDatum) { d.Value = "300" }, func(d *ResolvedDatum) { d.InputState = "authorized_record" },
		func(d *ResolvedDatum) { d.Source.ObjectUUID = uuid.NewString() }, func(d *ResolvedDatum) { d.Source.SnapshotSHA256 = toolDigest([]byte("other")) },
		func(d *ResolvedDatum) { d.Source.SourceUUID = uuid.NewString() }, func(d *ResolvedDatum) { d.Source.SnapshotUUID = uuid.NewString() }, func(d *ResolvedDatum) { d.Scope = "another cohort" },
	} {
		copy := datum
		mutate(&copy)
		require.Error(t, f.run.VerifyDatum(f.ctx, copy))
	}
	base := SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: f.run.InputObjectUUID(), Pointer: "/values/0/a~1b~0c", Quote: "30人", Literal: "30", Range: ByteRange{StartByte: len("客户 "), EndByte: len("客户 30人")}}
	for _, mutate := range []func(*SourceLocation){
		func(l *SourceLocation) { l.ObjectUUID = uuid.NewString() }, func(l *SourceLocation) { l.Pointer = "/values/00/a~1b~0c" }, func(l *SourceLocation) { l.Pointer = "/values/0/a~2b" },
		func(l *SourceLocation) { l.Range.StartByte = 1 }, func(l *SourceLocation) { l.Range.EndByte = l.Range.StartByte }, func(l *SourceLocation) { l.Quote = "300人" },
		func(l *SourceLocation) { l.ExecutionUUID = uuid.NewString() }, func(l *SourceLocation) { l.ObjectType = "file:///tmp/input" },
	} {
		copy := base
		mutate(&copy)
		_, err := f.run.ResolveDatum(f.ctx, copy, f.policy.Fields["a"])
		require.Error(t, err)
	}
	// 本 run 未声明的字段不接受模型临时改名。
	field := f.policy.Fields["a"]
	field.FieldKey = "unpublished"
	_, err := f.run.ResolveDatum(f.ctx, base, field)
	require.Error(t, err)
}

type authorizedFixtureSource struct {
	tenant, object, user string
	allow                bool
	field                SourceField
	json                 json.RawMessage
	calls                int
}

func (s *authorizedFixtureSource) ReadAuthorizedSource(ctx context.Context, in ObjectSourceRequest) (*AuthorizedSourceSnapshot, error) {
	s.calls++
	// fixture 明确模拟对象及字段授权，业务 reader 必须使用其正式仓储/授权服务。
	if !s.allow || in.TenantUUID != s.tenant || in.ObjectUUID != s.object || reqctx.GetUserUUID(ctx) != s.user || in.Pointer != "/customers" {
		return nil, errors.New("private DB/object details")
	}
	return &AuthorizedSourceSnapshot{TenantUUID: s.tenant, ObjectType: in.ObjectType, ObjectUUID: s.object, Values: append([]byte(nil), s.json...), Fields: map[string]SourceField{"/customers": s.field}}, nil
}

func TestAuthorizedSourcesReauthorizeTenantObjectAndFieldWithoutTrustUpgrade(t *testing.T) {
	f := numericFixture(t, "12")
	reader := &authorizedFixtureSource{tenant: f.run.identity.TenantUUID, object: uuid.NewString(), user: reqctx.GetUserUUID(f.ctx), allow: true, field: f.policy.Fields["a"], json: []byte(`{"customers":30}`)}
	svc := NewToolService(ToolServiceOptions{Objects: map[string]AuthorizedObjectSource{"example.customer_snapshot": reader}})
	policy := f.policy
	tool, err := svc.CalculatorDependency("calculator")
	require.NoError(t, err)
	policy.Tools.Tools = []ToolDependency{tool}
	run, err := svc.NewRun(f.ctx, f.run.Identity(), map[string]any{"b": 12}, policy)
	require.NoError(t, err)
	location := SourceLocation{Kind: "authorized_object", ObjectType: "example.customer_snapshot", ObjectUUID: reader.object, Pointer: "/customers", Quote: "30", Literal: "30", Range: ByteRange{EndByte: 2}}
	datum, err := run.ResolveDatum(f.ctx, location, reader.field)
	require.NoError(t, err)
	require.Equal(t, "authorized_record", datum.InputState)
	reader.json = []byte(`{"customers":300}`)
	stillFrozen, err := run.ResolveDatum(f.ctx, location, reader.field)
	require.NoError(t, err)
	require.Equal(t, "30", stillFrozen.Value)
	require.Equal(t, datum.Source.SnapshotSHA256, stillFrozen.Source.SnapshotSHA256)
	require.NoError(t, run.VerifyDatum(f.ctx, datum))
	require.GreaterOrEqual(t, reader.calls, 3)
	reader.allow = false
	require.Equal(t, "evidence.source_access_denied", run.VerifyDatum(f.ctx, datum).Error())
	_, denied := run.ResolveDatum(f.ctx, location, reader.field)
	require.Equal(t, "evidence.source_access_denied", denied.Error())
	reader.allow = true
	unknown := location
	unknown.ObjectUUID = uuid.NewString()
	_, missing := run.ResolveDatum(f.ctx, unknown, reader.field)
	require.Equal(t, denied.Error(), missing.Error())
	wrongField := location
	wrongField.Pointer = "/secret"
	_, err = run.ResolveDatum(f.ctx, wrongField, reader.field)
	require.Equal(t, denied.Error(), err.Error())
	reader.tenant = uuid.NewString()
	_, err = run.ResolveDatum(f.ctx, location, reader.field)
	require.Equal(t, denied.Error(), err.Error())
	reader.tenant = f.run.identity.TenantUUID
	_, err = run.ResolveDatum(reqctx.WithTenantUUID(f.ctx, uuid.NewString()), location, reader.field)
	require.Equal(t, denied.Error(), err.Error())
	_, err = run.ResolveDatum(context.Background(), location, reader.field)
	require.Equal(t, denied.Error(), err.Error())
	// 未登记的类型即使声称是授权对象也拒绝。
	location.ObjectType = "https://example.invalid/source"
	_, err = run.ResolveDatum(f.ctx, location, reader.field)
	require.Equal(t, denied.Error(), err.Error())
}

func TestToolOutputSourceCannotOutliveOriginalSourcePermission(t *testing.T) {
	f := numericFixture(t, "12")
	reader := &authorizedFixtureSource{tenant: f.run.identity.TenantUUID, object: uuid.NewString(), user: reqctx.GetUserUUID(f.ctx), allow: true, field: f.policy.Fields["a"], json: []byte(`{"customers":30}`)}
	svc := NewToolService(ToolServiceOptions{Objects: map[string]AuthorizedObjectSource{"example.customer_snapshot": reader}})
	run, err := svc.NewRun(f.ctx, f.run.Identity(), map[string]any{"b": 12}, f.policy)
	require.NoError(t, err)
	a, err := run.ResolveDatum(f.ctx, SourceLocation{Kind: "authorized_object", ObjectType: "example.customer_snapshot", ObjectUUID: reader.object, Pointer: "/customers", Quote: "30", Literal: "30", Range: ByteRange{EndByte: 2}}, reader.field)
	require.NoError(t, err)
	b, err := run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: "/b", Quote: "12", Literal: "12", Range: ByteRange{EndByte: 2}}, f.policy.Fields["b"])
	require.NoError(t, err)
	f.a, f.b = a, b
	receipt, err := run.Execute(f.ctx, f.call())
	require.NoError(t, err)
	location := SourceLocation{Kind: "tool_output", ObjectType: "powerx.tool_output", ObjectUUID: receipt.Result.ArtifactUUID, ExecutionUUID: receipt.ExecutionUUID, Pointer: "/value", Quote: "2.5", Literal: "2.5", Range: ByteRange{EndByte: 3}}
	datum, err := run.ResolveDatum(f.ctx, location, SourceField{FieldKey: "value", LabelKey: "calculation.value", DatumKind: "quantity", Unit: "1", Scope: a.Scope, NumericString: true})
	require.NoError(t, err)
	reader.allow = false
	require.ErrorContains(t, run.VerifyDatum(f.ctx, datum), "evidence.source_access_denied")
	_, err = run.ResolveDatum(f.ctx, location, SourceField{FieldKey: "value", LabelKey: "calculation.value", DatumKind: "quantity", Unit: "1", Scope: a.Scope, NumericString: true})
	require.ErrorContains(t, err, "evidence.source_access_denied")
}

func TestNumericStringObjectTrustIsExplicitAndExtractionStaysUnverified(t *testing.T) {
	for _, numericString := range []bool{false, true} {
		f := numericFixture(t, "12")
		field := f.policy.Fields["a"]
		field.NumericString = numericString
		text := `{"customers":"30人"}`
		quote := "30人"
		if numericString {
			text = `{"customers":"30"}`
			quote = "30"
		}
		reader := &authorizedFixtureSource{tenant: f.run.identity.TenantUUID, object: uuid.NewString(), user: reqctx.GetUserUUID(f.ctx), allow: true, field: field, json: []byte(text)}
		svc := NewToolService(ToolServiceOptions{Objects: map[string]AuthorizedObjectSource{"example.customer_snapshot": reader}})
		run, err := svc.NewRun(f.ctx, f.run.Identity(), map[string]any{}, f.policy)
		require.NoError(t, err)
		datum, err := run.ResolveDatum(f.ctx, SourceLocation{Kind: "authorized_object", ObjectType: "example.customer_snapshot", ObjectUUID: reader.object, Pointer: "/customers", Quote: quote, Literal: "30", Range: ByteRange{EndByte: len(quote)}}, field)
		require.NoError(t, err)
		state := "extracted_unverified"
		if numericString {
			state = "authorized_record"
		}
		require.Equal(t, state, datum.InputState)
	}
}

func TestSourceLiteralNormalizationAndUTF8AreValidatedBeforeFreeze(t *testing.T) {
	text := "+001,234.50人"
	f := newToolFixture(t, map[string]any{"message": text})
	datum := f.resolve(t, "a", "/message", text, "+001,234.50", 0)
	require.Equal(t, "1234.50", datum.Value)
	require.Equal(t, "+001,234.50", datum.Literal)
	logToolContractFixture(t, "datum", datum)
	_, err := FreezeEvidenceInput(map[string]any{"message": string([]byte{0xff})})
	require.ErrorContains(t, err, "evidence.source_invalid")
	_, err = FreezeEvidenceInput(map[string]any{string([]byte{0xff}): "30人"})
	require.ErrorContains(t, err, "evidence.source_invalid")
	key := snapshotKey(datum.Source.Kind, datum.Source.ObjectType, datum.Source.ObjectUUID)
	f.run.mu.Lock()
	snapshot := f.run.snapshots[key]
	snapshot.raw = []byte(`{"message":"forged"}`)
	f.run.snapshots[key] = snapshot
	f.run.mu.Unlock()
	require.ErrorContains(t, f.run.VerifyDatum(f.ctx, datum), "evidence.source_invalid")
}

func TestSourceUnitsShareTokenizerBoundaryWithoutClipping(t *testing.T) {
	for _, test := range []struct {
		text, literal, quote, unit string
		valid                      bool
	}{
		{"客单价6000元到2.4万元之间", "6000", "6000元", "元", true},
		{"90天内的结果", "90", "90天", "天", true},
		{"29.0万元的增量", "29.0", "29.0万元", "万元", true},
		{"6个月内", "6", "6个", "个", false},
		{"12cms", "12", "12cm", "cm", false},
		{"12cm3", "12", "12cm", "cm", false},
		{"27.8%", "27.8", "27.8", "1", false},
		{"27.8 %", "27.8", "27.8", "1", false},
	} {
		t.Run(test.text, func(t *testing.T) {
			f := newToolFixture(t, map[string]any{"message": test.text})
			field := f.policy.Fields["a"]
			field.Unit = test.unit
			policy := f.policy
			policy.Fields = map[string]SourceField{"a": field}
			run, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{"message": test.text}, policy)
			require.NoError(t, err)
			start := strings.Index(test.text, test.literal)
			_, err = run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: "/message", Quote: test.quote, Literal: test.literal, Range: ByteRange{StartByte: start, EndByte: start + len(test.quote)}}, field)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "evidence.source_invalid")
			}
		})
	}
}
