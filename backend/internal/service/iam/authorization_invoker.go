package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

const AuthorizationCheckEndpoint = "core://iam/authorization"

type AuthorizationInvoker struct{ Service *TenantDirectoryService }

func (i *AuthorizationInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID == MemberAuthorityCapability {
		if in.Endpoint != MemberAuthorityEndpoint || in.Method != "INVOKE" || len(in.Query) != 0 || len(in.Context) != 0 || in.TenantUUID != reqctx.GetTenantUUID(ctx) {
			return nil, DirectoryInvalidArgumentError(nil)
		}
		for key := range in.Payload {
			if key != "body" {
				return nil, DirectoryInvalidArgumentError(nil)
			}
		}
		var op struct {
			Operation string               `json:"operation"`
			Request   MemberAuthorityQuery `json:"request"`
		}
		raw, _ := json.Marshal(in.Body)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&op) != nil || decoder.Decode(&struct{}{}) != io.EOF || op.Operation != "read_member_authority" {
			return nil, DirectoryInvalidArgumentError(nil)
		}
		if i == nil || i.Service == nil {
			return nil, organizationError(503, "IAM_ORGANIZATION_UNAVAILABLE")
		}
		result, err := i.Service.ReadMemberAuthority(ctx, op.Request)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"result": result}, nil
	}
	if in.CapabilityID != directoryAuthorizationCheckCapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if in.Endpoint != AuthorizationCheckEndpoint || in.Method != "INVOKE" || len(in.Query) != 0 || len(in.Context) != 0 {
		return nil, DirectoryInvalidArgumentError(nil)
	}
	for key := range in.Payload {
		if key != "body" {
			return nil, DirectoryInvalidArgumentError(nil)
		}
	}
	raw, _ := json.Marshal(in.Body)
	var request struct {
		Operation string `json:"operation"`
		Request   struct {
			MemberUUID     string `json:"member_uuid"`
			UserUUID       string `json:"user_uuid"`
			PluginID       string `json:"plugin_id"`
			PermissionCode string `json:"permission_code"`
			Resource       string `json:"resource"`
			Action         string `json:"action"`
		} `json:"request"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&request) != nil || dec.Decode(&struct{}{}) != io.EOF || request.Operation != "check" {
		return nil, DirectoryInvalidArgumentError(nil)
	}
	r := request.Request
	// 服务内再次校验实时 grant；Admin JWT 不能通过内部分发绕过。
	access, err := cap.NewGrantStatusService(i.Service.db).CurrentAccess(ctx)
	if err != nil {
		return nil, pluginAuthorizationAccessError(err)
	}
	if err = access.Require(directoryAuthorizationCheckCapabilityID); err != nil {
		return nil, pluginAuthorizationAccessError(err)
	}
	if c := reqctx.GetClaims(ctx); c != nil && c.Issuer == "powerx-sts" && reqctx.STSIssuanceStatus(ctx) != 0 {
		if reqctx.STSIssuanceStatus(ctx) == 503 {
			return nil, DirectoryUpstreamDependencyError(nil)
		}
		return nil, directoryError(401, DirectoryReasonUnauthorized, nil)
	}
	result, err := i.Service.CheckAuthorization(ctx, in.TenantUUID, AuthorizationCheckInput{MemberUUID: r.MemberUUID, UserUUID: r.UserUUID, PluginID: r.PluginID, PermissionCode: r.PermissionCode, Resource: r.Resource, Action: r.Action})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"result": result}, nil
}
