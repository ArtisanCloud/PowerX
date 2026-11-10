package iam

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDepartmentLeaderHTTPSetPreserveClearAndTenantIsolation(t *testing.T) {
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&m.User{}, &m.Member{}, &m.Department{}, &m.DepartmentClosure{}))
	tenant := uuid.NewString()
	user := m.User{Email: "leader@example.invalid", DisplayName: "Leader", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := m.Member{TenantUUID: tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "leader", DisplayName: "Leader", Status: 1}
	foreign := m.Member{TenantUUID: uuid.NewString(), UserID: user.ID, UserUUID: user.UUID.String(), Username: "foreign", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&foreign).Error)
	department := m.Department{TenantUUID: tenant, Name: "Engineering", Key: "engineering", Status: 1}
	require.NoError(t, svc.NewOrgService(db).CreateDepartment(context.Background(), &department, nil))
	h := NewDepartmentHandler(&shared.Deps{DB: db})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenant))
		c.Next()
	})
	router.PATCH("/departments/:id", h.Update)
	update := func(body string, status int) m.Department {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/departments/%d", department.ID), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, status, response.Code, response.Body.String())
		var saved m.Department
		require.NoError(t, db.First(&saved, department.ID).Error)
		return saved
	}
	saved := update(fmt.Sprintf(`{"leader_member_id":%d}`, member.ID), 200)
	require.Equal(t, &member.ID, saved.LeaderMemberID)
	require.Equal(t, &member.UUID, saved.LeaderMemberUUID)
	saved = update(`{"name":"Renamed"}`, 200)
	require.Equal(t, &member.ID, saved.LeaderMemberID)
	saved = update(fmt.Sprintf(`{"name":"Must rollback","leader_member_id":%d}`, foreign.ID), 400)
	require.Equal(t, "Renamed", saved.Name)
	require.Equal(t, &member.UUID, saved.LeaderMemberUUID)
	saved = update(`{"leader_member_id":null}`, 200)
	require.Nil(t, saved.LeaderMemberID)
	require.Nil(t, saved.LeaderMemberUUID)
	require.NoError(t, db.Model(&member).Update("status", 0).Error)
	update(fmt.Sprintf(`{"leader_member_id":%d}`, member.ID), 400)
}
