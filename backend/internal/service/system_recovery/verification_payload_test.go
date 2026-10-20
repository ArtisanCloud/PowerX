package system_recovery

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBusinessProbeRejectsSuccessfulHTTPErrorEnvelopes(t *testing.T) {
	for _, body := range []string{`null`, `not-json`, `{"success":false,"data":[]}`, `{"code":403,"data":{}}`, `{"error":{"code":"IAM_UPSTREAM_DEPENDENCY"}}`, `{"data":null}`} {
		require.False(t, validBusinessProbePayload([]byte(body)), body)
	}
	for _, body := range []string{`{"success":true,"data":{"items":[]}}`, `{"code":200,"data":[],"error":null}`, `{"code":0,"data":{"items":[{"name":"fixture"}]}}`, `[]`} {
		require.True(t, validBusinessProbePayload([]byte(body)), body)
	}
}
