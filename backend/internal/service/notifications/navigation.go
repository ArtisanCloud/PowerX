package notifications

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/datatypes"
)

// Navigation is an optional same-origin business entry, not an authorization or
// a business read receipt. The destination must authorize the current member.
type Navigation struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

type Action struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Type   string            `json:"type"`
	Action string            `json:"action"`
	Params map[string]string `json:"params"`
}

func NavigationActions(n *Navigation) (datatypes.JSON, error) {
	if n == nil {
		return nil, nil
	}
	invalid := func() (datatypes.JSON, error) {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, "NOTIFICATION_NAVIGATION_INVALID", "notification.navigation_invalid", nil)
	}
	if n.Label == "" || strings.TrimSpace(n.Label) != n.Label || len([]rune(n.Label)) > 80 || len(n.Path) > 2048 || strings.TrimSpace(n.Path) != n.Path {
		return invalid()
	}
	for _, r := range n.Label + n.Path {
		if unicode.IsControl(r) {
			return invalid()
		}
	}
	u, err := url.ParseRequestURI(n.Path)
	if err != nil || !strings.HasPrefix(n.Path, "/") || strings.HasPrefix(n.Path, "//") || u.IsAbs() || u.Host != "" || u.Fragment != "" || strings.ContainsAny(n.Path, "\\#") {
		return invalid()
	}
	decoded := u.Path
	if strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\%") {
		return invalid()
	}
	for _, r := range decoded {
		if r <= 0x20 || unicode.IsControl(r) {
			return invalid()
		}
	}
	for _, part := range strings.Split(decoded, "/") {
		if part == "." || part == ".." {
			return invalid()
		}
	}
	raw, err := json.Marshal([]Action{{ID: "view_business", Label: n.Label, Type: "primary", Action: "navigate", Params: map[string]string{"path": n.Path}}})
	return datatypes.JSON(raw), err
}
