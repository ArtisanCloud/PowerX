package knowledge_space

import (
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContentCursorIntegrityExpiryAndQueryValues(t *testing.T) {
	s := &ContentOverviewService{signingKey: []byte(strings.Repeat("test-key", 4)), clock: func() time.Time { return time.Unix(1000, 0) }}
	c := contentCursor{Version: 1, Parameters: "parameters", Actor: "credential", Generation: uuid.NewString(), Fingerprint: "visibility", After: uuid.NewString(), ExpiresAt: 1100}
	encoded := s.encodeCursor(c)
	decoded, err := s.decodeCursor(encoded)
	require.NoError(t, err)
	require.Equal(t, c, decoded)
	parts := strings.Split(encoded, ".")
	_, err = s.decodeCursor(parts[0] + "." + strings.Repeat("A", len(parts[1])))
	require.Equal(t, "KNOWLEDGE_CONTENT_CURSOR_INVALID", dto.CodeOf(err))
	s.clock = func() time.Time { return time.Unix(1100, 0) }
	_, err = s.decodeCursor(encoded)
	require.Equal(t, 409, dto.StatusCode(err))
	require.Equal(t, "KNOWLEDGE_CONTENT_CURSOR_EXPIRED", dto.CodeOf(err))
	zero, size := 0, 2
	query := ContentOverviewQuery{Schema: ContentOverviewQuerySchema, SpaceUUID: uuid.NewString(), Mode: HybridMode, PageSize: &size, PreviewCodepoints: &zero}
	_, page, preview, err := normalizeContentQuery(query)
	require.NoError(t, err)
	require.Equal(t, 2, page)
	require.Equal(t, 0, preview)
	query.PageSize = &zero
	_, _, _, err = normalizeContentQuery(query)
	require.Equal(t, 400, dto.StatusCode(err))
}
