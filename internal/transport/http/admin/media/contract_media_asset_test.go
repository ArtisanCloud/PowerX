package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAdminMediaHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.NewServeMux()
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path string, query url.Values, payload any) *httptest.ResponseRecorder {
	t.Helper()

	target := buildTarget(path, query)

	var body *bytes.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(data)
	} else {
		body = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, target, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func buildTarget(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return fmt.Sprintf("%s?%s", path, query.Encode())
}

func applyPathParams(path string, params map[string]string) string {
	for key, value := range params {
		placeholder := fmt.Sprintf("{%s}", key)
		path = strings.ReplaceAll(path, placeholder, value)
	}
	return path
}

func decodeJSONResponse(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	return payload
}

func TestContractCreateMediaAsset(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	payload := map[string]any{
		"name":             "homepage-banner",
		"description":      "首页横幅图片",
		"driver":           "local",
		"ownerSubjectType": "campaign",
		"ownerSubjectId":   "cmp_123",
		"tags":             []string{"banner", "homepage"},
		"uploadMethod":     "direct_upload",
	}

	rr := performJSONRequest(t, handler, http.MethodPost, "/api/admin/v1/admin/media/assets", nil, payload)

	require.Equal(t, http.StatusCreated, rr.Code)

	response := decodeJSONResponse(t, rr)

	uuid, ok := response["uuid"].(string)
	require.True(t, ok)
	require.NotEmpty(t, uuid)

	driver, ok := response["driver"].(string)
	require.True(t, ok)
	require.Equal(t, "local", driver)

	status, ok := response["businessStatus"].(string)
	require.True(t, ok)
	require.Equal(t, "draft", status)
}

func TestContractListMediaAssets(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	query.Add("tags", "homepage")

	rr := performJSONRequest(t, handler, http.MethodGet, "/api/admin/v1/admin/media/assets", query, nil)

	require.Equal(t, http.StatusOK, rr.Code)

	response := decodeJSONResponse(t, rr)
	items, ok := response["items"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)

	total, ok := response["total"].(float64)
	require.True(t, ok)
	require.Greater(t, total, float64(0))

	page, ok := response["page"].(float64)
	require.True(t, ok)
	require.Equal(t, float64(1), page)

	pageSize, ok := response["pageSize"].(float64)
	require.True(t, ok)
	require.Equal(t, float64(20), pageSize)
}

func TestContractGetMediaAsset(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	path := applyPathParams("/api/admin/v1/admin/media/assets/{uuid}", map[string]string{
		"uuid": "mas_123",
	})

	rr := performJSONRequest(t, handler, http.MethodGet, path, nil, nil)

	require.Equal(t, http.StatusOK, rr.Code)

	response := decodeJSONResponse(t, rr)

	uuid, ok := response["uuid"].(string)
	require.True(t, ok)
	require.Equal(t, "mas_123", uuid)

	name, ok := response["name"].(string)
	require.True(t, ok)
	require.NotEmpty(t, name)

	tenantID, ok := response["tenantId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, tenantID)
}

func TestContractUpdateMediaAsset(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	payload := map[string]any{
		"name":           "homepage-banner-updated",
		"description":    "更新横幅描述",
		"businessStatus": "under_review",
		"tags":           []string{"banner", "homepage", "2025"},
	}

	path := applyPathParams("/api/admin/v1/admin/media/assets/{uuid}", map[string]string{
		"uuid": "mas_123",
	})

	rr := performJSONRequest(t, handler, http.MethodPatch, path, nil, payload)

	require.Equal(t, http.StatusOK, rr.Code)

	response := decodeJSONResponse(t, rr)

	name, ok := response["name"].(string)
	require.True(t, ok)
	require.Equal(t, "homepage-banner-updated", name)

	status, ok := response["businessStatus"].(string)
	require.True(t, ok)
	require.Equal(t, "under_review", status)

	tags, ok := response["tags"].([]any)
	require.True(t, ok)
	require.Contains(t, tags, "2025")
}

func TestContractDeleteMediaAsset(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	path := applyPathParams("/api/admin/v1/admin/media/assets/{uuid}", map[string]string{
		"uuid": "mas_123",
	})

	rr := performJSONRequest(t, handler, http.MethodDelete, path, nil, nil)

	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Empty(t, rr.Body.String())
}

func TestContractPresignMediaAsset(t *testing.T) {
	t.Parallel()

	handler := newAdminMediaHandler(t)

	path := applyPathParams("/api/admin/v1/admin/media/assets/{uuid}/presign", map[string]string{
		"uuid": "mas_123",
	})

	rr := performJSONRequest(t, handler, http.MethodPost, path, nil, map[string]any{
		"filename": "banner.png",
		"size":     524288,
		"mime":     "image/png",
	})

	require.Equal(t, http.StatusOK, rr.Code)

	response := decodeJSONResponse(t, rr)

	uploadURL, ok := response["uploadUrl"].(string)
	require.True(t, ok)
	require.NotEmpty(t, uploadURL)

	headers, ok := response["headers"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, headers)
}
