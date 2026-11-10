package plugin_marketplace

import (
	"errors"
	"sync"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
)

type publicProof struct {
	Schema      string `json:"schema"`
	ChallengeID string `json:"challenge_id"`
	Nonce       string `json:"nonce"`
	ExpiresAt   string `json:"expires_at"`
}
type storedProof struct {
	value    publicProof
	deadline time.Time
}
type deploymentState struct {
	mu         sync.Mutex
	enabled    bool
	generation uint64
	info       productinfo.Information
	site       string
	proofs     map[string]storedProof
	now        func() time.Time
}

// ConfigureDeployment 支持在配置重载边界替换/清空入口，立即停止旧证明。
// 无效可选入口不会阻塞 Core 启动、登记或本地安装。
func (s *Service) ConfigureDeployment(info productinfo.Information, site string) error {
	var err error
	if site != "" {
		site, err = market.NormalizeWebsite(site)
	}
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	if info.Product == "" {
		info = productinfo.Information{Product: "powerx", State: "development"}
	}
	d.enabled = err == nil
	d.generation++
	d.info = info
	d.site = site
	d.proofs = map[string]storedProof{}
	return err
}

// ProductInformation 读取公开构建状态，不读取私有分发日志。
func (s *Service) ProductInformation() productinfo.Information {
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.info
}
func (s *Service) deploymentDeclaration() (*market.DeploymentDeclaration, uint64) {
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled {
		return nil, d.generation
	}
	site := d.site
	declaration := &market.DeploymentDeclaration{PublicSiteURL: &site}
	if b := d.info.BuildInfo; b != nil && (d.info.State == "available" || d.info.State == "development") {
		declaration.BuildInfo = &market.BuildDeclaration{Schema: b.Schema, Component: b.Component, ReleaseID: b.ReleaseID, SourceCommit: b.SourceCommit, BuildID: b.BuildID}
	}
	return declaration, d.generation
}
func (s *Service) clearDeploymentProofs() {
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	d.generation++
	d.proofs = map[string]storedProof{}
}

// acceptDeployment 使用进程内单调期限。整个集合替换，重发同一挑战不能续期。
func (s *Service) acceptDeployment(response market.DeploymentResponse, generation uint64, err error) {
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	if generation != d.generation {
		return
	}
	if err != nil {
		var request *market.RequestError
		var protocol *market.Error
		clear := !s.client.DeploymentReady() || market.AuthenticationRejected(err)
		if errors.As(err, &request) && request.HTTPStatus == 400 {
			clear = true
		}
		if errors.As(err, &protocol) && (protocol.Code == "MARKETPLACE_DEPLOYMENT_PROTOCOL_INVALID" || protocol.Code == "MARKETPLACE_INVALID_RESPONSE") {
			clear = true
		}
		if clear {
			d.proofs = map[string]storedProof{}
		}
		return
	}
	if !response.Extended || response.SiteChallenges == nil || d.site == "" {
		d.proofs = map[string]storedProof{}
		return
	}
	now := d.now()
	next := map[string]storedProof{}
	for _, challenge := range *response.SiteChallenges {
		if challenge.SiteURL != d.site {
			continue
		}
		remaining := time.Duration(challenge.ExpiresInMS)*time.Millisecond - response.RoundTrip - time.Second

		received := response.ReceivedAt
		if received.IsZero() {
			received = now
		}
		deadline := received.Add(remaining)
		if old, ok := d.proofs[challenge.ChallengeID]; ok {
			if old.value.Nonce != challenge.Nonce || old.value.ExpiresAt != challenge.ExpiresAt {
				next[challenge.ChallengeID] = old
				continue
			}
			if old.deadline.Before(deadline) {
				deadline = old.deadline
			}
		}

		next[challenge.ChallengeID] = storedProof{publicProof{challenge.Schema, challenge.ChallengeID, challenge.Nonce, challenge.ExpiresAt}, deadline}
	}
	d.proofs = next
}

// MarketplaceProof 只读指定挑战；不返回实例 ID、网址、环境摘要或凭证。
func (s *Service) MarketplaceProof(id string) (any, bool) {
	d := s.deployment
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.enabled || !s.client.DeploymentReady() || s.distribution != nil && s.distribution.authRejected.Load() {
		return nil, false
	}
	proof, ok := d.proofs[id]
	if !ok || !proof.deadline.After(d.now()) {
		return nil, false
	}
	return proof.value, true
}
