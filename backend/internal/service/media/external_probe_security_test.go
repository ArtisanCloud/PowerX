package media

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
	"github.com/stretchr/testify/require"
)

func TestGuardedProbeDialAddresses(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:80", "10.0.0.1:80", "172.16.0.1:80", "192.168.1.1:80",
		"169.254.169.254:80", "255.255.255.255:80", "0.0.0.0:80", "0.1.2.3:80", "100.64.0.1:80", "100.127.255.254:80", "224.0.0.1:80",
		"[::1]:80", "[::]:80", "[fc00::1]:80", "[fd00::1]:80", "[fe80::1]:80", "[ff02::1]:80",
		"[::ffff:127.0.0.1]:80", "[::ffff:192.168.1.1]:80", "[fe80::1%lo0]:80", "unresolved.invalid:80", "invalid",
	} {
		t.Run(address, func(t *testing.T) { require.Error(t, guardedDialControl("tcp", address, nil)) })
	}
	for _, address := range []string{"8.8.8.8:443", "1.1.1.1:80", "[2606:4700:4700::1111]:443", "[::ffff:8.8.8.8]:443"} {
		t.Run(address, func(t *testing.T) { require.NoError(t, guardedDialControl("tcp", address, nil)) })
	}
}

func TestExternalProbeDefaultServiceBlocksLoopbackWithoutContact(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	repo := newStubAssetRepo()
	svc := NewMediaService(nil, repo, nil, &stubAuditService{}, time.Hour)
	_, err := svc.CreateAsset(context.Background(), CreateAssetInput{TenantUUID: mediaTenantUUID, Name: "blocked", Driver: "local", UploadMethod: UploadMethodExternalLink, ExternalURL: server.URL + "/asset"})
	require.ErrorContains(t, err, "non-public address blocked")
	require.EqualValues(t, 0, hits.Load())
	require.Equal(t, 0, repo.createCalls)
}

// Only the initial public hop is mapped to a local fixture. Real redirect targets
// retain the production DialContext and its socket-time address guard.
func publicFixtureClient(t *testing.T, fixture string) *http.Client {
	t.Helper()
	client := newGuardedHTTPClient(time.Second)
	transport := client.Transport.(*http.Transport)
	original := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host != "public.invalid" {
			return original(ctx, network, address)
		}
		if err := guardedDialControl(network, net.JoinHostPort("8.8.8.8", port), nil); err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(fixture, "http://"))
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func TestGuardedProbeRedirectToPrivateIsBlocked(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	var internalHits atomic.Int64
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { internalHits.Add(1); w.WriteHeader(200) }))
	defer internal.Close()
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/private", http.StatusFound)
	}))
	defer public.Close()
	_, err := publicFixtureClient(t, public.URL).Get("http://public.invalid/redirect")
	require.ErrorContains(t, err, "non-public address blocked")
	require.EqualValues(t, 0, internalHits.Load())
}

func TestGuardedProbeRedirectPolicy(t *testing.T) {
	for _, mode := range []string{"loop", "scheme"} {
		t.Run(mode, func(t *testing.T) {
			testutil.SkipIfNoLocalListener(t)
			var hits atomic.Int64
			public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				target := "/again"
				if mode == "scheme" {
					target = "ftp://public.invalid/asset"
				}
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer public.Close()
			_, err := publicFixtureClient(t, public.URL).Get("http://public.invalid/start")
			if mode == "loop" {
				require.ErrorContains(t, err, "too many redirects")
				require.EqualValues(t, 5, hits.Load())
			} else {
				require.ErrorContains(t, err, "unsupported redirect scheme")
				require.EqualValues(t, 1, hits.Load())
			}
		})
	}
}

func TestGuardedProbeResolvedAddressChangesBetweenHeadAndGet(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	var requests, dials atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	client := newGuardedHTTPClient(time.Second)
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		// Simulate the same hostname resolving publicly, then to loopback.
		ip := "8.8.8.8"
		if dials.Add(1) > 1 {
			ip = "127.0.0.1"
		}
		if err := guardedDialControl(network, net.JoinHostPort(ip, "80"), nil); err != nil {
			return nil, err
		}
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(server.URL, "http://"))
	}
	_, _, err := probeExternalObjectWithClient(context.Background(), "http://changing.invalid/asset", client)
	require.ErrorContains(t, err, "non-public address blocked")
	require.EqualValues(t, 1, requests.Load())
	require.GreaterOrEqual(t, dials.Load(), int64(2))
}

func TestExternalProbeHeadFallbackPreservesRangeAndMetadata(t *testing.T) {
	testutil.SkipIfNoLocalListener(t)
	var head, get atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			head.Add(1)
			w.WriteHeader(405)
			return
		}
		get.Add(1)
		if got, want := r.Header.Get("Range"), fmt.Sprintf("bytes=0-%d", externalProbeRangeSize-1); got != want {
			t.Errorf("range = %q, want %q", got, want)
		}
		w.Header().Set("Content-Range", "bytes 0-3/2048")
		w.Header().Set("Content-Type", "image/png; charset=utf-8")
		w.WriteHeader(206)
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer server.Close()
	size, mime, err := probeExternalObjectWithClient(context.Background(), server.URL+"/asset", server.Client())
	require.NoError(t, err)
	require.EqualValues(t, 2048, size)
	require.Equal(t, "image/png", mime)
	require.EqualValues(t, 1, head.Load())
	require.EqualValues(t, 1, get.Load())
}
