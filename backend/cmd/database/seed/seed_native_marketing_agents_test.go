package seed

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedSkillPackageURIUsesConfiguredMediaDriver(t *testing.T) {
	uri, err := seedSkillPackageURI("local", "", "skill-sources/tenant/skill/source.tar.gz")
	require.NoError(t, err)
	require.Equal(t, "local://skill-sources/tenant/skill/source.tar.gz", uri)

	uri, err = seedSkillPackageURI("s3", "powerx-media", "skill-packages/tenant/skill/revision.tar.gz")
	require.NoError(t, err)
	require.Equal(t, "s3://powerx-media/skill-packages/tenant/skill/revision.tar.gz", uri)
}

func TestSeedSkillPackageURIRejectsInvalidConfiguredStorage(t *testing.T) {
	_, err := seedSkillPackageURI("s3", "", "skill-packages/tenant/skill/revision.tar.gz")
	require.ErrorContains(t, err, "seed_skill_package_s3_bucket_required")

	_, err = seedSkillPackageURI("unsupported", "", "skill-packages/tenant/skill/revision.tar.gz")
	require.ErrorContains(t, err, "seed_skill_package_storage_driver_unsupported")
}
