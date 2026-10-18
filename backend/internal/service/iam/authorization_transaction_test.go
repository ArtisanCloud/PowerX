package iam

import (
	"context"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestMemberWritesKeepOneTransactionAndDoNotRetainClosedRepositories(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &im.Credential{}, &im.MemberDepartment{}, &im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}))
	require.NoError(t, baseline.Install(db))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service := NewMemberService(db)
	tenant := uuid.NewString()
	id, err := service.CreateMember(ctx, tenant, CreateMemberInput{User: im.User{Email: uuid.NewString() + "@example.invalid"}, Member: im.Member{Username: "member", DisplayName: "reader", Status: 1}})
	require.NoError(t, err)
	require.NotZero(t, id)
	require.NoError(t, service.SetMemberStatus(ctx, tenant, id, 2, "test"))
	require.NoError(t, service.SetMemberStatus(ctx, tenant, id, 1, "test"))
	var member im.Member
	require.NoError(t, db.First(&member, id).Error)
	require.EqualValues(t, 1, member.Status)
}
