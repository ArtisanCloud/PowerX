package manager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func (m *managerImpl) PinHTTP(ctx context.Context, id, version, digest string) (plugin_mgr.PinnedHTTPIdentity, error) {
	p, err := m.Get(ctx, id)
	if err != nil || p.State != plugin_mgr.StateEnabled || p.Version != version || !p.Runtime.PinExecutable || m.sup == nil {
		return plugin_mgr.PinnedHTTPIdentity{}, supervisor.ErrUnpinnedProcess
	}
	proof, err := m.sup.Pin(id)
	if err != nil || proof.ImplementationSHA256 != digest {
		return plugin_mgr.PinnedHTTPIdentity{}, supervisor.ErrUnpinnedProcess
	}
	// 实际被 Supervisor 固定的入口必须来自当前已安装插件目录。
	root, rootErr := filepath.EvalSymlinks(p.Paths.Root)
	entry, entryErr := filepath.EvalSymlinks(proof.InstalledEntry)
	declared, declaredErr := filepath.EvalSymlinks(p.Paths.Entry)
	relative, relErr := filepath.Rel(root, entry)
	if rootErr != nil || entryErr != nil || declaredErr != nil || relErr != nil || !filepath.IsLocal(relative) || entry != declared {
		return plugin_mgr.PinnedHTTPIdentity{}, supervisor.ErrUnpinnedProcess
	}
	return plugin_mgr.PinnedHTTPIdentity{PluginID: id, Version: version, ImplementationSHA256: digest, Generation: proof.Generation}, nil
}

func (m *managerImpl) InvokePinnedHTTP(ctx context.Context, proof plugin_mgr.PinnedHTTPIdentity, in plugin_mgr.PinnedHTTPRequest) ([]byte, error) {
	actual, err := m.PinHTTP(ctx, proof.PluginID, proof.Version, proof.ImplementationSHA256)
	if err != nil || actual != proof {
		return nil, supervisor.ErrUnpinnedProcess
	}
	path, err := url.ParseRequestURI(in.Path)
	if err != nil || !strings.HasPrefix(in.Path, "/") || strings.HasPrefix(in.Path, "//") || path.RawQuery != "" || path.Fragment != "" || path.RawPath != "" || strings.ContainsAny(in.Path, "\\\r\n") || in.Method != http.MethodPost || len(in.Body) > 1<<20 {
		return nil, fmt.Errorf("plugin.tool_request_invalid")
	}
	for _, segment := range strings.Split(path.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("plugin.tool_request_invalid")
		}
	}
	pin, err := m.sup.Pin(proof.PluginID)
	if err != nil || pin.Generation != proof.Generation {
		return nil, supervisor.ErrUnpinnedProcess
	}
	var output []byte
	err = m.sup.WithPinnedProcess(ctx, proof.PluginID, pin, func(port int, token string) error {
		if token == "" {
			return fmt.Errorf("plugin.tool_transport_unavailable")
		}
		request, err := http.NewRequestWithContext(ctx, in.Method, "http://127.0.0.1:"+strconv.Itoa(port)+in.Path, bytes.NewReader(in.Body))
		if err != nil {
			return err
		}
		for key, value := range in.Headers {
			request.Header.Set(key, value)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-PowerX-Internal-Token", token)
		transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		output, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(output) > 1<<20 || response.StatusCode != http.StatusOK {
			return fmt.Errorf("plugin.tool_result_invalid")
		}
		return nil
	})
	return output, err
}
