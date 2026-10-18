// Package mediaevents shares Media invalidations between admin and service actors.
package mediaevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Authorize func(context.Context) error

// Serve subscribes before the initial read and never polls status. Service
// access is checked before reading and emitting each snapshot, using live grants.
func Serve(c *gin.Context, client *redis.Client, svc *mediasvc.MediaService, tenant, asset string, authorize Authorize) {
	if svc == nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUpstreamDependencyError(errors.New("media service unavailable")))
		return
	}
	serve(c, client, tenant, asset, authorize, func(ctx context.Context) (*mediasvc.ImageDerivatives, error) {
		return svc.ImageDerivativeStatus(ctx, tenant, asset)
	}, time.Minute, 15*time.Second)
}

func serve(c *gin.Context, client *redis.Client, tenant, asset string, authorize Authorize, status func(context.Context) (*mediasvc.ImageDerivatives, error), lifetime, heartbeatInterval time.Duration) {
	id, err := uuid.Parse(asset)
	if err != nil || id == uuid.Nil || id.String() != asset || c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, mediasvc.MediaInvalidArgumentError(errors.New("invalid derivative event request")))
		return
	}
	if client == nil || status == nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUpstreamDependencyError(errors.New("media events unavailable")))
		return
	}
	deadline := time.Now().Add(lifetime)
	if claims := reqctx.GetClaims(c.Request.Context()); claims != nil && claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(deadline) {
		deadline = claims.ExpiresAt.Time
	}
	ctx, cancel := context.WithDeadline(c.Request.Context(), deadline)
	defer cancel()
	check := func() error {
		if err := ctx.Err(); err != nil {
			return mediasvc.MediaUnauthorizedError(err)
		}
		if authorize != nil {
			return authorize(ctx)
		}
		return nil
	}
	if err := check(); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	sub := client.Subscribe(ctx, mediasvc.ImageDerivativeEventChannel(tenant, asset))
	defer sub.Close()
	if _, err = sub.Receive(ctx); err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUpstreamDependencyError(err))
		return
	}
	snapshot := func() (*mediasvc.ImageDerivatives, error) {
		if err := check(); err != nil {
			return nil, err
		}
		state, err := status(ctx)
		if errors.Is(err, mediasvc.ErrAssetNotFound) {
			err = mediasvc.MediaAssetNotFoundError(err)
		} else if err != nil {
			err = mediasvc.MediaUpstreamDependencyError(err)
		}
		if err == nil {
			err = check()
		}
		return state, err
	}
	initial, err := snapshot()
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Header("X-Content-Type-Options", "nosniff")
	ctrl := http.NewResponseController(c.Writer)
	write := func(name string, data any) error {
		raw, e := json.Marshal(data)
		if e != nil {
			return e
		}
		_ = ctrl.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, e = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, raw); e != nil {
			return e
		}
		return ctrl.Flush()
	}
	if write("status", initial) != nil {
		return
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	changes := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			if c.Request.Context().Err() == nil {
				_ = write("reconnect", gin.H{"reason": "reauthorize"})
			}
			return
		case _, ok := <-changes:
			if !ok {
				return
			}
			state, err := snapshot()
			if err != nil {
				_ = write("invalidate", gin.H{"asset_uuid": asset, "reason": "access_or_state_changed"})
				return
			}
			if write("status", state) != nil {
				return
			}
		case <-heartbeat.C:
			if ctx.Err() != nil {
				continue
			}
			_ = ctrl.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil || ctrl.Flush() != nil {
				return
			}
		}
	}
}
