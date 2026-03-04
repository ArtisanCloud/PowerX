package capability_registry

import (
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
)

func TestRecordMatchesFilters_Source(t *testing.T) {
	t.Parallel()

	corexRecord := models.CapabilityRecord{
		CapabilityID: "corex.capability.a",
		PluginID:     "corex.platform",
	}
	pluginRecord := models.CapabilityRecord{
		CapabilityID: "plugin.capability.b",
		PluginID:     "com.powerx.plugins.base.template",
	}

	// source empty means no source filtering (equivalent to source=all/any).
	if !recordMatchesFilters(corexRecord, CapabilityListOptions{Source: ""}) {
		t.Fatal("expected corex record to pass when source filter is empty")
	}
	if !recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: ""}) {
		t.Fatal("expected plugin record to pass when source filter is empty")
	}

	if !recordMatchesFilters(corexRecord, CapabilityListOptions{Source: CapabilitySourceCoreX}) {
		t.Fatal("expected corex record to pass corex filter")
	}
	if recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: CapabilitySourceCoreX}) {
		t.Fatal("expected plugin record to be filtered out by corex filter")
	}
	if !recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: CapabilitySourcePlugin}) {
		t.Fatal("expected plugin record to pass plugin filter")
	}
	if recordMatchesFilters(corexRecord, CapabilityListOptions{Source: CapabilitySourcePlugin}) {
		t.Fatal("expected corex record to be filtered out by plugin filter")
	}
}
