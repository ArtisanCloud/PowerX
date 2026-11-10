package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRejectMalformedCatalogAndUpstreamErrors(t *testing.T) {
	for _, body := range []string{`not json`, `{"code":0,"data":null}`, `{"code":500,"data":{"items":[]}}`, `{"code":0,"data":{"items":"bad"}}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer s.Close()
			_, err := NewClient(s.URL, "", s.Client()).List(context.Background(), "linux", "amd64", 1, 12)
			require.Error(t, err)
		})
	}
}

func TestUnconfiguredAndInvalidRoots(t *testing.T) {
	for _, root := range []string{"", "file:///tmp/plugins", "http://user:password@localhost", "http://localhost?token=secret"} {
		_, err := NewClient(root, "", http.DefaultClient).List(context.Background(), "linux", "amd64", 1, 12)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
	}
}
