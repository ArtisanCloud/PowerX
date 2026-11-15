package capability_registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestIAMOrganizationTypedInvocationPreservesBusinessErrors(t *testing.T) {
	for _, item := range []struct {
		status int
		code   string
	}{
		{400, "IAM_ORGANIZATION_INVALID"},
		{401, "IAM_ORGANIZATION_SERVICE_REQUIRED"},
		{403, "IAM_ORGANIZATION_TENANT_UNAVAILABLE"},
		{503, "IAM_ORGANIZATION_UNAVAILABLE"},
		{503, "IAM_ORGANIZATION_SCHEMA_NOT_READY"},
		{503, "IAM_ORGANIZATION_LIMIT"},
		{503, "IAM_ORGANIZATION_IDENTITY_INCOMPLETE"},
	} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wrapped=%t", item.code, wrapped), func(t *testing.T) {
				var source error = dto.NewErrorWithCode(item.status, item.code, item.code, nil)
				if wrapped {
					source = fmt.Errorf("typed invocation: %w", source)
				}
				invoker := &externalIdentityContractInvoker{err: source}
				engine := externalIdentityContractEngine(invoker, true)
				body, err := json.Marshal(iamOrganizationInvocationRequest())
				require.NoError(t, err)
				request := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Request-ID", "fixture_organization_request")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)

				require.Equal(t, item.status, response.Code)
				var envelope dto.ErrorResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				require.Equal(t, item.status, envelope.Code)
				require.Equal(t, item.code, envelope.ErrorCode)
				require.Equal(t, item.code, envelope.ReasonCode)
				require.Equal(t, item.code, envelope.Details["reason_code"])
				require.Equal(t, "fixture_organization_request", envelope.RequestID)
				require.Equal(t, "fixture_organization_trace", envelope.Details["trace_id"])
				require.Equal(t, iamsvc.MemberAuthorityCapability, invoker.input.CapabilityID)
				require.Equal(t, externalIdentityContractTenantUUID, invoker.input.TenantUUID)
			})
		}
	}
}

func TestIAMOrganizationUnclassifiedFailureRemainsBadGateway(t *testing.T) {
	for _, source := range []error{
		errors.New("IAM_ORGANIZATION_SCHEMA_NOT_READY"),
		fmt.Errorf("typed invocation: %w", errors.New("IAM_ORGANIZATION_INVALID")),
		&capservice.RESTUpstreamError{StatusCode: http.StatusServiceUnavailable, RemoteCode: "IAM_ORGANIZATION_SCHEMA_NOT_READY"},
		dto.NewErrorWithCode(http.StatusForbidden, "UNKNOWN_ORGANIZATION_FAILURE", "fixture_unknown_error", nil),
	} {
		invoker := &externalIdentityContractInvoker{err: source}
		response := invokeExternalIdentityContract(t, externalIdentityContractEngine(invoker, true), iamOrganizationInvocationRequest())
		require.Equal(t, http.StatusBadGateway, response.Code, "only classified typed business errors preserve their status")
	}
}

func iamOrganizationInvocationRequest() map[string]interface{} {
	return map[string]interface{}{
		"capability_id": iamsvc.MemberAuthorityCapability, "preferred_protocol": "core_internal", "trace_id": "fixture_organization_trace",
		"payload": map[string]interface{}{"body": map[string]interface{}{
			"operation": "read_member_authority",
			"request": map[string]interface{}{
				"schema": "powerx.iam.member-authority-query.v1", "member_uuids": []string{"00000000-0000-4000-8000-000000000002"},
			},
		}},
	}
}
