package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const ArchiveSchema = "powerx.run-evidence/v1"

type EvidenceReference struct {
	Schema       string `json:"schema"`
	SnapshotUUID string `json:"snapshot_uuid"`
	SHA256       string `json:"sha256"`
}
type ArchivedSource struct {
	Kind         string `json:"kind"`
	ObjectType   string `json:"object_type"`
	ObjectUUID   string `json:"object_uuid"`
	SnapshotUUID string `json:"snapshot_uuid"`
	SHA256       string `json:"sha256"`
	JSON         []byte `json:"json"` // base64 保存精确原件，避免 JSON encoder 压缩/转义改变摘要。
}
type ArchivedArtifact struct {
	Reference ToolArtifact `json:"reference"`
	JSON      []byte       `json:"json"`
}
type RunEvidenceArchive struct {
	origin       *ToolRun
	exportDigest string
	Schema       string             `json:"schema"`
	AgentUUID    string             `json:"agent_uuid,omitempty"`
	TenantUUID   string             `json:"tenant_uuid"`
	RevisionUUID string             `json:"revision_uuid"`
	RunID        string             `json:"run_id"`
	TraceID      string             `json:"trace_id"`
	Sources      []ArchivedSource   `json:"sources"`
	Artifacts    []ArchivedArtifact `json:"artifacts"`
	Receipts     []ToolReceipt      `json:"receipts"`
	Failure      *ToolFailure       `json:"failure,omitempty"`
	Report       *ReportV5          `json:"report,omitempty"`
	Labels       ResponseLabels     `json:"labels"`
}

// EvidenceArchiveStore 只保存 Core 封存的原件；读取不能导入当前运行或重新计算。
type EvidenceArchiveStore interface {
	Ready(context.Context) error
	Save(context.Context, RunEvidenceArchive) (EvidenceReference, error)
	Read(context.Context, EvidenceReference) (RunEvidenceArchive, error)
}

func (r *ToolRun) ExportArchive(ctx context.Context, report *ReportV5, labels ResponseLabels) (RunEvidenceArchive, error) {
	if err := r.checkContext(ctx); err != nil {
		return RunEvidenceArchive{}, err
	}
	if report != nil {
		normalized, err := ValidateReportV5(report)
		if err != nil {
			return RunEvidenceArchive{}, err
		}
		if err := Verify(ctx, normalized); err != nil {
			return RunEvidenceArchive{}, toolError("evidence.tool_receipt_untrusted", "evidence_validation", err)
		}
	}
	receipts, err := r.Receipts(ctx)
	if err != nil {
		return RunEvidenceArchive{}, err
	}
	archive := RunEvidenceArchive{Schema: ArchiveSchema, AgentUUID: r.identity.AgentUUID, TenantUUID: r.identity.TenantUUID, RevisionUUID: r.identity.RevisionUUID, RunID: r.identity.RunID, TraceID: r.identity.TraceID, Sources: []ArchivedSource{}, Artifacts: []ArchivedArtifact{}, Receipts: receipts, Report: report, Labels: labels}
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := []string{}
	for key := range r.snapshots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s := r.snapshots[key]
		parts := strings.Split(key, "\x00")
		archive.Sources = append(archive.Sources, ArchivedSource{Kind: parts[0], ObjectType: parts[1], ObjectUUID: parts[2], SnapshotUUID: s.uuid, SHA256: s.digest, JSON: append([]byte(nil), s.raw...)})
	}
	keys = nil
	for key := range r.artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := r.artifacts[key]
		archive.Artifacts = append(archive.Artifacts, ArchivedArtifact{Reference: ToolArtifact{ArtifactUUID: key, SHA256: toolDigest(raw)}, JSON: append([]byte(nil), raw...)})
	}
	archive.origin = r
	raw, err := json.Marshal(archive)
	if err != nil {
		return RunEvidenceArchive{}, err
	}
	archive.exportDigest = toolDigest(raw)
	return archive, nil
}

// ExportFailureArchive 仅保留实际调用和稳定失败，不生成成功业务报告。
func (r *ToolRun) ExportFailureArchive(ctx context.Context, failure ToolFailure, labels ResponseLabels) (RunEvidenceArchive, error) {
	if !boundedText(failure.Code, 512) || !validFailureStage(failure.Stage) {
		return RunEvidenceArchive{}, fmt.Errorf("evidence.archive_invalid")
	}
	a, err := r.ExportArchive(ctx, nil, labels)
	if err != nil {
		return a, err
	}
	a.Failure = &failure
	raw, err := json.Marshal(a)
	if err != nil {
		return RunEvidenceArchive{}, err
	}
	a.exportDigest = toolDigest(raw)
	return a, nil
}

// VerifyOrigin 只允许同次 Core 导出的原件入库；历史 JSON 无法恢复该进程内权威。
func (a RunEvidenceArchive) VerifyOrigin(ctx context.Context) error {
	if a.origin == nil {
		return toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	if err := a.origin.checkContext(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(a)
	if err != nil || toolDigest(raw) != a.exportDigest {
		return toolError("evidence.tool_receipt_untrusted", "evidence_validation", err)
	}
	return nil
}

// ValidateArchive 只检查原始字节、关联与完整性；没有工具执行和对象重新读取。
func ValidateArchive(a RunEvidenceArchive) error {
	fail := func() error { return fmt.Errorf("evidence.archive_invalid") }
	if a.Schema != ArchiveSchema || !canonicalUUID(a.TenantUUID) || !canonicalUUID(a.RevisionUUID) || !boundedText(a.RunID, 512) || !boundedText(a.TraceID, 512) || len(a.Receipts) > 64 || len(a.Sources) > 577 || len(a.Artifacts) > 128 {
		return fail()
	}
	if a.Labels.Schema != "powerx.response-labels/v1" || a.Labels.RevisionUUID != a.RevisionUUID || len(a.Labels.Locales) == 0 {
		return fail()
	}
	sources := map[string]ArchivedSource{}
	artifacts := map[string]ArchivedArtifact{}
	for _, source := range a.Sources {
		if !canonicalUUID(source.SnapshotUUID) || !canonicalUUID(source.ObjectUUID) || len(source.JSON) > toolJSONLimit || toolDigest(source.JSON) != source.SHA256 {
			return fail()
		}
		if _, ok := sources[source.SnapshotUUID]; ok {
			return fail()
		}
		sources[source.SnapshotUUID] = source
	}
	for _, artifact := range a.Artifacts {
		if !validArtifact(artifact.Reference) || len(artifact.JSON) > toolJSONLimit || toolDigest(artifact.JSON) != artifact.Reference.SHA256 {
			return fail()
		}
		if _, ok := artifacts[artifact.Reference.ArtifactUUID]; ok {
			return fail()
		}
		artifacts[artifact.Reference.ArtifactUUID] = artifact
	}
	for _, receipt := range a.Receipts {
		if receipt.TenantUUID != a.TenantUUID || receipt.RevisionUUID != a.RevisionUUID || receipt.RunID != a.RunID || receipt.TraceID != a.TraceID {
			return fail()
		}
		request, ok := artifacts[receipt.Request.ArtifactUUID]
		if !ok || request.Reference != receipt.Request {
			return fail()
		}
		if receipt.Result != nil {
			result, ok := artifacts[receipt.Result.ArtifactUUID]
			if !ok || result.Reference != *receipt.Result {
				return fail()
			}
		}
	}
	if a.Failure != nil && (a.Report != nil || !boundedText(a.Failure.Code, 512) || !validFailureStage(a.Failure.Stage)) {
		return fail()
	}
	if a.Report == nil {
		return nil
	}
	if _, err := ValidateReportV5(a.Report); err != nil {
		return err
	}
	if !reflect.DeepEqual(a.Report.Receipts, a.Receipts) {
		return fail()
	}
	if a.Report.TenantUUID != a.TenantUUID || a.Report.RevisionUUID != a.RevisionUUID || a.Report.RunID != a.RunID || a.Report.TraceID != a.TraceID {
		return fail()
	}
	refs := []string{}
	for _, source := range a.Sources {
		refs = append(refs, source.SnapshotUUID+":"+source.SHA256)
	} // Export 顺序是固定 source key 顺序。
	raw, _ := json.Marshal(refs)
	if toolDigest(raw) != a.Report.SourceDigest {
		return fail()
	}
	for _, datum := range a.Report.Presentation.Reported {
		source, ok := sources[datum.Source.SnapshotUUID]
		if !ok || source.SHA256 != datum.Source.SnapshotSHA256 || source.Kind != datum.Source.Kind || source.ObjectType != datum.Source.ObjectType || source.ObjectUUID != datum.Source.ObjectUUID {
			return fail()
		}
		value, err := snapshotPointer(source.JSON, datum.Source.Pointer)
		if err != nil {
			return fail()
		}
		text, ok := value.(string)
		if !ok {
			number, ok := value.(json.Number)
			if !ok {
				return fail()
			}
			text = number.String()
		}
		span := datum.Source.Range
		if span.EndByte > len(text) || text[span.StartByte:span.EndByte] != datum.Source.Quote {
			return fail()
		}
	}
	for _, computed := range a.Report.Presentation.Computed {
		var receipt *ToolReceipt
		for i := range a.Report.Receipts {
			if a.Report.Receipts[i].ExecutionUUID == computed.ReceiptUUID {
				receipt = &a.Report.Receipts[i]
				break
			}
		}
		if receipt == nil || receipt.Result == nil {
			return fail()
		}
		raw := artifacts[receipt.Result.ArtifactUUID].JSON
		value, err := snapshotPointer(raw, computed.OutputPointer)
		if err != nil {
			return fail()
		}
		switch v := value.(type) {
		case string:
			if v != computed.Value {
				return fail()
			}
		case json.Number:
			if v.String() != computed.Value {
				return fail()
			}
		default:
			return fail()
		}
	}
	return nil
}

type invocationScope struct{ RunID, NodeID string }
type invocationScopeKey struct{}

func WithInvocationScope(ctx context.Context, runID, nodeID string) context.Context {
	return context.WithValue(ctx, invocationScopeKey{}, invocationScope{runID, nodeID})
}
func InvocationScope(ctx context.Context) (string, string) {
	scope, _ := ctx.Value(invocationScopeKey{}).(invocationScope)
	return scope.RunID, scope.NodeID
}

type toolAgentScopeKey struct{}

func WithToolAgentID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, toolAgentScopeKey{}, id)
}
func ToolAgentID(ctx context.Context) uint64 {
	id, _ := ctx.Value(toolAgentScopeKey{}).(uint64)
	return id
}
