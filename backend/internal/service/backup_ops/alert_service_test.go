package backup_ops

import (
	"testing"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
)

func TestAlertLevelForConsecutiveFailures(t *testing.T) {
	if got := alertLevelForConsecutiveFailures(1); got != modelops.BackupAlertLevelMedium {
		t.Fatalf("expected medium for 1 consecutive failure, got %s", got)
	}
	if got := alertLevelForConsecutiveFailures(2); got != modelops.BackupAlertLevelHigh {
		t.Fatalf("expected high for 2 consecutive failures, got %s", got)
	}
	if got := alertLevelForConsecutiveFailures(5); got != modelops.BackupAlertLevelHigh {
		t.Fatalf("expected high for 5 consecutive failures, got %s", got)
	}
}
