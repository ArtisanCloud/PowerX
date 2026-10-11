package plugin_identity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
)

func Decode(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(&struct{}{}) != io.EOF {
		return failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	return nil
}

type Invoker struct{ Service *Service }

func (i *Invoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != CapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if in.Endpoint != Endpoint || in.Method != "INVOKE" || len(in.Query) > 0 {
		return nil, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	for key := range in.Payload {
		if key != "body" && key != "method" && key != "endpoint" {
			return nil, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	var request struct {
		Operation string          `json:"operation"`
		Request   json.RawMessage `json:"request"`
	}
	if err = Decode(raw, &request); err != nil {
		return nil, err
	}
	var result Result
	switch request.Operation {
	case "resolve_request_identity":
		var query Query
		if err = Decode(request.Request, &query); err != nil {
			return nil, err
		}
		result, err = i.Service.Resolve(ctx, query)
	case "revalidate_ws_identity":
		var query WSQuery
		if err = Decode(request.Request, &query); err != nil {
			return nil, err
		}
		result, err = i.Service.RevalidateWS(ctx, query)
	default:
		return nil, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	if err != nil {
		return nil, err
	}
	if result.TenantUUID != in.TenantUUID {
		return nil, failure(403, "IAM_CONTEXT_TENANT_FORBIDDEN")
	}
	return map[string]interface{}{"result": result}, nil
}
