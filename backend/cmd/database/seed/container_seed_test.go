package seed

import "testing"

func TestContainerDistributionNeverSeedsPublicDevelopmentKeys(t *testing.T) {
	t.Setenv("POWERX_MODE", "docker")
	if shouldSeedDevelopmentAPIKeys() {
		t.Fatal("container deployment must not seed public development keys")
	}
	t.Setenv("POWERX_MODE", "")
	if !shouldSeedDevelopmentAPIKeys() {
		t.Fatal("existing local development behavior must be preserved")
	}
}
