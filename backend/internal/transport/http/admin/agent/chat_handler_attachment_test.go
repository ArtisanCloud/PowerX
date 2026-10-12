package agent

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAttachmentUUIDStringsAcceptsPersistedJSONArrayForms(t *testing.T) {
	first := uuid.NewString()
	second := uuid.NewString()

	fromStrings, err := attachmentUUIDStrings([]string{first, second})
	require.NoError(t, err)
	require.Equal(t, []string{first, second}, fromStrings)

	fromJSON, err := attachmentUUIDStrings([]interface{}{first, second})
	require.NoError(t, err)
	require.Equal(t, []string{first, second}, fromJSON)
}

func TestAttachmentUUIDStringsRejectsNonStringJSONValues(t *testing.T) {
	_, err := attachmentUUIDStrings([]interface{}{uuid.NewString(), 1})
	require.Error(t, err)

	_, err = attachmentUUIDStrings(uuid.NewString())
	require.Error(t, err)
}
