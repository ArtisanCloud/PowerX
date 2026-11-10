package connectionprobe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func assertFailure(t *testing.T, err error, status int, code string) *dto.AppError {
	t.Helper()
	var app *dto.AppError
	require.True(t, errors.As(err, &app), "expected structured failure: %v", err)
	require.Equal(t, status, app.HTTPCode)
	require.Equal(t, code, app.Code)
	require.Equal(t, false, app.Details["generation_submitted"])
	return app
}

func TestVerifyReadOnlyAuthentication(t *testing.T) {
	for _, strategy := range []string{Models, KeyInfo} {
		t.Run(strategy, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, int64(0), r.ContentLength)
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				path := "/v1/models"
				if strategy == KeyInfo {
					path = "/v1/auth/key"
				}
				require.Equal(t, path, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer test-valid-key" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if strategy == KeyInfo {
					_, _ = w.Write([]byte(`{"data":{"label":"test"}}`))
				} else {
					_, _ = w.Write([]byte(`{"data":[]}`))
				}
			}))
			defer srv.Close()
			result, err := Verify(context.Background(), Input{Provider: "example", Endpoint: srv.URL + "/v1/", APIKey: "test-valid-key", Strategy: strategy})
			require.NoError(t, err)
			require.EqualValues(t, 2, requests.Load())
			require.True(t, result.EndpointReachable)
			require.True(t, result.APIKeyVerified)
			require.Equal(t, "api_key", result.Authentication)
			require.False(t, result.ModelAccessVerified)
			require.False(t, result.GenerationSubmitted)
		})
	}
}

func TestVerifyRejectsUnverifiableAuthentication(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	_, err := Verify(context.Background(), Input{Endpoint: srv.URL, APIKey: "any-key", Strategy: Models})
	assertFailure(t, err, 501, "AI_CONNECTION_AUTH_UNVERIFIABLE")
	require.EqualValues(t, 2, requests.Load())
}

func TestVerifyStructuredUpstreamFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		code       string
		coreStatus int
	}{
		{"invalid key", 401, `{"error":"SECRET upstream diagnostic"}`, "AI_CONNECTION_AUTH_FAILED", 422},
		{"forbidden key", 403, `{"error":"SECRET"}`, "AI_CONNECTION_AUTH_FAILED", 422},
		{"not supported", 404, `not found`, "AI_CONNECTION_PROBE_UNSUPPORTED", 501},
		{"wrong method", 405, ``, "AI_CONNECTION_PROBE_UNSUPPORTED", 501},
		{"throttled", 429, ``, "AI_CONNECTION_RATE_LIMITED", 429},
		{"server failure", 500, `SECRET`, "AI_CONNECTION_UPSTREAM_FAILED", 502},
		{"html instead of API", 200, `<html>login</html>`, "AI_CONNECTION_RESPONSE_INVALID", 502},
		{"missing data", 200, `{}`, "AI_CONNECTION_RESPONSE_INVALID", 502},
		{"null data", 200, `{"data":null}`, "AI_CONNECTION_RESPONSE_INVALID", 502},
		{"wrong shape", 200, `{"data":{}}`, "AI_CONNECTION_RESPONSE_INVALID", 502},
		{"oversize", 200, strings.Repeat("x", maxResponseBytes+1), "AI_CONNECTION_RESPONSE_INVALID", 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				require.Equal(t, http.MethodGet, r.Method)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := Verify(context.Background(), Input{Endpoint: srv.URL, APIKey: "SECRET", Strategy: Models})
			app := assertFailure(t, err, tc.coreStatus, tc.code)
			require.Equal(t, tc.status, app.Details["upstream_status"])
			serialized, _ := json.Marshal(app)
			require.NotContains(t, string(serialized), "SECRET")
			require.NotContains(t, string(serialized), srv.URL)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

func TestVerifyDoesNotFollowRedirects(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	_, err := Verify(context.Background(), Input{Endpoint: srv.URL, APIKey: "SECRET", Strategy: Models})
	assertFailure(t, err, 502, "AI_CONNECTION_REDIRECT_REJECTED")
	require.Zero(t, targetCalls.Load())
}

func TestVerifyDeadline(t *testing.T) {
	for _, stage := range []string{"headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "body" {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			_, err := Verify(ctx, Input{Endpoint: srv.URL, APIKey: "key", Strategy: Models})
			assertFailure(t, err, 504, "AI_CONNECTION_TIMEOUT")
		})
	}
}

func TestVerifyOllamaDoesNotClaimKeyVerification(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/tags", r.URL.Path)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()
	for _, suffix := range []string{"", "/v1", "/api"} {
		result, err := Verify(context.Background(), Input{Endpoint: srv.URL + suffix, Strategy: OllamaTags})
		require.NoError(t, err)
		require.Equal(t, "not_required", result.Authentication)
		require.False(t, result.APIKeyVerified)
	}
	require.EqualValues(t, 3, requests.Load())
}

func TestVerifyInvalidConfigurationDoesNotRequest(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer srv.Close()
	for _, endpoint := range []string{"", "ftp://invalid", srv.URL + "?key=SECRET", srv.URL + "#fragment", "http://user:pass@localhost"} {
		_, err := Verify(context.Background(), Input{Endpoint: endpoint, APIKey: "key", Strategy: Models})
		assertFailure(t, err, 422, "AI_CONNECTION_CONFIG_INVALID")
	}
	_, err := Verify(context.Background(), Input{Endpoint: srv.URL, APIKey: "key", Strategy: "unknown"})
	assertFailure(t, err, 501, "AI_CONNECTION_PROBE_UNSUPPORTED")
	_, err = Verify(context.Background(), Input{Endpoint: srv.URL, Strategy: Models})
	assertFailure(t, err, 422, "AI_CONNECTION_CONFIG_INVALID")
	require.Zero(t, requests.Load())
}
