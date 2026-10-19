package notifications

import (
	"encoding/json"
	"strings"

	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
)

// CreatedEvent uses the persisted identity and member target, so consumers can
// open the same record through the existing member-scoped notification API.
func CreatedEvent(item *nm.Notification) map[string]any {
	body := map[string]any{
		"id": item.UUID.String(), "title": item.Title, "content": item.Content,
		"type": item.Type, "category": item.Category,
		"isRead": item.IsRead, "isImportant": item.IsImportant,
		"createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt,
		"userId":    strings.TrimSpace(item.MemberUUID),
		"relatedId": item.RelatedID, "relatedType": item.RelatedType,
	}
	if len(item.Actions) > 0 {
		var actions []any
		if json.Unmarshal(item.Actions, &actions) == nil {
			body["actions"] = actions
		}
	}
	return body
}
