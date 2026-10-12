package runtime

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentProfileReaderReturnsOnlyGovernanceMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agents (
		id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
		env text, tenant_uuid text, name text, description text, source text, type_id text, scene text,
		status text, prompt_seed text, persona text
	)`).Error)
	tenantUUID := uuid.NewString()
	agentUUID := uuid.New()
	require.NoError(t, db.Exec("INSERT INTO public.agents (uuid, env, tenant_uuid, name, description, source, type_id, scene, status, prompt_seed, persona) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", agentUUID.String(), "test", tenantUUID, "Agent", "description", "core", "type", "scene", "active", "secret prompt", "secret persona").Error)

	ctx := context.WithValue(context.Background(), "env", "test")
	got, err := NewAgentProfileReader(db).ReadObservation(ctx, ResourceDescriptor{ResourceUUID: agentUUID, Kind: ResourceKindAgentProfile, TenantUUID: tenantUUID, ReadGranted: true}, "plan")
	require.NoError(t, err)
	require.Contains(t, got.Summary, "description")
	require.NotContains(t, got.Summary, "secret prompt")
	require.NotContains(t, got.Summary, "secret persona")
}
