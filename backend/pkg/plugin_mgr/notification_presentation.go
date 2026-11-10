package plugin_mgr

import (
	"fmt"
	"regexp"
	"strings"
)

// Routes are plugin-relative read-only Admin API paths, registered at install.
// The Gateway supplies the current member identity and evaluates plugin RBAC.
type NotificationPresentationSpec struct {
	BusinessType     string `yaml:"business_type" json:"business_type"`
	Contract         string `yaml:"contract" json:"contract"`
	PresentationPath string `yaml:"presentation_path" json:"presentation_path"`
	PreviewPath      string `yaml:"preview_path" json:"preview_path"`
}

var PresentationToken = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,256}$`)

func (s NotificationPresentationSpec) Validate() error {
	if !PresentationToken.MatchString(s.BusinessType) || !IsBusinessCardContract(s.Contract) {
		return fmt.Errorf("invalid notification presentation contract")
	}
	for _, p := range []string{s.PresentationPath, s.PreviewPath} {
		if !strings.HasPrefix(p, "/admin/") || strings.ContainsAny(p, "?%#\\\r\n") || strings.Contains(p, "//") || strings.Count(p, "{delivery_key}") != 1 {
			return fmt.Errorf("invalid notification presentation path")
		}
		q := strings.ReplaceAll(strings.ReplaceAll(p, "{delivery_key}", "key"), "{image_key}", "image")
		for _, part := range strings.Split(q, "/")[1:] {
			if !PresentationToken.MatchString(part) {
				return fmt.Errorf("invalid notification presentation segment")
			}
		}
	}
	if strings.Contains(s.PresentationPath, "{image_key}") || strings.Count(s.PreviewPath, "{image_key}") != 1 {
		return fmt.Errorf("invalid notification image binding")
	}
	return nil
}

// Both versions are registered explicitly; v1 never acquires v2 image fields.
func IsBusinessCardContract(contract string) bool {
	return contract == "business_notification_card.v1" || contract == "business_notification_card.v2"
}
