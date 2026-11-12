package plugin

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	install "github.com/ArtisanCloud/PowerX/internal/service/plugin_install"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func installationService(c *gin.Context, deps *shared.Deps) *install.Service {
	if deps == nil || deps.PluginInstallations == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "PLUGIN_INSTALL_TASKS_UNAVAILABLE", "Plugin installation tasks are unavailable", nil))
		return nil
	}
	return deps.PluginInstallations
}
func PluginInstallationCreate(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		var in install.Input
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		if e := c.ShouldBindJSON(&in); e != nil {
			dto.ResponseError(c, 400, "PLUGIN_INSTALL_TASK_INVALID", nil)
			return
		}
		meta, e := coalesceInstallMetadata(c, in.Metadata)
		if e != nil {
			dto.ResponseError(c, 400, "PLUGIN_INSTALL_METADATA_INVALID", nil)
			return
		}
		in.Metadata = meta
		out, e := svc.Create(c.Request.Context(), in)
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		dto.ResponseSuccess(c, out)
	}
}
func PluginInstallationList(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		out, e := svc.List(c.Request.Context())
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		dto.ResponseSuccess(c, gin.H{"items": out})
	}
}
func PluginInstallationGet(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		out, e := svc.Get(c.Request.Context(), c.Param("task_id"))
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		dto.ResponseSuccess(c, out)
	}
}
func PluginInstallationDiscard(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		out, e := svc.Discard(c.Request.Context(), c.Param("task_id"))
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		dto.ResponseSuccess(c, out)
	}
}
func PluginInstallationUpload(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		state, e := svc.Get(c.Request.Context(), c.Param("task_id"))
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		if state.Status != "awaiting_upload" {
			dto.ResponseSuccess(c, state)
			return
		}
		// Admission covers multipart spill and bounded extraction on their actual filesystem.
		run, release, e := budget.BeginPath(c.Request.Context(), os.TempDir(), "plugin-install-upload", 3*install.MaxSourceBytes+8<<20)
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		defer release()
		c.Request = c.Request.WithContext(run)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, install.MaxSourceBytes+16<<20)
		e = c.Request.ParseMultipartForm(8 << 20)
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if e != nil {
			dto.ResponseError(c, 400, "PLUGIN_INSTALL_UPLOAD_INVALID", nil)
			return
		}
		source, cleanup, e := prepareInstallationUpload(run, c.Request.MultipartForm)
		if cleanup != nil {
			defer cleanup()
		}
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		out, e := svc.AcceptSource(run, c.Param("task_id"), source)
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		dto.ResponseSuccess(c, out)
	}
}
func PluginInstallationEvents(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc := installationService(c, deps)
		if svc == nil {
			return
		}
		if c.Request.URL.RawQuery != "" {
			dto.ResponseError(c, 400, "PLUGIN_INSTALL_TASK_INVALID", nil)
			return
		}
		if _, e := svc.Get(c.Request.Context(), c.Param("task_id")); e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		if deps.EventFabric == nil || deps.EventFabric.RedisClient == nil {
			dto.ResponseError(c, 503, "PLUGIN_INSTALL_EVENTS_UNAVAILABLE", nil)
			return
		}
		deadline := time.Now().Add(time.Minute)
		claims := reqctx.GetClaims(c.Request.Context())
		if claims != nil && claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(deadline) {
			deadline = claims.ExpiresAt.Time
		}
		ctx, cancel := context.WithDeadline(c.Request.Context(), deadline)
		defer cancel()
		sub := deps.EventFabric.RedisClient.Subscribe(ctx, svc.EventChannel(reqctx.GetTenantUUID(ctx), c.Param("task_id")))
		defer sub.Close()
		subscribe, stop := context.WithTimeout(ctx, 5*time.Second)
		_, e := sub.Receive(subscribe)
		stop()
		if e != nil {
			dto.ResponseError(c, 503, "PLUGIN_INSTALL_EVENTS_UNAVAILABLE", nil)
			return
		}
		// Subscribe before the snapshot; receive messages only. Heartbeats do no DB IO.
		initial, e := svc.Get(ctx, c.Param("task_id"))
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		changes := make(chan []string, 1)
		problems := make(chan error, 1)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				_, e := sub.ReceiveMessage(ctx)
				if e != nil {
					select {
					case problems <- e:
					default:
					}
					return
				}
				select {
				case changes <- nil:
				default:
				}
			}
		}()
		defer func() { cancel(); sub.Close(); <-done }()
		first := true
		e = eventstream.ServeNotifications(ctx, c.Writer, changes, problems, func([]string) (eventstream.Event, error) {
			if first {
				first = false
				return eventstream.Event{Name: "status", Data: initial}, nil
			}
			out, e := svc.Get(ctx, c.Param("task_id"))
			return eventstream.Event{Name: "status", Data: out}, e
		})
		if e != nil && !c.Writer.Written() {
			dto.ResponseError(c, 503, "PLUGIN_INSTALL_EVENTS_UNAVAILABLE", nil)
		}
	}
}
