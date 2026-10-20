package notifications

import (
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPersonalInboxNeverUsesRootOrSubjectAsMemberWildcard(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/iam.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&tm.Tenant{}, &im.User{}, &im.Member{}))
	tenant := uuid.NewString()
	require.NoError(t, db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(tenant)}, Key: tenant, Name: tenant, Status: 1}).Error)
	user := im.User{Email: "root@inbox.test", Status: 1, IsRoot: true}
	require.NoError(t, db.Create(&user).Error)
	h := NewHandler(&shared.Deps{DB: db})
	request := func(memberID uint64) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/notifications", nil)
		claims := &reqctx.CoreXClaims{TenantUUID: tenant, UserID: user.ID, MemberID: memberID, IsRoot: true}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		ctx = reqctx.WithTenantUUID(ctx, tenant)
		ctx = reqctx.WithUserID(ctx, user.ID)
		ctx = reqctx.WithMemberID(ctx, memberID)
		ctx = reqctx.WithIsRoot(ctx, true)
		ctx = reqctx.WithSubject(ctx, "not-a-member-uuid")
		c.Request = c.Request.WithContext(ctx)
		return c
	}
	_, err = h.memberForRequest(request(0), tenant)
	require.Error(t, err, "empty member must not become an unrestricted inbox/delete filter")
	member := im.Member{TenantUUID: tenant, UserID: user.ID, Username: "root", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	id, err := h.memberForRequest(request(member.ID), tenant)
	require.NoError(t, err)
	require.Equal(t, member.UUID.String(), id)
}
