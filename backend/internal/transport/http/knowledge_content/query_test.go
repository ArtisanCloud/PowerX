package knowledge_content

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContentQueryRejectsProxyOverridesAndPreservesZero(t *testing.T) {
	space := uuid.NewString()
	in, err := Query(url.Values{"mode": {"hybrid"}, "preview_codepoints": {"0"}}, space)
	require.NoError(t, err)
	require.NotNil(t, in.PreviewCodepoints)
	require.Equal(t, 0, *in.PreviewCodepoints)
	for _, values := range []url.Values{{"mode": {"hybrid", "semantic"}}, {"tenant_uuid": {uuid.NewString()}}, {"endpoint": {"http://other"}}, {"page_size": {""}}} {
		_, err := Query(values, space)
		require.Error(t, err)
	}
}
