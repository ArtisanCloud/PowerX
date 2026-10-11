package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/stretchr/testify/require"
)

func TestUpgradeCLIRejectsUnsafeArgumentsWithOneRedactedJSON(t *testing.T) {
	previous := commit
	commit = strings.Repeat("a", 40)
	t.Cleanup(func() { commit = previous })
	for _, test := range []struct {
		args   []string
		code   int
		reason string
	}{
		{[]string{"migrate", "--action=plan", "--source-sha=password=TOP_SECRET"}, 2, "UPGRADE_SOURCE_INVALID"},
		{[]string{"migrate", "--action=apply", "--target-release=sha-" + commit}, 2, "UPGRADE_OFFLINE_CONFIRMATION_REQUIRED"},
		{[]string{"migrate", "--action=plan", "--target-release=sha-" + strings.Repeat("b", 40)}, 2, "UPGRADE_TARGET_RELEASE_MISMATCH"},
		{[]string{"migrate", "--action=plan", "--lock-timeout=TOP_SECRET"}, 2, "UPGRADE_ARGUMENT_INVALID"},
		{[]string{"migrate", "--action=plan", "--target-release=sha-" + commit, "--config=/missing/config.yaml"}, 2, "UPGRADE_CONFIG_UNREADABLE"},
	} {
		var out, errOut bytes.Buffer
		handled, code := handleUpgradeCommand(test.args, &out, &errOut)
		require.True(t, handled)
		require.Equal(t, test.code, code)
		var result database.UpgradeResult
		require.NoError(t, json.Unmarshal(out.Bytes(), &result))
		require.Equal(t, test.reason, result.ErrorCode)
		require.NotContains(t, out.String()+errOut.String(), "TOP_SECRET")
		require.Equal(t, 1, strings.Count(out.String(), "\n"))
	}
	var out bytes.Buffer
	handled, _ := handleUpgradeCommand([]string{"migrate"}, &out, &out)
	require.False(t, handled)
	require.Empty(t, out.String())
}
