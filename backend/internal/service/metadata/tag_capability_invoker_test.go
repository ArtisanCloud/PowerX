package metadata

import (
	"context"
	"testing"

	capsvc "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestTagCapabilityUpdateAndStrictContract(t *testing.T) {
	db := newServiceTagTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
	tenant := uuid.NewString()
	plugin := "com.powerx.tag.test"
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: TagManageCapabilityID, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant, CapabilityID: TagManageCapabilityID, ContractRef: "test", Status: "published", Version: 1}).Error)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"allowed_capabilities":["com.corex.metadata.tag.manage"]}`)}).Error)
	row := model.Tag{TenantUUID: tenant, Namespace: "corex.test", ResourceType: "test.item", Code: "test", LabelI18n: datatypes.JSON(`{"zh-CN":"标签"}`)}
	require.NoError(t, db.Create(&row).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}})
	invoker := NewTagCapabilityInvoker(db)
	input := capsvc.CoreCapabilityInvokeInput{TenantUUID: tenant, CapabilityID: TagManageCapabilityID, Method: "INVOKE", Endpoint: "core://metadata/tags", Body: map[string]interface{}{"operation": "update", "tag_uuid": row.UUID.String(), "color": "#123456"}}
	_, err := invoker.InvokeCoreCapability(ctx, input)
	require.NoError(t, err)
	for _, field := range []string{"tenant_uuid", "actor", "plugin_id", "method", "endpoint", "headers", "raw_body"} {
		input.Body[field] = "override"
		_, err = invoker.InvokeCoreCapability(ctx, input)
		var app *dto.AppError
		require.ErrorAs(t, err, &app)
		require.Equal(t, 400, app.HTTPCode)
		delete(input.Body, field)
	}
	input.Method = "PATCH"
	_, err = invoker.InvokeCoreCapability(ctx, input)
	require.Error(t, err)
	input.Method = "INVOKE"
	input.Endpoint = "/api/v1/admin/metadata/tags"
	_, err = invoker.InvokeCoreCapability(ctx, input)
	require.Error(t, err)
	input.Endpoint = "core://metadata/tags"
	input.Body["tag_uuid"] = uuid.NewString()
	_, err = invoker.InvokeCoreCapability(ctx, input)
	var app *dto.AppError
	require.ErrorAs(t, err, &app)
	require.Equal(t, 404, app.HTTPCode)
	input.Body["tag_uuid"] = "invalid"
	_, err = invoker.InvokeCoreCapability(ctx, input)
	require.ErrorAs(t, err, &app)
	require.Equal(t, 400, app.HTTPCode)
	input.Body["tag_uuid"] = row.UUID.String()
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("plugin_id = ?", plugin).Update("enabled", false).Error)
	_, err = invoker.InvokeCoreCapability(ctx, input)
	require.ErrorAs(t, err, &app)
	require.Equal(t, 403, app.HTTPCode)
	var actual model.Tag
	require.NoError(t, db.First(&actual, "uuid = ?", row.UUID).Error)
	require.Equal(t, "#123456", actual.Color)
}
