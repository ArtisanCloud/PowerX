package mediaevents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestHeartbeatsNeverReadStatusAndConnectionReauthorizes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var reads atomic.Int32
	engine := gin.New()
	asset := uuid.NewString()
	engine.GET("/events", func(c *gin.Context) {
		serve(c, client, "tenant", asset, nil, func(context.Context) (*mediasvc.ImageDerivatives, error) {
			reads.Add(1)
			return &mediasvc.ImageDerivatives{AssetUUID: asset, Status: "queued"}, nil
		}, time.Second, 10*time.Millisecond)
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), ": keepalive")
	require.Contains(t, w.Body.String(), "event: reconnect")
	require.Equal(t, int32(1), reads.Load(), "idle heartbeats cannot become status polling")
}

func TestExpiredCredentialCannotOpenOrEmitStatus(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var reads atomic.Int32
	engine := gin.New()
	engine.GET("/events", func(c *gin.Context) {
		claims := &reqctx.CoreXClaims{RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Second))}}
		c.Request = c.Request.WithContext(reqctx.WithClaims(c.Request.Context(), claims))
		serve(c, client, "tenant", uuid.NewString(), nil, func(context.Context) (*mediasvc.ImageDerivatives, error) { reads.Add(1); return nil, nil }, time.Minute, time.Second)
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
	require.Equal(t, 401, w.Code)
	require.Zero(t, reads.Load())
	require.NotContains(t, w.Body.String(), "event: status")
}

func TestCancelledClientReleasesRedisSubscription(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	asset := uuid.NewString()
	channel := mediasvc.ImageDerivativeEventChannel("tenant", asset)
	engine := gin.New()
	closed := make(chan struct{})
	engine.GET("/events", func(c *gin.Context) {
		defer close(closed)
		serve(c, client, "tenant", asset, nil, func(context.Context) (*mediasvc.ImageDerivatives, error) {
			return &mediasvc.ImageDerivatives{Status: "queued"}, nil
		}, time.Minute, time.Second)
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	cancel()
	_ = resp.Body.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("subscription handler did not exit")
	}
	require.Eventually(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), channel).Result()
		return err == nil && counts[channel] == 0
	}, time.Second, 10*time.Millisecond)
}
