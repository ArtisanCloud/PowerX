package minio

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ObjectVersion 是包含删除标记的版本清单；永久清理必须指定确切版本。
type ObjectVersion struct {
	DeleteMarker bool      `xml:"-"`
	Key          string    `xml:"Key"`
	VersionID    string    `xml:"VersionId"`
	IsLatest     bool      `xml:"IsLatest"`
	LastModified time.Time `xml:"LastModified"`
	Size         int64     `xml:"Size"`
}
type ObjectVersions struct {
	Versions            []ObjectVersion `xml:"Version"`
	DeleteMarkers       []ObjectVersion `xml:"DeleteMarker"`
	Truncated           bool            `xml:"IsTruncated"`
	NextKeyMarker       string          `xml:"NextKeyMarker"`
	NextVersionIDMarker string          `xml:"NextVersionIdMarker"`
}

func (c *Client) bucketDocument(ctx context.Context, bucket string, query url.Values) ([]byte, error) {
	u, path, err := c.buildBucketURL(bucket)
	if err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if err = c.signRequest(req, path, "UNSIGNED-PAYLOAD"); err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.parseError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return nil, fmt.Errorf("s3 bucket response exceeds limit")
	}
	return raw, nil
}
func (c *Client) BucketVersioningEnabled(ctx context.Context, bucket string) (bool, error) {
	raw, err := c.bucketDocument(ctx, bucket, url.Values{"versioning": {""}})
	if err != nil {
		return false, err
	}
	var d struct {
		Status string `xml:"Status"`
	}
	if err = xml.Unmarshal(raw, &d); err != nil {
		return false, err
	}
	return d.Status == "Enabled", nil
}
func (c *Client) BucketPolicy(ctx context.Context, bucket string) ([]byte, error) {
	return c.bucketDocument(ctx, bucket, url.Values{"policy": {""}})
}
func (c *Client) BucketACL(ctx context.Context, bucket string) ([]byte, error) {
	return c.bucketDocument(ctx, bucket, url.Values{"acl": {""}})
}
func (c *Client) BucketLifecycle(ctx context.Context, bucket string) ([]byte, error) {
	return c.bucketDocument(ctx, bucket, url.Values{"lifecycle": {""}})
}

// ListObjectVersions 返回一页有界结果；调用者负责显式处理分页和覆盖不完整。
func (c *Client) ListObjectVersions(ctx context.Context, bucket, prefix, keyMarker, versionMarker string, limit int) (ObjectVersions, error) {
	var out ObjectVersions
	if limit < 1 || limit > 1000 {
		return out, fmt.Errorf("invalid object version page size")
	}
	q := url.Values{"versions": {""}, "prefix": {prefix}, "max-keys": {strconv.Itoa(limit)}}
	if keyMarker != "" {
		q.Set("key-marker", keyMarker)
	}
	if versionMarker != "" {
		q.Set("version-id-marker", versionMarker)
	}
	raw, err := c.bucketDocument(ctx, bucket, q)
	if err != nil {
		return out, err
	}
	err = xml.Unmarshal(raw, &out)
	return out, err
}
