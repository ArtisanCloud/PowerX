package system

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSystemUserHTTPDepartmentsBindFilterReadAndClear(t *testing.T) {
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&tm.Tenant{}, &m.User{}, &m.Member{}, &m.Department{}, &m.DepartmentClosure{}, &m.MemberDepartment{}))
	tenant := tm.Tenant{Key: "test", Name: "Test", Status: 1}
	require.NoError(t, db.Create(&tenant).Error)
	user := m.User{Email: "user@example.invalid", DisplayName: "User", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := m.Member{TenantUUID: tenant.UUID.String(), UserID: user.ID, UserUUID: user.UUID.String(), Username: "user", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	parent := m.Department{TenantUUID: tenant.UUID.String(), Name: "Parent", Key: "parent", Status: 1}
	child := m.Department{TenantUUID: tenant.UUID.String(), Name: "Child", Key: "child", Status: 1}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&m.DepartmentClosure{TenantUUID: tenant.UUID.String(), AncestorID: parent.ID, DescendantID: child.ID, Depth: 1}).Error)
	h := NewUserHandler(&shared.Deps{DB: db})
	router := gin.New()
	router.GET("/users", h.List)
	router.PATCH("/users/:user_uuid", h.Update)
	patch := func(ids []uint64) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"tenant_uuid": tenant.UUID.String(), "dept_ids": ids})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPatch, "/users/"+user.UUID.String(), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	patch([]uint64{child.ID}) // 仅修改部门字段不能被当作空请求。
	list := func(suffix string, total int) {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users?tenant_uuid=%s&dept_id=%d%s", tenant.UUID, parent.ID, suffix), nil))
		require.Equal(t, 200, response.Code, response.Body.String())
		var body struct {
			Data struct {
				Items      []MemberWithProfileDTO `json:"items"`
				Pagination struct {
					Total int `json:"total"`
				} `json:"pagination"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Equal(t, total, body.Data.Pagination.Total)
		if total > 0 {
			require.Len(t, body.Data.Items, 1)
			require.Equal(t, []uint64{child.ID}, body.Data.Items[0].DeptIDs)
		}
	}
	list("&recursive=true", 1)
	list("&recursive=false", 0)
	patch([]uint64{})
	list("&recursive=true", 0)
}
