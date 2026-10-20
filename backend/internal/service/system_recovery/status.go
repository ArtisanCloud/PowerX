package system_recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"os"
)

func (c *Coordinator) StatusHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /__powerx_recovery/jobs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		id := r.PathValue("id")
		if len(r.URL.Query()) != 0 || !c.Store.VerifyProgressToken(id, token) {
			http.Error(w, "progress authorization required", http.StatusUnauthorized)
			return
		}
		if _, e := c.Store.Get(id); e != nil {
			http.Error(w, "job unavailable", http.StatusNotFound)
			return
		}
		sub, e := opswatch.Subscribe(r.Context(), map[string][]string{"job": {filepath.Join(c.Store.Directory, "jobs")}}, func(path string) bool { return filepath.Base(path) == id+".json" })
		if e != nil {
			http.Error(w, "progress notifications unavailable", http.StatusServiceUnavailable)
			return
		}
		defer sub.Close()
		_ = eventstream.Serve(r.Context(), w, sub, func([]string) (eventstream.Event, error) {
			if !c.Store.VerifyProgressToken(id, token) {
				return eventstream.Event{}, errors.New("progress ticket expired")
			}
			job, e := c.Store.Get(id)
			job.RequestHash = ""
			return eventstream.Event{Name: "progress", Data: map[string]any{"recovery": job}}, e
		})
	})
	mux.HandleFunc("GET /__powerx_recovery/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if len(r.URL.Query()) != 0 || !c.Store.VerifyProgressToken(r.PathValue("id"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
			http.Error(w, "progress authorization required", http.StatusUnauthorized)
			return
		}
		job, e := c.Store.Get(r.PathValue("id"))
		if e != nil {
			http.Error(w, "job unavailable", http.StatusNotFound)
			return
		}
		job.RequestHash = ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"recovery": job})
	})
	return mux
}
func (c *Coordinator) runWithStatus(ctx context.Context, address string) error {
	keyFile, e := c.EnsureRecoveryAccess()
	if e != nil {
		return e
	}
	fmt.Printf("独立恢复控制台已启用：%s（恢复管理员密钥文件：%s；密钥内容不会输出）\n", recoveryEntry, keyFile)
	stopGateways, e := c.startGateways(ctx)
	if e != nil {
		return e
	}
	defer stopGateways()
	host, _, e := net.SplitHostPort(address)
	if e != nil || host != "127.0.0.1" {
		return errors.New("status server must bind an explicit loopback port")
	}
	listener, e := net.Listen("tcp", address)
	if e != nil {
		return e
	}
	server := &http.Server{Handler: c.StatusHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(cleanup)
	}()
	go func() { _ = server.Serve(listener) }()
	err := c.Run(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, errCoordinatorAlreadyRunning) {
		return err
	}
	if err != nil {
		// Keep the recovery entrance available for authentication and diagnostics.
		// A failed executor may not accept destructive work until restarted.
		c.executorFailure.Store(err.Error())
		_ = snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "executor-error.json"), map[string]string{"error": err.Error()})
		_ = os.Remove(filepath.Join(c.Store.Directory, "heartbeat.json"))
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}
