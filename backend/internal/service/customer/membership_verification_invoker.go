package customer

import (
	"context"
	"encoding/json"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// VerifyCustomerRequest 是固定的 v1 服务核验正文，不携带调用身份或租户。
type VerifyCustomerRequest struct {
	Operation     string `json:"operation"`
	SchemaVersion int    `json:"schema_version"`
	CustomerUUID  string `json:"customer_uuid"`
}

func decodeVerifyCustomerRequest(body map[string]interface{}) (VerifyCustomerRequest, error) {
	var request VerifyCustomerRequest
	if len(body) != 3 {
		return request, ErrVerificationInvalidArgument
	}
	for key := range body {
		if key != "operation" && key != "schema_version" && key != "customer_uuid" {
			return request, ErrVerificationInvalidArgument
		}
	}
	raw, err := json.Marshal(body)
	if err != nil || json.Unmarshal(raw, &request) != nil || request.Operation != "verify_customer" || request.SchemaVersion != 1 || !verificationUUID(request.CustomerUUID) {
		return request, ErrVerificationInvalidArgument
	}
	return request, nil
}

func (i *CapabilityInvoker) invokeMembershipVerification(ctx context.Context, in capabilityregistry.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.Method != "INVOKE" || in.Endpoint != CustomerMembershipsCoreEndpoint || len(in.Query) != 0 || len(in.Context) != 0 {
		return nil, ErrVerificationInvalidArgument
	}
	for key := range in.Payload {
		if key != "body" {
			return nil, ErrVerificationInvalidArgument
		}
	}
	if in.TenantUUID == "" || in.TenantUUID != reqctx.GetTenantUUID(ctx) {
		return nil, ErrVerificationForbidden
	}
	request, err := decodeVerifyCustomerRequest(in.Body)
	if err != nil {
		return nil, err
	}
	result, err := i.memberships.VerifyCustomer(ctx, request.CustomerUUID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"item": result}, nil
}
