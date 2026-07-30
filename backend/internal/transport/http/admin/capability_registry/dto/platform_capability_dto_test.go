package capability_registrydto

import (
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestPlatformCapabilityToDTODerivesPluginModuleFromPluginID(t *testing.T) {
	dto := PlatformCapabilityToDTO(&models.CapabilityRecord{
		CapabilityID:     "com.powerx.plugins.mediax-studio.local.creation.video_rebuilder.prepare",
		PluginID:         "com.powerx.plugins.mediax-studio.local",
		PluginVersion:    "0.1.0",
		Title:            "com.powerx.plugins.mediax-studio.local.creation.video_rebuilder.prepare",
		CapabilitiesHash: "hash",
		ProtocolHash:     "protocol",
		Status:           "published",
		Annotations:      []byte(`{}`),
		Protocols:        []byte(`[{"channel":"rest"}]`),
	})

	require.Equal(t, "mediax_studio", dto.Module)
	require.Equal(t, "plugin", dto.Source)
}

func TestPlatformCapabilityToDTODerivesBasePluginModule(t *testing.T) {
	dto := PlatformCapabilityToDTO(&models.CapabilityRecord{
		CapabilityID:     "com.powerx.plugins.base.local.template.create",
		PluginID:         "com.powerx.plugins.base.local",
		PluginVersion:    "0.1.0",
		Title:            "创建模板",
		CapabilitiesHash: "hash",
		ProtocolHash:     "protocol",
		Status:           "published",
		Annotations:      []byte(`{}`),
		Protocols:        []byte(`[{"channel":"rest"}]`),
	})

	require.Equal(t, "base", dto.Module)
	require.Equal(t, "plugin", dto.Source)
}
