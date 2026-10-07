package agent_session

import (
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestArchiveBackpressureReturnsRetryableReason(t *testing.T) {
	for _, item := range []struct {
		err    error
		status int
		code   string
	}{{agent_run.ErrArchiveBackpressure, 429, "archive.backpressure"}, {agent_run.ErrArchiveHealthStale, 503, "archive.health_stale"}} {
		t.Run(item.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/invocations", nil)
			respondError(ctx, item.err)
			require.Equal(t, item.status, recorder.Code)
			require.Equal(t, "30", recorder.Header().Get("Retry-After"))
			require.Contains(t, recorder.Body.String(), item.code)
		})
	}
}
