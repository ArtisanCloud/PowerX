package skillsintegration

import (
	"context"
	"net"
	"testing"
	"time"

	skillsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/skills/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestLegacyGRPCRequiresDefinitionUpgrade(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{SkillID: "customer.parcel.legacy", Version: "1.0.0", Source: skillmodel.SkillSourcePlugin, Status: skillmodel.SkillStatusPublished, IsLatestPublished: true, BundleURI: "s3://fixture/legacy.tgz", Checksum: "sha256:fixture", ImportType: "upload", UpdatedBy: "fixture"}).Error)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	skillgrpc.RegisterInvokeService(server, &shared.Deps{DB: db})
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	go func() { _ = server.Serve(listener) }()
	connection, err := grpc.NewClient("passthrough:///skills", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = skillsv1.NewSkillInvokeServiceClient(connection).InvokeSkill(ctx, &skillsv1.InvokeSkillRequest{TenantUuid: uuid.NewString(), SkillId: "customer.parcel.legacy", Version: "1.0.0"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Equal(t, skillservice.ErrDefinitionContractUpgradeRequired.Error(), status.Convert(err).Message())
}
