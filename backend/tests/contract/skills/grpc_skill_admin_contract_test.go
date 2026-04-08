package skillscontract

import (
	"context"
	"net"
	"testing"
	"time"

	skillsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/skills/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillsgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/skills"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGRPCSkillAdminContract(t *testing.T) {
	deps := setupSkillsGRPCDeps(t)
	seedSkillsGRPCFixtures(t, deps.DB)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	skillsgrpc.RegisterAdminService(server, deps)
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
		<-done
	})

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := skillsv1.NewSkillAdminServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	importResp, err := client.ImportSkill(ctx, &skillsv1.ImportSkillRequest{
		SkillId:   "skill.grpc.import",
		Version:   "1.0.0",
		Source:    "plugin",
		BundleUri: "s3://skills/skill.grpc.import-1.0.0.tgz",
		Checksum:  "sha256-grpc-import-100",
	})
	require.NoError(t, err)
	require.Equal(t, "draft", importResp.GetSkill().GetStatus())

	listResp, err := client.ListSkills(ctx, &skillsv1.ListSkillsRequest{
		SkillId: "skill.grpc.import",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), listResp.GetTotal())
	require.Len(t, listResp.GetItems(), 1)

	pubResp, err := client.PublishSkill(ctx, &skillsv1.PublishSkillRequest{
		SkillId: "skill.lifecycle.grpc",
		Version: "2.0.0",
	})
	require.NoError(t, err)
	require.Equal(t, "published", pubResp.GetSkill().GetStatus())

	rollbackResp, err := client.RollbackSkill(ctx, &skillsv1.RollbackSkillRequest{
		SkillId:       "skill.lifecycle.grpc",
		TargetVersion: "1.0.0",
		Reason:        "contract rollback",
	})
	require.NoError(t, err)
	require.Equal(t, "1.0.0", rollbackResp.GetSkill().GetVersion())
	require.True(t, rollbackResp.GetSkill().GetIsLatestPublished())

	bindResp, err := client.BindCapability(ctx, &skillsv1.BindCapabilityRequest{
		SkillId:      "skill.lifecycle.grpc",
		Version:      "1.0.0",
		CapabilityId: "cap.skills.demo",
		ToolGrants:   []string{"grant.read"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, bindResp.GetBindingId())
	require.NotEmpty(t, bindResp.GetStatus())
}

func setupSkillsGRPCDeps(t *testing.T) *shared.Deps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&skillmodel.SkillRegistryRecord{},
		&skillmodel.OfficialSkillCatalogEntry{},
		&skillmodel.SkillCapabilityBinding{},
		&skillmodel.SkillExecutionTrace{},
		&skillmodel.SkillLifecycleAudit{},
	))
	return &shared.Deps{DB: db}
}

func seedSkillsGRPCFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle.grpc",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.lifecycle.grpc-1.0.0.tgz",
		Checksum:          "sha256-grpc-lifecycle-100",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle.grpc",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDraft,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.lifecycle.grpc-2.0.0.tgz",
		Checksum:          "sha256-grpc-lifecycle-200",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
}
