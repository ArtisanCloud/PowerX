package brainx

import (
	"PowerX/internal/config"
	"PowerX/internal/logic/openapi/provider/brainx/schema"
	providerclient "PowerX/internal/provider"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/cache"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/contextx"
	"io"
	"net/http"
	"time"
)

type BrainXProviderClient struct {
	providerclient.ProviderClientInterface

	AuthToken  *schema.ResponseAuthToken
	BaseURL    string
	httpClient *http.Client
	conf       *config.Config
	cache      cache.CacheInterface
	tokenKey   string
}

func NewBrainXProviderClient(config *config.Config, cache cache.CacheInterface) *BrainXProviderClient {

	return &BrainXProviderClient{
		BaseURL:    config.OpenAPI.Providers.BrainX.BaseUrl,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		conf:       config,
		cache:      cache,
		tokenKey:   "provider.brainx.access_token",
	}
}

func (sp *BrainXProviderClient) Auth(ctx context.Context) (*schema.ResponseAuthToken, error) {
	url := "/auth"

	// 构造请求 body
	body := map[string]string{
		"access_key": sp.conf.OpenAPI.Providers.BrainX.AccessKey,    // 从配置中获取
		"secret_key": sp.conf.OpenAPI.Providers.BrainX.SecretKey,    // 从配置中获取
		"platform":   sp.conf.OpenAPI.Providers.BrainX.ProviderName, // 从配置中获取
	}

	// 发起 POST 请求
	resp, err := sp.HTTPPost(ctx, url, body, false, nil) // use_auth 设置为 false
	if err != nil {
		return nil, err
	}

	// 将 JSON 响应转换为 ResponseAuthToken 对象
	var token *schema.ResponseAuthToken
	err = json.Unmarshal(resp, &token)
	if err != nil {
		return nil, err
	}
	if token.Token.AccessToken == "" {
		return nil, errors.New("auth returned invalid  access token")
	}

	return token, nil
}

func (sp *BrainXProviderClient) GetAccessToken(ctx context.Context) (string, error) {
	// 从缓存中获取 token
	token, err := sp.cache.Get(sp.tokenKey, nil)
	if err != nil {
		if !errors.Is(err, cache.ErrCacheMiss) {
			return "", err
		}
	}

	if token == nil {
		// 如果缓存中没有 token，则调用 Auth 获取新的 token
		sp.AuthToken, err = sp.Auth(ctx)
		if err != nil {
			return "", fmt.Errorf("request powerx provider auth error: %v", err)
		}

		// 将 token 存入缓存，设置过期时间
		expiredIn := sp.AuthToken.Token.ExpiresIn
		err = sp.cache.Set(sp.tokenKey, sp.AuthToken, time.Duration(expiredIn)*time.Second)
		if err != nil {
			return "", err
		}
	} else {
		// 从 map[string]interface{} 转换到 ResponseAuthToken
		tokenMap, ok := token.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("invalid token type in cache")
		}

		// Marshal and Unmarshal to convert
		tokenBytes, err := json.Marshal(tokenMap)
		if err != nil {
			return "", err
		}

		var authToken schema.ResponseAuthToken
		if err := json.Unmarshal(tokenBytes, &authToken); err != nil {
			return "", err
		}
		sp.AuthToken = &authToken
	}

	// 返回 accessToken
	return sp.AuthToken.Token.AccessToken, nil
}
func (sp *BrainXProviderClient) doRequest(ctx context.Context, req *http.Request, useAuth bool) ([]byte, error) {
	// 如果需要授权，添加 Authorization 头部
	if useAuth {
		token, err := sp.GetAccessToken(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	// 发起 HTTP 请求
	resp, err := sp.httpClient.Do(req)
	print(resp)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 检查 HTTP 响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status code %d", resp.StatusCode)
	}

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBody, nil
}

func (sp *BrainXProviderClient) HTTPGet(ctx context.Context, uri string, params map[string]string, useAuth bool, headers map[string]string) ([]byte, error) {
	url := sp.BaseURL + uri
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 添加查询参数
	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	// 添加自定义头部信息
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return sp.doRequest(ctx, req, useAuth)
}

func (sp *BrainXProviderClient) HTTPPost(ctx context.Context, uri string, jsonData interface{}, useAuth bool, headers map[string]string) ([]byte, error) {
	newCtx := contextx.ValueOnlyFrom(ctx)
	timeoutCtx, cancel := context.WithTimeout(newCtx, 60*time.Second)
	defer cancel()

	// 将 jsonData 转换为 JSON 格式的字节流
	body, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	url := sp.BaseURL + uri
	req, err := http.NewRequestWithContext(timeoutCtx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头部信息
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return sp.doRequest(timeoutCtx, req, useAuth)
}
