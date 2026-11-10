package modeldiscovery

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelDiscoveryReadsImageInputsAndStructuredCapabilities(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"native-vision","input_modalities":["image","text"],"output_modalities":["text"]}]}`,
		`{"data":[{"id":"native-vision","architecture":{"input_modalities":["image","text"],"output_modalities":["text"]}}]}`,
		`{"data":[{"id":"native-vision","capabilities":{"image_input":{"supported":true}}}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		result, err := Fetch(context.Background(), Input{Provider: "openai", Endpoint: srv.URL + "/v1", APIKey: "fixture"})
		srv.Close()
		require.NoError(t, err)
		require.Contains(t, result.Models[0].Modalities, "vlm")
	}
}
