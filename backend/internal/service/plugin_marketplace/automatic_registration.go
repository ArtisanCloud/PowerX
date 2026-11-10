package plugin_marketplace

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/google/uuid"
)

// ConnectInstance 在启动 worker 内探测市场并自动登记，不阻塞 Core 启动或本地上传。
func (s *Service) ConnectInstance(ctx context.Context) error {
	d := s.distribution
	if d == nil || d.store == nil {
		return nil
	}
	if d.enrollment != "" {
		return s.connectEnrolledInstance(ctx)
	}
	d.connectMu.Lock()
	defer d.connectMu.Unlock()
	state := d.store.Identity()
	if d.registered() {
		return nil
	}
	if state.AuthRejected || d.authRejected.Load() {
		return marketStateError("MARKETPLACE_INSTANCE_REJECTED", "市场实例凭证已被拒绝，保留原身份和交付记录")
	}
	if _, err := s.client.Discover(ctx); err != nil {
		return d.automaticConnectionError(err)
	}
	credential := state.PendingInstanceToken
	if credential == "" {
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return marketStateError("MARKETPLACE_STATE_UNAVAILABLE", "无法生成实例凭证")
		}
		credential = "pxm_instance_" + base64.RawURLEncoding.EncodeToString(secret[:])
		if err := d.store.Update(func(st *market.DistributionState) error {
			st.PendingInstanceToken = credential
			st.RegistrationError = "MARKETPLACE_REGISTRATION_PENDING"
			return nil
		}); err != nil {
			return err
		}
	}
	name := state.EnvironmentID
	if len(name) > 12 {
		name = name[:12]
	}
	declaration, generation := s.deploymentDeclaration()
	result, err := s.client.RegisterAutomaticWithDeployment(ctx, credential, state.Fingerprint, s.coreVersion, "PowerX "+name, declaration)
	if err != nil {
		s.acceptDeployment(result.DeploymentResponse, generation, err)
		return d.automaticConnectionError(err)
	}
	if _, err := uuid.Parse(result.Instance.ID); err != nil {
		return d.automaticConnectionError(marketStateError("MARKETPLACE_REGISTRATION_INVALID", "市场实例 ID 无效"))
	}
	if err := d.store.Update(func(st *market.DistributionState) error {
		for _, a := range st.Attempts {
			if a.InstanceID != result.Instance.ID {
				a.Blocked = "MARKETPLACE_PREVIOUS_INSTANCE"
			}
		}
		st.InstanceID, st.InstanceToken = result.Instance.ID, credential
		st.PendingInstanceToken, st.RegistrationError = "", ""
		st.AuthRejected = false
		return nil
	}); err != nil {
		// 远端可能已经登记；预先保存的凭证继续用于下次幂等重试。
		return err
	}
	s.client.SetInstance(credential, state.Fingerprint)
	s.acceptDeployment(result.DeploymentResponse, generation, nil)
	return nil
}

func (d *distribution) automaticConnectionError(cause error) error {
	if err := d.store.Update(func(st *market.DistributionState) error {
		st.RegistrationError = "MARKETPLACE_REGISTRATION_UNAVAILABLE"
		if market.AuthenticationRejected(cause) {
			st.AuthRejected = true
		}
		return nil
	}); err != nil {
		return err
	}
	return cause
}
