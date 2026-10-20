package system_recovery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"gopkg.in/yaml.v3"
)

// 固定入口由独立协调器持有。切换 Core/Node 时，页面仍可收到原任务进度。
func (c *Coordinator) startGateways(ctx context.Context) (func(), error) {
	native, ok := c.Driver.(*Native)
	if !ok {
		return func() {}, nil
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	var mu sync.Mutex
	servers := map[string]*http.Server{}
	closeAll := func() {
		cancelWatch()
		mu.Lock()
		defer mu.Unlock()
		for _, s := range servers {
			_ = s.Close()
		}
		servers = map[string]*http.Server{}
	}
	listen := func(address string, handler http.Handler) error {
		host, port, e := net.SplitHostPort(address)
		if e != nil || host != "127.0.0.1" || port == "0" {
			return errors.New("恢复入口必须使用预登记的本地端口")
		}
		listener, e := net.Listen("tcp", address)
		if e != nil {
			return e
		}
		s := &http.Server{Handler: handler, ReadHeaderTimeout: 5_000_000_000, MaxHeaderBytes: 1 << 20}
		mu.Lock()
		servers[address] = s
		mu.Unlock()
		go func() { _ = s.Serve(listener) }()
		return nil
	}
	for _, address := range []string{native.Profile.GatewayWebListen, native.Profile.GatewayBackendListen} {
		if address == "" {
			continue
		}
		if e := listen(address, http.HandlerFunc(c.gateway)); e != nil {
			closeAll()
			return nil, e
		}
	}
	syncPreviews := func() {
		jobs, e := c.Store.List()
		if e != nil {
			return
		}
		active, _ := c.Active()
		changing := active.Maintenance
		for _, job := range jobs {
			if (job.Status == "running" || job.Status == "queued") && (job.Kind == "cleanup" || job.Kind == "cutover" || job.Kind == "overwrite" || job.Kind == "rollback") {
				changing = true
			}
		}
		wanted := map[string]bool{native.Profile.GatewayWebListen: true, native.Profile.GatewayBackendListen: true}
		for _, j := range jobs {
			if changing || j.Kind != "prepare" || j.Status != "verified" || j.ValidationURL == "" {
				continue
			}
			var d Deployment
			if snapshots.ReadCoordinatorJSON(c.candidateFile(j.ID), &d) != nil || d.ID == active.Deployment.ID {
				continue
			}
			parsed, e := url.Parse(j.ValidationURL)
			if e != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() != strconv.Itoa(d.WebPort) {
				continue
			}
			wanted[parsed.Host] = true
			mu.Lock()
			_, exists := servers[parsed.Host]
			mu.Unlock()
			if exists {
				continue
			}
			var record processRecord
			if snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "processes/instance.json"), &record) == nil {
				identity, err := processIdentity(record.PID)
				if err != nil || identity != record.Started {
					if native.Start(ctx, d, true) != nil {
						continue
					}
				}
			} else {
				continue
			}
			candidate := d
			_ = listen(parsed.Host, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				a, e := c.Active()
				latest, je := c.Store.Get(candidate.ID)
				if e != nil || je != nil || latest.Status != "verified" || a.Deployment.ID == candidate.ID {
					http.Error(w, "验证环境已停用；请返回备份中心", http.StatusGone)
					return
				}
				if r.Header.Get("Upgrade") != "" || (r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && !(r.Method == "POST" && (strings.HasSuffix(r.URL.Path, "/admin/user/auth/login") || strings.HasSuffix(r.URL.Path, "/admin/user/auth/refresh") || strings.HasSuffix(r.URL.Path, "/admin/user/auth/logout")))) {
					http.Error(w, "恢复验证环境只读", http.StatusServiceUnavailable)
					return
				}
				proxyDeployment(w, r, candidate, true)
			}))
		}
		mu.Lock()
		for address, s := range servers {
			if !wanted[address] {
				_ = s.Close()
				delete(servers, address)
			}
		}
		mu.Unlock()
	}
	sub, e := opswatch.Subscribe(watchCtx, map[string][]string{"environments": {c.Store.Directory, filepath.Join(c.Store.Directory, "jobs")}}, func(path string) bool {
		return strings.HasPrefix(filepath.Base(path), "candidate-") || filepath.Base(path) == "active.json" || filepath.Dir(path) == filepath.Join(c.Store.Directory, "jobs")
	})
	if e != nil {
		closeAll()
		return nil, e
	}
	syncPreviews()
	go func() {
		defer sub.Close()
		for {
			select {
			case <-watchCtx.Done():
				closeAll()
				return
			case _, ok := <-sub.Changes:
				if !ok {
					return
				}
				syncPreviews()
			case <-sub.Errors:
				return
			}
		}
	}()
	return closeAll, nil
}
func (c *Coordinator) gateway(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/__powerx_recovery/jobs/") {
		c.StatusHandler().ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, recoveryEntry) {
		c.portal(w, r)
		return
	}
	a, e := c.Active()
	if e == nil && c.control(w, r, a) {
		return
	}
	if e != nil || a.Maintenance {
		if recoveryNavigation(r) {
			c.serveRecoveryPage(w, r)
			return
		}
		w.Header().Set("Retry-After", "5")
		http.Error(w, "当前站点正在恢复切换；恢复进度连接仍可用", http.StatusServiceUnavailable)
		return
	}
	if recoveryNavigation(r) && !c.businessOnline(r.Context(), a.Deployment) {
		c.serveRecoveryPage(w, r)
		return
	}
	proxyDeploymentWithFailure(w, r, a.Deployment, false, func(w http.ResponseWriter, r *http.Request) {
		if recoveryNavigation(r) {
			c.serveRecoveryPage(w, r)
			return
		}
		http.Error(w, "业务服务尚未就绪；恢复控制台仍可使用", 503)
	})
}
func recoveryNavigation(r *http.Request) bool {
	return (r.Method == "GET" || r.Method == "HEAD") && strings.Contains(r.Header.Get("Accept"), "text/html")
}
func proxyDeployment(w http.ResponseWriter, r *http.Request, d Deployment, preview bool) {
	proxyDeploymentWithFailure(w, r, d, preview, nil)
}
func proxyDeploymentWithFailure(w http.ResponseWriter, r *http.Request, d Deployment, preview bool, onFailure http.HandlerFunc) {
	file := filepath.Join(d.Root, "launch.yaml")
	if d.SourceManaged || d.ConfigFile == filepath.Join(d.Root, "etc/config.yaml") {
		file = d.ConfigFile
	}
	raw, e := os.ReadFile(file)
	var cfg config.Config
	if e != nil || yaml.Unmarshal(raw, &cfg) != nil {
		http.Error(w, "恢复环境配置不可读取", 503)
		return
	}
	prefix := "/" + strings.Trim(cfg.Server.APIPrefix, "/")
	wsPrefix := strings.TrimRight(cfg.Server.WSPrefix, "/")
	if wsPrefix == "" {
		wsPrefix = "/wx"
	}
	core := r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") || strings.HasPrefix(r.URL.Path, strings.TrimRight(cfg.Plugin.BasePrefix, "/")+"/") || r.URL.Path == "/api/ws" || r.URL.Path == wsPrefix || strings.HasPrefix(r.URL.Path, wsPrefix+"/") || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/media/")
	target := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(cfg.WebAdminPort)}
	socket := ""
	var spec RunSpec
	isolatedWeb := !d.SourceManaged && snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "run.json"), &spec) == nil && spec.Deployment.Root == d.Root && spec.Isolated
	if core {
		target.Host = "127.0.0.1:" + strconv.Itoa(cfg.Server.Port)
		if !d.SourceManaged {
			socket = filepath.Join(d.Root, "core.sock")
		}
	} else if preview || isolatedWeb {
		socket = filepath.Join(d.Root, "web.sock")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	transport := &http.Transport{DisableKeepAlives: true}
	if socket != "" {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}
	}
	proxy.Transport = transport
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		if onFailure != nil {
			onFailure(w, r)
			return
		}
		http.Error(w, "恢复环境尚未就绪，请返回备份中心查看任务", 503)
	}
	defer transport.CloseIdleConnections()
	proxy.ServeHTTP(w, r)
}
