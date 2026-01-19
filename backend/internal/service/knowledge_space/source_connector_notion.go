package knowledge_space

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type NotionConnector struct {
	http *httpRetryClient
}

func NewNotionConnector(client *http.Client) *NotionConnector {
	return &NotionConnector{http: newHTTPRetryClient(client)}
}

func (c *NotionConnector) Fetch(ctx context.Context, req SourceFetchRequest) (SourceFetchResponse, error) {
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.notion.com"
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		// 无凭据时返回占位单元，避免整条链路空白；后续会由 remediation 指引安装/配置。
		return SourceFetchResponse{
			Units: []DocumentUnit{{
				Content: "notion connector: missing token (dev placeholder)",
				Provenance: map[string]any{
					"provider": "notion",
					"reason":   "missing_token",
				},
				Confidence: 0.2,
			}},
			HasMore: false,
		}, nil
	}

	scope := req.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	dbID := strings.TrimSpace(fmtAny(scope["databaseId"]))
	if dbID == "" {
		dbID = strings.TrimSpace(fmtAny(scope["database_id"]))
	}
	pageID := strings.TrimSpace(fmtAny(scope["pageId"]))
	if pageID == "" {
		pageID = strings.TrimSpace(fmtAny(scope["page_id"]))
	}
	if dbID != "" {
		since := strings.TrimSpace(fmtAny(scope["updatedSince"]))
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["updated_since"]))
		}
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["since"]))
		}
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["lastOkAt"]))
		}
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["last_ok_at"]))
		}
		return c.fetchDatabase(ctx, baseURL, token, dbID, req.Cursor, req.Limit, since)
	}
	if pageID != "" {
		return c.fetchPage(ctx, baseURL, token, pageID)
	}
	return SourceFetchResponse{
		Units: []DocumentUnit{{
			Content: "notion connector: missing scope (databaseId/pageId)",
			Provenance: map[string]any{
				"provider": "notion",
				"reason":   "missing_scope",
			},
			Confidence: 0.2,
		}},
		HasMore: false,
	}, nil
}

func (c *NotionConnector) fetchDatabase(ctx context.Context, baseURL, token, databaseID, cursor string, limit int, updatedSince string) (SourceFetchResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/databases/" + strings.TrimSpace(databaseID) + "/query"

	payload := map[string]any{
		"page_size": limit,
		"sorts": []map[string]any{{
			"timestamp": "last_edited_time",
			"direction": "ascending",
		}},
	}
	if strings.TrimSpace(cursor) != "" {
		payload["start_cursor"] = strings.TrimSpace(cursor)
	}
	if strings.TrimSpace(updatedSince) != "" {
		// best-effort incremental filter; if Notion rejects the filter, the retry client will surface a clear error.
		payload["filter"] = map[string]any{
			"timestamp": "last_edited_time",
			"last_edited_time": map[string]any{
				"on_or_after": strings.TrimSpace(updatedSince),
			},
		}
	}
	raw, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(raw)))
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Notion-Version", "2022-06-28")

	_, body, err := c.http.do(ctx, httpReq)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	m, err := jsonMap(body)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	results, _ := m["results"].([]any)
	units := make([]DocumentUnit, 0, len(results))
	for _, item := range results {
		content := extractPlainText(item, 4000)
		if strings.TrimSpace(content) == "" {
			content = "notion item (empty text)"
		}
		units = append(units, DocumentUnit{
			Content: content,
			Provenance: map[string]any{
				"provider":   "notion",
				"databaseId": databaseID,
				"cursor":     strings.TrimSpace(cursor),
			},
			Confidence: 0.7,
		})
	}
	hasMore, _ := m["has_more"].(bool)
	nextCursor := strings.TrimSpace(fmtAny(m["next_cursor"]))
	return SourceFetchResponse{
		Units:      units,
		HasMore:    hasMore && nextCursor != "",
		NextCursor: nextCursor,
	}, nil
}

func (c *NotionConnector) fetchPage(ctx context.Context, baseURL, token, pageID string) (SourceFetchResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/pages/" + strings.TrimSpace(pageID)
	httpReq, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Notion-Version", "2022-06-28")

	_, body, err := c.http.do(ctx, httpReq)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	m, err := jsonMap(body)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	content := extractPlainText(m, 8000)
	if strings.TrimSpace(content) == "" {
		content = "notion page (empty text)"
	}
	return SourceFetchResponse{
		Units: []DocumentUnit{{
			Content: content,
			Provenance: map[string]any{
				"provider": "notion",
				"pageId":   pageID,
			},
			Confidence: 0.7,
		}},
		HasMore: false,
	}, nil
}

func fmtAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}
