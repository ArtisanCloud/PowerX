package iamcontract

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestTenantMemberDirectoryHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	doc, err := (&openapi3.Loader{IsExternalRefsAllowed: true}).LoadFromFile(specPath)
	require.NoError(t, err)

	memberPath := doc.Paths.Find("/tenant/iam/members/{member_uuid}")
	require.NotNil(t, memberPath)
	require.NotNil(t, memberPath.Get)
	assertDirectoryErrors(t, memberPath.Get)
	require.NotNil(t, memberPath.Get.Security)
	require.Len(t, *memberPath.Get.Security, 2)
	require.Contains(t, (*memberPath.Get.Security)[0], "GatewayAPIKey")
	require.Contains(t, (*memberPath.Get.Security)[1], "StsTenantJWT")

	batchPath := doc.Paths.Find("/tenant/iam/members:batch-get")
	require.NotNil(t, batchPath)
	require.NotNil(t, batchPath.Post)
	assertDirectoryErrors(t, batchPath.Post)
	require.Len(t, *batchPath.Post.Security, 2)
	require.Contains(t, (*batchPath.Post.Security)[0], "GatewayAPIKey")
	require.Contains(t, (*batchPath.Post.Security)[1], "StsTenantJWT")

	requestSchema := batchPath.Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	memberUUIDs := requestSchema.Properties["member_uuids"]
	require.NotNil(t, memberUUIDs)
	require.Equal(t, "array", memberUUIDs.Value.Type)
	require.EqualValues(t, 1, memberUUIDs.Value.MinItems)

	for _, path := range []string{"/tenant/iam/tenant", "/tenant/iam/members"} {
		item := doc.Paths.Find(path)
		require.NotNilf(t, item, "missing %s", path)
		require.NotNil(t, item.Get)
		require.Len(t, *item.Get.Security, 2)
		require.Contains(t, (*item.Get.Security)[0], "GatewayAPIKey")
		require.Contains(t, (*item.Get.Security)[1], "StsTenantJWT")
	}
	membersPath := doc.Paths.Find("/tenant/iam/members")
	for _, status := range []string{"400", "401", "403", "503"} {
		_, ok := membersPath.Get.Responses[status]
		require.Truef(t, ok, "missing %s member page error response", status)
	}
	pageSchema := doc.Components.Schemas["IAMDirectoryMemberPageResponse"].Value.Properties["data"].Value
	require.Contains(t, pageSchema.Required, "items")
	require.Contains(t, pageSchema.Required, "pagination")

	resolvePath := doc.Paths.Find("/tenant/iam/members:batch-resolve")
	require.NotNil(t, resolvePath)
	require.NotNil(t, resolvePath.Post)
	assertResolveErrors(t, resolvePath.Post)
	require.Len(t, *resolvePath.Post.Security, 2)
	require.Contains(t, (*resolvePath.Post.Security)[0], "GatewayAPIKey")
	require.Contains(t, (*resolvePath.Post.Security)[1], "StsTenantJWT")
	resolveRequest := resolvePath.Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	require.NotNil(t, resolveRequest.Properties["member_uuids"])
	resolveResponse := doc.Components.Schemas["IAMDirectoryMemberResolveResponse"].Value
	resolveData := resolveResponse.Properties["data"].Value
	require.Contains(t, resolveData.Required, "items")
	require.Contains(t, resolveData.Required, "missing_member_uuids")

	errorSchema := doc.Components.Schemas["ErrorResponse"].Value
	require.NotNil(t, errorSchema.Properties["reason_code"])
	require.NotNil(t, errorSchema.Properties["error_code"])

	for _, path := range []string{"/tenant/iam/departments", "/tenant/iam/roles", "/tenant/iam/permissions"} {
		item := doc.Paths.Find(path)
		require.NotNilf(t, item, "missing %s", path)
		require.NotNil(t, item.Get)
		require.Len(t, *item.Get.Security, 2)
		require.Contains(t, (*item.Get.Security)[0], "GatewayAPIKey")
		require.Contains(t, (*item.Get.Security)[1], "StsTenantJWT")
	}

	authorizationPath := doc.Paths.Find("/tenant/iam/authorization:check")
	require.NotNil(t, authorizationPath)
	require.NotNil(t, authorizationPath.Post)
	assertDirectoryErrors(t, authorizationPath.Post)
	require.Len(t, *authorizationPath.Post.Security, 2)
	request := authorizationPath.Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	require.Equal(t, []string{"member_uuid", "user_uuid", "resource", "action"}, request.Required)
	response := doc.Components.Schemas["IAMAuthorizationCheck"].Value
	require.Equal(t, "boolean", response.Properties["allowed"].Value.Type)
	require.NotNil(t, response.Properties["reason_code"])

	memberSchema := doc.Components.Schemas["IAMDirectoryMember"].Value
	require.NotNil(t, memberSchema.Properties["department_uuids"])
}

func assertDirectoryErrors(t *testing.T, operation *openapi3.Operation) {
	t.Helper()
	for _, status := range []string{"400", "401", "403", "404", "503"} {
		_, ok := operation.Responses[status]
		require.Truef(t, ok, "missing %s directory error response", status)
	}
}

func assertResolveErrors(t *testing.T, operation *openapi3.Operation) {
	t.Helper()
	for _, status := range []string{"400", "401", "403", "503"} {
		_, ok := operation.Responses[status]
		require.Truef(t, ok, "missing %s batch resolve error response", status)
	}
	_, hasNotFound := operation.Responses["404"]
	require.False(t, hasNotFound, "batch resolve represents missing members in a 200 response")
}
