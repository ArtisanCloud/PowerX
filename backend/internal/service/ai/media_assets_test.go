package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/stretchr/testify/require"
)

func TestMediaFetcherRejectsPrivateSupplierURLBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("private destination must not be requested")
	}))
	defer server.Close()
	s := &MediaRuntime{media: &mediasvc.MediaService{}, fetch: newMediaFetcher()}
	_, err := s.materialize(context.Background(), "tenant", "task", mediaProviderOutput{Kind: "video", URLs: []string{server.URL + "/clip.mp4"}})
	requireMediaCode(t, err, "AI_MEDIA_URL_INVALID")
	require.Error(t, publicMediaURL("https://user:password@example.com/output"))
}
