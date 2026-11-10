package mediacontract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestThumbnail64SharedExamplesMatchPublishedSchemas(t *testing.T) {
	root := repoRootFromHere(t)
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	media, err := loader.LoadFromFile(filepath.Join(root, "specs/001-media-storage/contracts/image-derivatives.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, media.Validate(context.Background()))
	raw, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/media-image-derivatives.v2.json"))
	require.NoError(t, err)
	var examples map[string]any
	require.NoError(t, json.Unmarshal(raw, &examples))
	for _, key := range []string{"ready", "partial_processing", "partial_failed"} {
		require.NoError(t, media.Components.Schemas["ImageDerivatives"].Value.VisitJSON(examples[key]), key)
	}
	ready := examples["ready"].(map[string]any)
	items := ready["items"].([]any)
	first := items[0].(map[string]any)
	saved := first["variant_uuid"]
	delete(first, "variant_uuid")
	require.Error(t, media.Components.Schemas["ImageDerivatives"].Value.VisitJSON(ready), "ready without a UUID is invalid")
	first["variant_uuid"] = saved
	items[0], items[2] = items[2], items[0]
	require.NoError(t, media.Components.Schemas["ImageDerivatives"].Value.VisitJSON(ready), "order is not part of the contract")
	cards, err := loader.LoadFromFile(filepath.Join(root, "docs/contracts/business-notification-card.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, cards.Validate(context.Background()))
	for _, version := range []string{"v1", "v2"} {
		raw, err = os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/business-notification-card."+version+".json"))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &examples))
		require.NoError(t, cards.Components.Schemas["Presentation"].Value.VisitJSON(examples["presentation"]), version)
		require.NoError(t, cards.Components.Schemas["InvalidateCardOperation"].Value.VisitJSON(examples["invalidate"]), version)
		if version == "v2" {
			require.NoError(t, cards.Components.Schemas["Presentation"].Value.VisitJSON(examples["presentation_pending"]))
			require.NoError(t, cards.Components.Schemas["PresentationBinding"].Value.VisitJSON(examples["presentation_binding"]))
			require.NoError(t, cards.Components.Schemas["InvalidateCardOperation"].Value.VisitJSON(examples["upgrade_v1_reference"]))
		}
	}
}
