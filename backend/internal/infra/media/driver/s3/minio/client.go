package minio

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Options 定义客户端初始化所需参数（为适配 S3/MinIO 简化）。
type Options struct {
	AccessKey      string
	SecretKey      string
	SessionToken   string
	Secure         bool
	Region         string
	ForcePathStyle bool
	HTTPClient     *http.Client
}

// Client 为最小化实现的 S3 兼容客户端，覆盖本项目所需操作。
type Client struct {
	endpoint     *url.URL
	accessKey    string
	secretKey    string
	sessionToken string
	region       string
	forcePath    bool
	httpClient   *http.Client
}

// PutObjectOptions 定义上传对象时的可选参数。
type PutObjectOptions struct {
	ContentType  string
	UserMetadata map[string]string
	OnlyIfAbsent bool
}

// UploadInfo 为上传成功后的返回信息。
type UploadInfo struct {
	Size      int64
	ETag      string
	VersionID string
}

// GetObjectOptions 为下载对象提供扩展字段（暂为空）。
type GetObjectOptions struct{}

// RemoveObjectOptions 定义删除对象时的附加参数。
type RemoveObjectOptions struct {
	ForceDelete      bool
	VersionID        string
	GovernanceBypass bool
}

// ObjectInfo 描述对象元信息。
type ObjectInfo struct {
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
}

// Object 封装对象读取句柄。
type Object struct {
	body io.ReadCloser
	info ObjectInfo
}

func (o *Object) Read(p []byte) (int, error) {
	return o.body.Read(p)
}

func (o *Object) Close() error {
	return o.body.Close()
}

func (o *Object) Stat() (ObjectInfo, error) {
	return o.info, nil
}

// ErrorResponse 表示 S3 返回的错误。
type ErrorResponse struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ErrorResponse) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("s3 error %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("s3 error: %s", e.Message)
}

// New 创建客户端实例。
func New(endpoint string, opts *Options) (*Client, error) {
	if opts == nil {
		opts = &Options{}
	}
	if endpoint == "" {
		return nil, fmt.Errorf("minio: endpoint 不能为空")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("minio: endpoint 非法: %w", err)
	}
	if parsed.Scheme == "" {
		if opts.Secure {
			parsed.Scheme = "https"
		} else {
			parsed.Scheme = "http"
		}
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("minio: endpoint 缺少主机名")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	client := &Client{
		endpoint:     parsed,
		accessKey:    opts.AccessKey,
		secretKey:    opts.SecretKey,
		sessionToken: opts.SessionToken,
		region:       opts.Region,
		forcePath:    opts.ForcePathStyle,
		httpClient:   opts.HTTPClient,
	}
	if client.region == "" {
		client.region = "us-east-1"
	}
	return client, nil
}

// PutObject 上传对象。
func (c *Client) PutObject(ctx context.Context, bucket, object string, reader io.Reader, size int64, opts PutObjectOptions) (UploadInfo, error) {
	urlObj, canonicalPath, err := c.buildObjectURL(bucket, object)
	if err != nil {
		return UploadInfo{}, err
	}
	if reader == nil {
		return UploadInfo{}, fmt.Errorf("minio: reader 不能为空")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, urlObj.String(), reader)
	if err != nil {
		return UploadInfo{}, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if opts.ContentType != "" {
		req.Header.Set("Content-Type", opts.ContentType)
	}
	for k, v := range opts.UserMetadata {
		if k == "" {
			continue
		}
		headerKey := "x-amz-meta-" + strings.ToLower(k)
		req.Header.Set(headerKey, v)
	}
	if opts.OnlyIfAbsent {
		req.Header.Set("If-None-Match", "*")
	}

	if err = c.signRequest(req, canonicalPath, "UNSIGNED-PAYLOAD"); err != nil {
		return UploadInfo{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UploadInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPreconditionFailed {
		return UploadInfo{}, &ErrorResponse{StatusCode: resp.StatusCode, Code: "PreconditionFailed", Message: "object already exists"}
	}
	if resp.StatusCode >= 400 {
		return UploadInfo{}, c.parseError(resp)
	}

	info := UploadInfo{Size: size}
	if etag := resp.Header.Get("ETag"); etag != "" {
		info.ETag = strings.Trim(etag, "\"")
	}
	if vid := resp.Header.Get("x-amz-version-id"); vid != "" {
		info.VersionID = vid
	}
	return info, nil
}

// GetObject 下载对象。
func (c *Client) GetObject(ctx context.Context, bucket, object string, _ GetObjectOptions) (*Object, error) {
	urlObj, canonicalPath, err := c.buildObjectURL(bucket, object)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlObj.String(), nil)
	if err != nil {
		return nil, err
	}
	if err = c.signRequest(req, canonicalPath, "UNSIGNED-PAYLOAD"); err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.parseError(resp)
	}

	info := ObjectInfo{}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, convErr := parseInt64(cl); convErr == nil {
			info.Size = n
		}
	}
	info.ContentType = resp.Header.Get("Content-Type")
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if tm, parseErr := time.Parse(time.RFC1123, lm); parseErr == nil {
			info.LastModified = tm.UTC()
		}
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		info.ETag = strings.Trim(etag, "\"")
	}

	return &Object{body: resp.Body, info: info}, nil
}

// RemoveObject 删除对象。
func (c *Client) RemoveObject(ctx context.Context, bucket, object string, opts RemoveObjectOptions) error {
	urlObj, canonicalPath, err := c.buildObjectURL(bucket, object)
	if err != nil {
		return err
	}
	query := urlObj.Query()
	if opts.VersionID != "" {
		query.Set("versionId", opts.VersionID)
		urlObj.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, urlObj.String(), nil)
	if err != nil {
		return err
	}
	if opts.GovernanceBypass {
		req.Header.Set("x-amz-bypass-governance-retention", "true")
	}
	if err = c.signRequest(req, canonicalPath, "UNSIGNED-PAYLOAD"); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ErrorResponse{StatusCode: resp.StatusCode, Code: "NotFound", Message: "object not found"}
	}
	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}
	return nil
}

// PresignedGetObject 生成预签名下载链接。
func (c *Client) PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, params url.Values) (*url.URL, error) {
	if expires <= 0 {
		return nil, fmt.Errorf("minio: expires must be positive")
	}
	urlObj, canonicalPath, err := c.buildObjectURL(bucket, object)
	if err != nil {
		return nil, err
	}
	return c.presignURL(ctx, http.MethodGet, urlObj, canonicalPath, expires, params, nil)
}

// PresignedPutObject 生成预签名上传链接。
func (c *Client) PresignedPutObject(ctx context.Context, bucket, object string, expires time.Duration) (*url.URL, error) {
	if expires <= 0 {
		return nil, fmt.Errorf("minio: expires must be positive")
	}
	urlObj, canonicalPath, err := c.buildObjectURL(bucket, object)
	if err != nil {
		return nil, err
	}
	return c.presignURL(ctx, http.MethodPut, urlObj, canonicalPath, expires, nil, nil)
}

// BucketExists 判断桶是否存在。
func (c *Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	urlObj, canonicalPath, err := c.buildBucketURL(bucket)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, urlObj.String(), nil)
	if err != nil {
		return false, err
	}
	if err = c.signRequest(req, canonicalPath, "UNSIGNED-PAYLOAD"); err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		return false, c.parseError(resp)
	}
	return true, nil
}

func (c *Client) buildObjectURL(bucket, object string) (*url.URL, string, error) {
	if strings.TrimSpace(object) == "" {
		return nil, "", fmt.Errorf("minio: object key 不能为空")
	}
	return c.buildURL(bucket, object)
}

func (c *Client) buildBucketURL(bucket string) (*url.URL, string, error) {
	return c.buildURL(bucket, "")
}

func (c *Client) buildURL(bucket, object string) (*url.URL, string, error) {
	base := *c.endpoint
	objClean := strings.TrimLeft(object, "/")
	if c.forcePath || bucket == "" {
		base.Path = joinURLPath(base.Path, bucket, objClean)
	} else {
		base.Host = bucket + "." + base.Host
		base.Path = joinURLPath(base.Path, objClean)
	}
	canonicalPath := base.Path
	if canonicalPath == "" {
		canonicalPath = "/"
	}
	return &base, canonicalPath, nil
}

func (c *Client) signRequest(req *http.Request, canonicalPath, payloadHash string) error {
	now := time.Now().UTC()
	if payloadHash == "" {
		payloadHash = "UNSIGNED-PAYLOAD"
	}
	req.Header.Set("x-amz-date", now.Format("20060102T150405Z"))
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if c.sessionToken != "" {
		req.Header.Set("x-amz-security-token", c.sessionToken)
	}
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}
	canonicalQuery := canonicalQueryString(req.URL.Query())
	canonicalHeaders, signedHeaders := canonicalHeadersAndSigned(req.Header, req.URL.Host)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(canonicalPath),
		canonicalQuery,
		canonicalHeaders,
		"",
		signedHeaders,
		payloadHash,
	}, "\n")
	hashed := sha256.Sum256([]byte(canonicalRequest))
	scope := credentialScope(now, c.region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		now.Format("20060102T150405Z"),
		scope,
		hex.EncodeToString(hashed[:]),
	}, "\n")
	signingKey := deriveSigningKey(c.secretKey, now, c.region)
	signature := hmacSHA256Hex(signingKey, stringToSign)

	authValue := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", c.accessKey, scope, signedHeaders, signature)
	req.Header.Set("Authorization", authValue)
	return nil
}

func (c *Client) presignURL(ctx context.Context, method string, baseURL *url.URL, canonicalPath string, expires time.Duration, params url.Values, headers http.Header) (*url.URL, error) {
	if expires <= 0 {
		return nil, fmt.Errorf("minio: expires must be positive")
	}
	seconds := int64(expires / time.Second)
	if seconds <= 0 {
		seconds = 1
	}
	now := time.Now().UTC()
	if params == nil {
		params = url.Values{}
	} else {
		params = cloneValues(params)
	}
	params.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	params.Set("X-Amz-Credential", fmt.Sprintf("%s/%s", c.accessKey, credentialScope(now, c.region)))
	params.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	params.Set("X-Amz-Expires", fmt.Sprintf("%d", seconds))

	if headers == nil {
		headers = http.Header{}
	}
	headers = headers.Clone()
	headers.Set("Host", baseURL.Host)
	if c.sessionToken != "" {
		params.Set("X-Amz-Security-Token", c.sessionToken)
	}
	canonicalHeaders, signedHeaders := canonicalHeadersAndSigned(headers, baseURL.Host)
	params.Set("X-Amz-SignedHeaders", signedHeaders)

	payloadHash := "UNSIGNED-PAYLOAD"
	canonicalQuery := canonicalQueryString(params)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI(canonicalPath),
		canonicalQuery,
		canonicalHeaders,
		"",
		signedHeaders,
		payloadHash,
	}, "\n")
	hashed := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		now.Format("20060102T150405Z"),
		credentialScope(now, c.region),
		hex.EncodeToString(hashed[:]),
	}, "\n")
	signingKey := deriveSigningKey(c.secretKey, now, c.region)
	signature := hmacSHA256Hex(signingKey, stringToSign)
	params.Set("X-Amz-Signature", signature)

	result := *baseURL
	result.RawQuery = canonicalQueryString(params)
	return &result, nil
}

func (c *Client) parseError(resp *http.Response) error {
	defer io.Copy(io.Discard, resp.Body)
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var decoded struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	if err := xml.Unmarshal(data, &decoded); err != nil {
		return &ErrorResponse{StatusCode: resp.StatusCode, Code: resp.Status, Message: string(bytes.TrimSpace(data))}
	}
	return &ErrorResponse{StatusCode: resp.StatusCode, Code: decoded.Code, Message: decoded.Message}
}

func credentialScope(t time.Time, region string) string {
	date := t.Format("20060102")
	return fmt.Sprintf("%s/%s/s3/aws4_request", date, region)
}

func canonicalURI(p string) string {
	if p == "" {
		return "/"
	}
	hasTrailing := strings.HasSuffix(p, "/") && len(p) > 1
	segments := strings.Split(p, "/")
	var builder strings.Builder
	for i, seg := range segments {
		if i > 0 {
			builder.WriteByte('/')
		}
		builder.WriteString(uriEncode(seg, false))
	}
	if builder.Len() == 0 {
		builder.WriteByte('/')
	} else if hasTrailing {
		if builder.Len() == 0 || builder.String()[builder.Len()-1] != '/' {
			builder.WriteByte('/')
		}
	}
	return builder.String()
}

func canonicalQueryString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var builder strings.Builder
	first := true
	for _, k := range keys {
		vals := values[k]
		sort.Strings(vals)
		for _, v := range vals {
			if !first {
				builder.WriteByte('&')
			}
			builder.WriteString(uriEncode(k, true))
			builder.WriteByte('=')
			builder.WriteString(uriEncode(v, true))
			first = false
		}
	}
	return builder.String()
}

func canonicalHeadersAndSigned(header http.Header, host string) (string, string) {
	temp := make(map[string][]string)
	for k, vs := range header {
		lower := strings.ToLower(k)
		temp[lower] = append(temp[lower], vs...)
	}
	temp["host"] = []string{host}
	keys := make([]string, 0, len(temp))
	for k := range temp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var headerBuilder strings.Builder
	var signedBuilder strings.Builder
	for i, k := range keys {
		vals := temp[k]
		for idx := range vals {
			vals[idx] = strings.TrimSpace(vals[idx])
			vals[idx] = strings.Join(strings.Fields(vals[idx]), " ")
		}
		sort.Strings(vals)
		if i > 0 {
			signedBuilder.WriteByte(';')
		}
		signedBuilder.WriteString(k)
		headerBuilder.WriteString(k)
		headerBuilder.WriteByte(':')
		headerBuilder.WriteString(strings.Join(vals, ","))
		headerBuilder.WriteByte('\n')
	}
	return headerBuilder.String(), signedBuilder.String()
}

func uriEncode(s string, encodeSlash bool) string {
	encoded := url.PathEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	if !encodeSlash {
		encoded = strings.ReplaceAll(encoded, "%2F", "/")
	}
	return encoded
}

func deriveSigningKey(secret string, t time.Time, region string) []byte {
	date := t.Format("20060102")
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func hmacSHA256Hex(key []byte, data string) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for k, vs := range values {
		copied := make([]string, len(vs))
		copy(copied, vs)
		cloned[k] = copied
	}
	return cloned
}

// ToErrorResponse 模拟 MinIO SDK 的帮助方法，用于提取错误响应。
func ToErrorResponse(err error) ErrorResponse {
	if err == nil {
		return ErrorResponse{}
	}
	var resp *ErrorResponse
	if errors.As(err, &resp) {
		return *resp
	}
	return ErrorResponse{Message: err.Error()}
}

func joinURLPath(basePath string, parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	trimmedBase := strings.Trim(basePath, "/")
	if trimmedBase != "" {
		segments = append(segments, trimmedBase)
	}
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			segments = append(segments, p)
		}
	}
	return "/" + strings.Join(segments, "/")
}
