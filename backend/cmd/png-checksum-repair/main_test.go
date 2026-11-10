package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPNGRepairRejectsAmbiguousArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--report", "x"}, {"--apply", "--plan", "x", "--report", "y"}, {"--apply", "--confirm", "--plan", "x", "--tenant", "other", "--report", "y"}, {"--confirm", "--tenant", "x", "--asset", "y", "--report", "z"}} {
		require.ErrorContains(t, run(args), "MEDIA_PNG_REPAIR_ARGUMENTS_INVALID")
	}
}
