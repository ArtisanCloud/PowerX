package authorization_baseline

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
)

type Invoker struct{ Service *Service }

func decode(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(&struct{}{}) != io.EOF {
		return failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	return nil
}
func (i *Invoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != CapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if in.Endpoint != Endpoint || in.Method != "INVOKE" || len(in.Query) > 0 || len(in.Context) > 0 {
		return nil, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	for key := range in.Payload {
		if key != "body" {
			return nil, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	var op struct {
		Operation string          `json:"operation"`
		Request   json.RawMessage `json:"request"`
	}
	if err = decode(raw, &op); err != nil {
		return nil, err
	}
	if op.Operation != Operation {
		return nil, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	var query Query
	if err = decode(op.Request, &query); err != nil {
		return nil, err
	}
	result, err := i.Service.ReadMemberBaselines(ctx, query)
	if err != nil {
		return nil, err
	}
	if result.TenantUUID != in.TenantUUID {
		return nil, failure(403, "IAM_BASELINE_SERVICE_FORBIDDEN")
	}
	return map[string]interface{}{"result": result}, nil
}
