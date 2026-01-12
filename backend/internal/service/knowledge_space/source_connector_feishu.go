package knowledge_space

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FeishuConnector struct {
	http *httpRetryClient
}

func NewFeishuConnector(client *http.Client) *FeishuConnector {
	return &FeishuConnector{http: newHTTPRetryClient(client)}
}

func (c *FeishuConnector) Fetch(ctx context.Context, req SourceFetchRequest) (SourceFetchResponse, error) {
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = "https://open.feishu.cn"
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SourceFetchResponse{
			Units: []DocumentUnit{{
				Content: "feishu connector: missing token (dev placeholder)",
				Provenance: map[string]any{
					"provider": "feishu",
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
	docToken := strings.TrimSpace(fmtAny(scope["docToken"]))
	if docToken == "" {
		docToken = strings.TrimSpace(fmtAny(scope["doc_token"]))
	}
	if docToken != "" {
		return c.fetchDoc(ctx, baseURL, token, docToken)
	}

	wikiSpaceID := strings.TrimSpace(fmtAny(scope["wikiSpaceId"]))
	if wikiSpaceID == "" {
		wikiSpaceID = strings.TrimSpace(fmtAny(scope["wiki_space_id"]))
	}
	if wikiSpaceID == "" {
		wikiSpaceID = strings.TrimSpace(fmtAny(scope["spaceId"]))
	}
	if wikiSpaceID == "" {
		wikiSpaceID = strings.TrimSpace(fmtAny(scope["space_id"]))
	}
	folderToken := strings.TrimSpace(fmtAny(scope["folderToken"]))
	if folderToken == "" {
		folderToken = strings.TrimSpace(fmtAny(scope["folder_token"]))
	}
	if wikiSpaceID != "" {
		since := strings.TrimSpace(fmtAny(scope["updatedSince"]))
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["updated_since"]))
		}
		if since == "" {
			since = strings.TrimSpace(fmtAny(scope["since"]))
		}
		return c.fetchWikiNodes(ctx, baseURL, token, wikiSpaceID, folderToken, req.Cursor, req.Limit, since)
	}

	return SourceFetchResponse{
		Units: []DocumentUnit{{
			Content: "feishu connector: missing scope (docToken/wikiSpaceId)",
			Provenance: map[string]any{
				"provider": "feishu",
				"reason":   "missing_scope",
			},
			Confidence: 0.2,
		}},
		HasMore: false,
	}, nil
}

func (c *FeishuConnector) fetchDoc(ctx context.Context, baseURL, token, docToken string) (SourceFetchResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/open-apis/docx/v1/documents/" + strings.TrimSpace(docToken)
	httpReq, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	httpReq.Header.Set("Authorization", "Bearer "+token)

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
		content = "feishu doc (empty text)"
	}
	return SourceFetchResponse{
		Units: []DocumentUnit{{
			Content: content,
			Provenance: map[string]any{
				"provider": "feishu",
				"docToken": docToken,
			},
			Confidence: 0.7,
		}},
		HasMore: false,
	}, nil
}

func (c *FeishuConnector) fetchWikiNodes(ctx context.Context, baseURL, token, wikiSpaceID, parentNodeToken, cursor string, limit int, updatedSince string) (SourceFetchResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	query := url.Values{}
	query.Set("page_size", strconv.Itoa(limit))
	if strings.TrimSpace(cursor) != "" {
		query.Set("page_token", strings.TrimSpace(cursor))
	}
	if strings.TrimSpace(parentNodeToken) != "" {
		query.Set("parent_node_token", strings.TrimSpace(parentNodeToken))
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/open-apis/wiki/v2/spaces/" + strings.TrimSpace(wikiSpaceID) + "/nodes?" + query.Encode()

	httpReq, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	httpReq.Header.Set("Authorization", "Bearer "+token)

	_, body, err := c.http.do(ctx, httpReq)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	root, err := jsonMap(body)
	if err != nil {
		return SourceFetchResponse{}, err
	}
	data, _ := root["data"].(map[string]any)
	items, _ := data["items"].([]any)

	since := parseRFC3339(updatedSince)
	units := make([]DocumentUnit, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		objType := strings.TrimSpace(fmtAny(m["obj_type"]))
		objToken := strings.TrimSpace(fmtAny(m["obj_token"]))
		title := strings.TrimSpace(fmtAny(m["title"]))
		editTime := parseUnixSeconds(fmtAny(m["obj_edit_time"]))
		if since != nil && editTime != nil && editTime.Before(*since) {
			continue
		}
		content := strings.TrimSpace(title)
		if objType == "docx" && objToken != "" {
			if docResp, err := c.fetchDoc(ctx, baseURL, token, objToken); err == nil && len(docResp.Units) > 0 {
				content = strings.TrimSpace(docResp.Units[0].Content)
			}
		}
		if content == "" {
			content = fmt.Sprintf("feishu wiki node (%s)", objType)
		}
		units = append(units, DocumentUnit{
			Content: content,
			Provenance: map[string]any{
				"provider":     "feishu",
				"wikiSpaceId":  wikiSpaceID,
				"parentToken":  strings.TrimSpace(parentNodeToken),
				"objType":      objType,
				"objToken":     objToken,
			},
			Confidence: 0.6,
		})
	}

	hasMore, _ := data["has_more"].(bool)
	nextCursor := strings.TrimSpace(fmtAny(data["page_token"]))
	return SourceFetchResponse{
		Units:      units,
		HasMore:    hasMore && nextCursor != "",
		NextCursor: nextCursor,
	}, nil
}

func parseRFC3339(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		tt := t.UTC()
		return &tt
	}
	return nil
}

func parseUnixSeconds(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return nil
	}
	t := time.Unix(n, 0).UTC()
	return &t
}
