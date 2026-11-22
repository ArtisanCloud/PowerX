package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const DatasetRegisterCapability = "com.corex.storage.datasets.register"
const datasetEndpoint = "core://storage-governance/datasets"

type DatasetRegisterInvoker struct {
	Service func() *Service
}

type datasetRegisterOperation struct {
	ContractVersion string `json:"contract_version"`
	Operation       string `json:"operation"`
	Input           struct {
		RequestUUID       string `json:"request_uuid"`
		DatasetKey        string `json:"dataset_key"`
		DefinitionVersion int64  `json:"definition_version"`
		DefinitionDigest  string `json:"definition_digest"`
		SourceBindingRef  string `json:"source_binding_ref"`
	} `json:"input"`
}

func (i *DatasetRegisterInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	if in.CapabilityID == DatasetExecutionCapability {
		return i.invokeExecution(ctx, in)
	}
	if in.CapabilityID == DatasetPreviewCapability {
		return i.invokePreview(ctx, in)
	}
	if in.CapabilityID != DatasetRegisterCapability {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	bad := func() error { return controlError(400, "DATASET_REGISTRATION_INVALID") }
	if in.Method != "INVOKE" || in.Endpoint != datasetEndpoint || len(in.Query) != 0 || len(in.Context) != 0 || len(in.Payload) != 1 {
		return nil, bad()
	}
	if _, ok := in.Payload["body"]; !ok {
		return nil, bad()
	}
	raw, err := json.Marshal(in.Body)
	if err != nil || len(raw) > 4096 {
		return nil, bad()
	}
	var op datasetRegisterOperation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&op) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		op.ContractVersion != datasetHostContract || op.Operation != "register" ||
		!datasetKeyPattern.MatchString(op.Input.DatasetKey) || !datasetKeyPattern.MatchString(op.Input.SourceBindingRef) ||
		!datasetRevisionOK(op.Input.DefinitionVersion) || !datasetDigestPattern.MatchString(op.Input.DefinitionDigest) {
		return nil, bad()
	}
	if _, err := parseDatasetUUID(op.Input.RequestUUID); err != nil {
		return nil, bad()
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.PluginID == "" {
		return nil, controlError(403, "DATASET_STS_REQUIRED")
	}
	tid := reqctx.GetTenantUUID(ctx)
	tenant, err := parseDatasetUUID(tid)
	if err != nil || tid != claims.TenantUUID || tid != in.TenantUUID {
		return nil, controlError(403, "DATASET_TENANT_FORBIDDEN")
	}
	if i.Service == nil {
		return nil, controlError(503, "DATASET_REGISTRATION_UNAVAILABLE")
	}
	s := i.Service()
	if s == nil || s.opts.DB == nil {
		return nil, controlError(503, "DATASET_REGISTRATION_UNAVAILABLE")
	}
	access, err := cap.NewGrantStatusService(s.opts.DB).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(in.CapabilityID)
	}
	if err != nil {
		var grantErr *cap.DirectGrantError
		if errors.As(err, &grantErr) && grantErr.Status < 500 {
			return nil, controlError(403, "DATASET_REGISTRATION_FORBIDDEN")
		}
		return nil, controlError(503, "DATASET_REGISTRATION_UNAVAILABLE")
	}
	if !access.OwnsPlugin(claims.PluginID) || access.Subject == "" {
		return nil, controlError(403, "DATASET_OWNER_REQUIRED")
	}
	if err := pluginservice.NewPluginDrainJobService(s.opts.DB).EnsurePluginAcceptsNewUsage(ctx, claims.PluginID); err != nil {
		var appError *dto.AppError
		if errors.As(err, &appError) && appError.HTTPCode == 409 {
			return nil, controlError(409, "DATASET_PLUGIN_DRAINING")
		}
		return nil, controlError(503, "DATASET_REGISTRATION_UNAVAILABLE")
	}
	source, err := s.resolveInstalledDataset(ctx, claims.PluginID, op.Input.DatasetKey)
	if err != nil {
		return nil, err
	}
	def := source.declaration
	if source.spec.SHA256 != op.Input.DefinitionDigest || def.ContractVersion != op.Input.DefinitionVersion ||
		def.SourceBindingRef != op.Input.SourceBindingRef || def.SourceBindingRef != "database" {
		return nil, controlError(409, "DATASET_DEFINITION_CONFLICT")
	}
	_, fingerprint, _ := s.sampleInstalledDataset(ctx, source)
	result, err := s.AdmitVerifiedDataset(ctx, VerifiedDatasetAdmission{
		RequestUUID: op.Input.RequestUUID, Actor: access.Subject, CallerPluginID: claims.PluginID, OwnerID: source.plugin.ID,
		TenantUUID: tenant, Installation: source.plugin.Revision, BindingUUID: source.binding,
		DefinitionDigest: source.definitionFingerprint(), SourceFingerprint: fingerprint, Declaration: def,
	})
	if err != nil {
		var appError *dto.AppError
		if errors.As(err, &appError) {
			return nil, err
		}
		return nil, controlError(503, "DATASET_REGISTRATION_UNAVAILABLE")
	}
	return map[string]any{"result": result}, nil
}
