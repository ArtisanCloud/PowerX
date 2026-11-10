package marketplace

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DeploymentFingerprintFeature 是协商后的可选扩展；旧方法和旧实例请求保持原格式。
const DeploymentFingerprintFeature = "powerx.deployment-fingerprint/v1"

// BuildDeclaration 是清单摘要声明，不是完整清单，也不等价于远程验真。
type BuildDeclaration struct {
	Schema       string `json:"schema"`
	Component    string `json:"component"`
	ReleaseID    string `json:"release_id"`
	SourceCommit string `json:"source_commit"`
	BuildID      string `json:"build_id"`
}

// DeploymentDeclaration 用指针区分入口省略和显式清空；凭证始终走实例请求头。
type DeploymentDeclaration struct {
	BuildInfo     *BuildDeclaration `json:"build_info,omitempty"`
	PublicSiteURL *string           `json:"public_site_url,omitempty"`
}

// SiteChallenge 来自已认证市场的完整集合，不进入持久化实例凭证文件。
type SiteChallenge struct {
	Schema      string `json:"schema"`
	ChallengeID string `json:"challenge_id"`
	SiteURL     string `json:"site_url"`
	Nonce       string `json:"nonce"`
	ExpiresAt   string `json:"expires_at"`
	ExpiresInMS int64  `json:"expires_in_ms"`
}

// DeploymentResponse 的空集合与字段缺失具有不同含义。
type DeploymentResponse struct {
	ServerTime     *string          `json:"server_time,omitempty"`
	SiteChallenges *[]SiteChallenge `json:"site_challenges,omitempty"`
	Extended       bool             `json:"-"`
	RoundTrip      time.Duration    `json:"-"`
	ReceivedAt     time.Time        `json:"-"`
}

// DeploymentHeartbeat 保留原心跳状态，附加协商成功的证明投递。
type DeploymentHeartbeat struct {
	Status string `json:"status"`
	DeploymentResponse
}

type deploymentProtocolState struct {
	mu                                  sync.Mutex
	refresh                             sync.Mutex
	generation                          uint64
	known, supported                    bool
	paused                              bool
	learnedAt, nextRefresh, forcedRetry time.Time
	failures, forcedFailures            int
	lastError                           error
}

func protocolBackoff(failures int) time.Duration {
	d := 5 * time.Second
	for i := 1; i < failures && d < 5*time.Minute; i++ {
		d *= 2
	}
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

func (c *Client) deploymentSupport(ctx context.Context, force bool, expected uint64, recovering400 bool) (bool, uint64, error) {
	s := &c.deployment
	s.refresh.Lock()
	defer s.refresh.Unlock()
	s.mu.Lock()
	if force && s.generation != expected {
		supported, generation, err := s.supported, s.generation, s.lastError
		s.mu.Unlock()
		return supported, generation, err
	}
	if !force && s.known && time.Since(s.learnedAt) < 10*time.Minute {
		supported, generation := s.supported, s.generation
		s.mu.Unlock()
		return supported, generation, nil
	}
	if time.Now().Before(s.nextRefresh) || recovering400 && time.Now().Before(s.forcedRetry) {
		supported, generation, err := s.supported, s.generation, s.lastError
		s.mu.Unlock()
		return supported, generation, err
	}
	s.mu.Unlock()
	discovery, err := c.Discover(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.lastError = err
	s.known = err == nil
	s.supported = false
	if recovering400 {
		s.forcedFailures++
		s.forcedRetry = time.Now().Add(protocolBackoff(s.forcedFailures))
	}
	if err != nil {
		s.failures++
		s.nextRefresh = time.Now().Add(protocolBackoff(s.failures))
		return false, s.generation, err
	}
	s.failures = 0
	s.nextRefresh = time.Time{}
	s.learnedAt = time.Now()
	for _, feature := range discovery.Features {
		if feature == DeploymentFingerprintFeature {
			s.supported = true
		}
	}
	return s.supported, s.generation, nil
}

// RefreshDeploymentCapabilities 在发送器启动及能力缓存到期时使用；不会改变实例身份。
func (c *Client) RefreshDeploymentCapabilities(ctx context.Context) error {
	c.deployment.mu.Lock()
	generation := c.deployment.generation
	c.deployment.mu.Unlock()
	_, _, err := c.deploymentSupport(ctx, true, generation, false)
	return err
}

func (c *Client) invalidateDeploymentResponse() {
	s := &c.deployment
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known = false
	s.supported = false
	s.generation++
	s.failures++
	s.lastError = &Error{502, "MARKETPLACE_DEPLOYMENT_PROTOCOL_INVALID", "Marketplace 部署证明响应无效"}
	s.nextRefresh = time.Now().Add(protocolBackoff(s.failures))
}

func compatibilityFailure(err error) bool {
	var upstream *RequestError
	return errors.As(err, &upstream) && upstream.HTTPStatus == 400 && (upstream.UpstreamCode == "invalid_input" || upstream.UpstreamCode == "instance_extension_unsupported")
}

func (c *Client) deploymentPOST(ctx context.Context, suffix string, base map[string]any, declaration *DeploymentDeclaration, out any, override string) (bool, time.Duration, error) {
	supported, generation := false, uint64(0)
	if declaration != nil {
		var err error
		supported, generation, err = c.deploymentSupport(ctx, false, 0, false)
		if err != nil && suffix == "/host/instances/register" {
			return false, 0, err
		}
	}
	body := base
	if supported && declaration != nil {
		body = make(map[string]any, len(base)+3)
		for k, v := range base {
			body[k] = v
		}
		body["features"] = []string{DeploymentFingerprintFeature}
		if declaration.BuildInfo != nil {
			body["build_info"] = declaration.BuildInfo
		}
		if declaration.PublicSiteURL != nil {
			body["public_site_url"] = *declaration.PublicSiteURL
		}
	}
	started := time.Now()
	err := c.requestWithHeaders(ctx, http.MethodPost, suffix, nil, body, out, override, "")
	elapsed := time.Since(started)
	if err == nil || !supported || !compatibilityFailure(err) {
		if err == nil && supported {
			c.deployment.mu.Lock()
			c.deployment.paused = false
			c.deployment.mu.Unlock()
		}
		return supported, elapsed, err
	}
	c.deployment.mu.Lock()
	c.deployment.paused = true
	c.deployment.mu.Unlock()
	// 原 POST 的参数错误必须保留；仅 discovery 明确否认扩展后才重试旧格式。
	stillSupported, _, discoveryErr := c.deploymentSupport(ctx, true, generation, true)
	if discoveryErr != nil || stillSupported {
		return false, elapsed, err
	}
	started = time.Now()
	err = c.requestWithHeaders(ctx, http.MethodPost, suffix, nil, base, out, override, "")
	return false, time.Since(started), err
}

// RegisterAutomaticWithDeployment 是显式使用的扩展入口；旧自动登记方法仍可直接复用。
func (c *Client) RegisterAutomaticWithDeployment(ctx context.Context, credential, fingerprint, version, name string, declaration *DeploymentDeclaration) (Registration, error) {
	var out Registration
	extended, elapsed, err := c.deploymentPOST(ctx, "/host/instances/register", map[string]any{"fingerprint_sha256": fingerprint, "powerx_version": strings.TrimPrefix(version, "v"), "os": runtime.GOOS, "arch": runtime.GOARCH, "name": name}, declaration, &out, credential)
	out.Extended, out.RoundTrip, out.ReceivedAt = extended, elapsed, time.Now()
	if err == nil && out.Protocol != AutomaticRegistrationProtocol {
		err = &Error{502, "MARKETPLACE_REGISTRATION_UNSUPPORTED", "Marketplace 自动登记响应无效"}
	}
	if err == nil && extended {
		err = validateDeploymentResponse(out.DeploymentResponse, declaration)
	}
	if err != nil && extended && !compatibilityFailure(err) {
		c.invalidateIfProtocolError(err)
	}
	if err != nil {
		out.Extended = false
	}
	return out, err
}

// HeartbeatWithDeployment 不将结果回报失败转换为重新登记、换钥匙或安装动作。
func (c *Client) HeartbeatWithDeployment(ctx context.Context, version string, declaration *DeploymentDeclaration) (DeploymentHeartbeat, error) {
	var out DeploymentHeartbeat
	extended, elapsed, err := c.deploymentPOST(ctx, "/host/instances/heartbeat", map[string]any{"powerx_version": strings.TrimPrefix(version, "v")}, declaration, &out, "")
	out.Extended, out.RoundTrip, out.ReceivedAt = extended, elapsed, time.Now()
	if err == nil && out.Status != "recorded" {
		err = &Error{502, "MARKETPLACE_DEPLOYMENT_PROTOCOL_INVALID", "Marketplace 心跳响应无效"}
	}
	if err == nil && extended {
		err = validateDeploymentResponse(out.DeploymentResponse, declaration)
	}
	if err != nil && extended {
		c.invalidateIfProtocolError(err)
	}
	if err != nil {
		out.Extended = false
	}
	return out, err
}

func (c *Client) invalidateIfProtocolError(err error) {
	var failure *Error
	if errors.As(err, &failure) {
		switch failure.Code {
		case "MARKETPLACE_DEPLOYMENT_PROTOCOL_INVALID", "MARKETPLACE_INVALID_RESPONSE", "MARKETPLACE_REGISTRATION_UNSUPPORTED":
			c.invalidateDeploymentResponse()
		}
	}
}

func validateDeploymentResponse(response DeploymentResponse, declaration *DeploymentDeclaration) error {
	invalid := func() error {
		return &Error{502, "MARKETPLACE_DEPLOYMENT_PROTOCOL_INVALID", "Marketplace 部署证明响应无效"}
	}
	if response.ServerTime == nil || response.SiteChallenges == nil || len(*response.SiteChallenges) > 4 {
		return invalid()
	}
	if !deploymentUTCMilliseconds.MatchString(*response.ServerTime) {
		return invalid()
	}
	server, err := time.Parse(time.RFC3339Nano, *response.ServerTime)
	if err != nil || server.Nanosecond()%1000000 != 0 {
		return invalid()
	}
	seen := map[string]bool{}
	for _, challenge := range *response.SiteChallenges {
		if !deploymentUTCMilliseconds.MatchString(challenge.ExpiresAt) {
			return invalid()
		}
		if id, err := uuid.Parse(challenge.ChallengeID); err != nil || id == uuid.Nil || id.String() != challenge.ChallengeID || seen[challenge.ChallengeID] {
			return invalid()
		}
		seen[challenge.ChallengeID] = true
		nonce, err := base64.RawURLEncoding.DecodeString(challenge.Nonce)
		if err != nil || len(nonce) != 32 || base64.RawURLEncoding.EncodeToString(nonce) != challenge.Nonce {
			return invalid()
		}
		expires, err := time.Parse(time.RFC3339Nano, challenge.ExpiresAt)
		if err != nil || expires.Nanosecond()%1000000 != 0 || challenge.ExpiresInMS <= 0 || challenge.ExpiresInMS > 600000 || expires.Sub(server).Milliseconds() != challenge.ExpiresInMS {
			return invalid()
		}
		if challenge.Schema != "powerx.instance-proof/v1" || !strings.HasPrefix(challenge.SiteURL, "https://") || declaration == nil || declaration.PublicSiteURL == nil || *declaration.PublicSiteURL == "" || challenge.SiteURL != *declaration.PublicSiteURL {
			return invalid()
		}
	}
	return nil
}

var deploymentUTCMilliseconds = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$`)

// DeploymentReady 控制公开证明发布；扩展参数错误发生后立即暂停，不能等重探测结束。
func (c *Client) DeploymentReady() bool {
	c.deployment.mu.Lock()
	defer c.deployment.mu.Unlock()
	return c.deployment.known && c.deployment.supported && !c.deployment.paused && time.Since(c.deployment.learnedAt) < 10*time.Minute
}
