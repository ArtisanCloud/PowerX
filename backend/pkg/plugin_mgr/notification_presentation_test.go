package plugin_mgr

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNotificationPresentationRejectsArbitraryRoutes(t *testing.T) {
	good := NotificationPresentationSpec{Contract: "business_notification_card.v1", BusinessType: "sample_track", PresentationPath: "/admin/notices/{delivery_key}/presentation", PreviewPath: "/admin/notices/{delivery_key}/images/{image_key}"}
	require.NoError(t, good.Validate())
	for _, path := range []string{"https://example.test/{delivery_key}", "//example.test/{delivery_key}", "/admin/../{delivery_key}", "/admin/%2e%2e/{delivery_key}", "/admin/{delivery_key}?token=abc", "/admin/{delivery_key}/{unknown}", "/admin/{delivery_key}/{delivery_key}", "/public/{delivery_key}"} {
		bad := good
		bad.PresentationPath = path
		require.Error(t, bad.Validate(), path)
	}
}

func TestNotificationPresentationV2KeepsRegisteredImageRoute(t *testing.T) {
	good := NotificationPresentationSpec{Contract: "business_notification_card.v2", BusinessType: "sample_track", PresentationPath: "/admin/notices/{delivery_key}/presentation-v2", PreviewPath: "/admin/notices/{delivery_key}/images/{image_key}"}
	require.NoError(t, good.Validate())
	good.Contract = "business_notification_card.v3"
	require.Error(t, good.Validate())
}
