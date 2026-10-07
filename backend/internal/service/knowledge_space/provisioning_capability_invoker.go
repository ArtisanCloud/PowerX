package knowledge_space

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type ProvisioningCapabilityInvoker struct {
	db      *gorm.DB
	service func() *Service
	host    func() *HostContractService
}

func NewProvisioningCapabilityInvoker(db *gorm.DB, service func() *Service, host ...func() *HostContractService) *ProvisioningCapabilityInvoker {
	out := &ProvisioningCapabilityInvoker{db: db, service: service}
	if len(host) > 0 {
		out.host = host[0]
	}
	return out
}
func (i *ProvisioningCapabilityInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != KnowledgeCatalogReadCapabilityID && in.CapabilityID != KnowledgeSpaceCreateCapabilityID && in.CapabilityID != KnowledgeDocumentManageCapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	endpoint := "core://knowledge/catalog"
	if in.CapabilityID == KnowledgeDocumentManageCapabilityID {
		endpoint = "core://knowledge/documents"
	}
	if in.CapabilityID == KnowledgeSpaceCreateCapabilityID {
		endpoint = "core://knowledge/spaces"
	}
	if in.Method != "INVOKE" || in.Endpoint != endpoint || len(in.Query) != 0 {
		return nil, KnowledgeInvalidArgumentError(errors.New("invalid typed knowledge binding"))
	}
	for key := range in.Payload {
		if key != "body" && key != "method" && key != "endpoint" {
			return nil, KnowledgeInvalidArgumentError(errors.New("unsupported knowledge invocation envelope field: " + key))
		}
	}
	access := NewHostContractAccess(i.db)
	var tenant string
	var err error
	if in.CapabilityID == KnowledgeCatalogReadCapabilityID {
		tenant, err = access.AuthorizeCatalogRead(ctx, reqctx.AuthenticatedAPIKeyHash(ctx))
	} else if in.CapabilityID == KnowledgeDocumentManageCapabilityID {
		tenant, err = access.AuthorizeDocumentManage(ctx, reqctx.AuthenticatedAPIKeyHash(ctx))
	} else {
		tenant, err = access.AuthorizeSpaceCreate(ctx, reqctx.AuthenticatedAPIKeyHash(ctx))
	}
	if err != nil {
		return nil, err
	}
	if tenant != in.TenantUUID {
		return nil, KnowledgeForbiddenError(errors.New("tenant context mismatch"))
	}
	if in.CapabilityID == KnowledgeDocumentManageCapabilityID {
		if i.host == nil || i.host() == nil {
			return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge document runtime unavailable"))
		}
		return invokeHostDocument(ctx, i.host(), tenant, in.Body)
	}
	if i.service == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge provisioning unavailable"))
	}
	svc := i.service()
	if svc == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge provisioning unavailable"))
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if in.CapabilityID == KnowledgeCatalogReadCapabilityID {
		var request struct {
			Operation string `json:"operation"`
		}
		if err := decoder.Decode(&request); err != nil || request.Operation != "catalog" {
			return nil, KnowledgeInvalidArgumentError(errors.New("expected catalog operation"))
		}
		catalog, err := svc.GetHostCatalog(ctx, tenant)
		return map[string]interface{}{"catalog": catalog}, err
	}
	var request struct {
		Operation string `json:"operation"`
		HostCreateSpaceRequest
	}
	if err := decoder.Decode(&request); err != nil || request.Operation != "create" {
		return nil, KnowledgeInvalidArgumentError(errors.New("expected typed create operation"))
	}
	item, err := svc.CreateHostSpace(ctx, tenant, request.HostCreateSpaceRequest)
	return map[string]interface{}{"item": item}, err
}

func invokeHostDocument(ctx context.Context, service *HostContractService, tenant string, body any) (map[string]interface{}, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	var op struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(raw, &op) != nil {
		return nil, KnowledgeInvalidArgumentError(errors.New("invalid typed operation"))
	}
	switch op.Operation {
	case "submit_document":
		var in struct {
			Operation string            `json:"operation"`
			SpaceUUID string            `json:"space_uuid"`
			Document  HostDocumentInput `json:"document"`
		}
		if strictJSON(raw, &in) != nil {
			return nil, KnowledgeInvalidArgumentError(errors.New("expected typed document input"))
		}
		job, err := service.UpsertDocument(ctx, tenant, in.SpaceUUID, in.Document)
		return map[string]interface{}{"job": job}, err
	case "get_job", "get_job_chunks":
		var in struct {
			Operation string `json:"operation"`
			JobUUID   string `json:"job_uuid"`
		}
		if strictJSON(raw, &in) != nil {
			return nil, KnowledgeInvalidArgumentError(errors.New("expected typed job input"))
		}
		if op.Operation == "get_job_chunks" {
			items, err := service.GetJobChunks(ctx, tenant, in.JobUUID)
			return map[string]interface{}{"items": items}, err
		}
		job, err := service.GetIndexJob(ctx, tenant, in.JobUUID)
		return map[string]interface{}{"job": job}, err
	default:
		return nil, KnowledgeInvalidArgumentError(errors.New("unsupported knowledge document operation"))
	}
}
