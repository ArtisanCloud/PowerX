package storage

import (
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDatabaseRetentionTimeoutPreservesCodeWithoutMigrationHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/admin/ops/storage/database-retention/previews", nil)
	(&handler{}).stateError(c, dto.NewErrorWithCode(503, "DATABASE_RETENTION_QUERY_TIMEOUT", "DATABASE_RETENTION_QUERY_TIMEOUT", nil))
	require.Equal(t, 503, w.Code)
	require.Contains(t, w.Body.String(), "DATABASE_RETENTION_QUERY_TIMEOUT")
	require.NotContains(t, w.Body.String(), "STORAGE_STATE_UNAVAILABLE")
	require.NotContains(t, w.Body.String(), "迁移")
}
