package plugin_identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	rbac "github.com/ArtisanCloud/PowerX/internal/service/iam"
	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	plugin "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	igw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	igwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Options struct {
	Namespace    string
	UserIssuer   string
	UserAudience string
}
type Service struct {
	db    *gorm.DB
	repo  *iamrepo.PluginIdentityRepository
	store cache.ICache
	opts  Options
	clock func() time.Time
}
type proof struct {
	Version            int       `json:"version"`
	PluginID           string    `json:"plugin_id"`
	TenantUUID         string    `json:"tenant_uuid"`
	UserUUID           string    `json:"user_uuid"`
	MemberUUID         string    `json:"member_uuid"`
	SessionJTI         string    `json:"session_jti"`
	IsRoot             bool      `json:"is_root"`
	ExpiresAt          time.Time `json:"expires_at"`
	UserExpiresAt      time.Time `json:"user_expires_at"`
	Method             string    `json:"method"`
	Path               string    `json:"path"`
	Transport          string    `json:"transport"`
	RequestID          string    `json:"request_id"`
	TraceID            string    `json:"trace_id"`
	RequiredPermission string    `json:"required_permission"`
}

var ticketPattern = regexp.MustCompile(`^pxctx_v1_[A-Za-z0-9_-]{43}$`)
var pluginPattern = regexp.MustCompile(`^[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)+$`)

func NewService(db *gorm.DB, store cache.ICache, opts Options) *Service {
	return &Service{db: db, repo: iamrepo.NewPluginIdentityRepository(db), store: store, opts: opts, clock: time.Now}
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *Service) key(ticket string) string {
	return "iam:request-identity:v1:" + s.opts.Namespace + ":" + hash(ticket)
}
func canonical(value string) string {
	u, e := uuid.Parse(value)
	if e != nil || u == uuid.Nil {
		return ""
	}
	return u.String()
}
func has(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func (s *Service) ready() bool {
	return s != nil && s.db != nil && s.store != nil && s.opts.Namespace != ""
}

// Issue is called only by the Core gateway after user and route authorization.
// It deliberately accepts verified claims, never a browser-provided identity DTO.
func (s *Service) Issue(ctx context.Context, in IssueInput) (string, error) {
	if !s.ready() {
		return "", failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	c := in.Claims
	now := s.clock().UTC()
	if c.Issuer != s.opts.UserIssuer || !has(c.Audience, s.opts.UserAudience) || c.Scope != "access" || c.ExpiresAt == nil || !now.Before(c.ExpiresAt.Time) || c.ID == "" {
		return "", failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
	}
	p := proof{Version: 1, PluginID: in.PluginID, TenantUUID: canonical(c.TenantUUID), UserUUID: canonical(c.UserUUID), MemberUUID: canonical(c.MemberUUID), SessionJTI: c.ID, IsRoot: c.IsRoot, UserExpiresAt: c.ExpiresAt.Time.UTC(), Method: in.Method, Path: in.Path, Transport: in.Transport, RequestID: in.RequestID, TraceID: in.TraceID}
	p.RequiredPermission = in.RequiredPermission
	if !pluginPattern.MatchString(p.PluginID) || p.TenantUUID == "" || p.UserUUID == "" || p.Path == "" || p.Method == "" || in.TraceID == "" || in.RequestID == "" {
		return "", failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
	}
	if p.Transport != "http" && p.Transport != "ws_grant" && p.Transport != "ws_upgrade" {
		return "", failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	if _, _, err := s.subject(ctx, p); err != nil {
		return "", err
	}
	p.ExpiresAt = now.Add(time.Minute)
	if p.UserExpiresAt.Before(p.ExpiresAt) {
		p.ExpiresAt = p.UserExpiresAt
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	ticket := "pxctx_v1_" + base64.RawURLEncoding.EncodeToString(random)
	raw, _ := json.Marshal(p)
	if err := s.store.Set(ctx, s.key(ticket), raw, p.ExpiresAt.Sub(now)); err != nil {
		return "", failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	logger.InfoF(ctx, "iam_request_identity_issued plugin_id=%s tenant_uuid=%s source_request_id=%s source_trace_id=%s transport=%s", p.PluginID, p.TenantUUID, p.RequestID, p.TraceID, p.Transport)
	return ticket, nil
}

func (s *Service) subject(ctx context.Context, p proof) (models.User, models.Member, error) {
	var member models.Member
	now := s.clock()
	if !now.Before(p.UserExpiresAt) {
		return models.User{}, member, failure(401, "IAM_CONTEXT_USER_SESSION_EXPIRED")
	}
	session, err := s.repo.Session(ctx, p.SessionJTI)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.User{}, member, failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
	}
	if err != nil {
		return models.User{}, member, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if session.RevokedAt != nil || session.ExpiresAt <= now.UnixMilli() || session.UserUUID != p.UserUUID || session.TenantUUID != p.TenantUUID || session.MemberUUID != p.MemberUUID {
		return models.User{}, member, failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
	}
	user, err := s.repo.User(ctx, p.UserUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, member, failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
	}
	if err != nil {
		return user, member, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if user.Status != models.UserStatusActive || user.IsRoot != p.IsRoot {
		return user, member, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
	}
	ten, err := s.repo.Tenant(ctx, p.TenantUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, member, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
	}
	if err != nil {
		return user, member, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if ten.Status != 1 {
		return user, member, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
	}
	if !user.IsRoot || p.MemberUUID != "" {
		if p.MemberUUID == "" {
			return user, member, failure(403, "IAM_CONTEXT_MEMBER_REQUIRED")
		}
		member, err = s.repo.Member(ctx, p.TenantUUID, p.UserUUID, p.MemberUUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user, member, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
		}
		if err != nil {
			return user, member, failure(503, "IAM_CONTEXT_UNAVAILABLE")
		}
		if member.Status != 1 {
			return user, member, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
		}
	}
	if err := plugin.NewTenantPluginInstanceService(s.db).RequireEnabled(ctx, p.TenantUUID, p.PluginID); err != nil {
		if dto.StatusCode(err) == 403 {
			return user, member, failure(403, "IAM_CONTEXT_PLUGIN_FORBIDDEN")
		}
		return user, member, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	return user, member, nil
}

func (s *Service) serviceActor(ctx context.Context) (string, error) {
	c := reqctx.GetClaims(ctx)
	tenant := canonical(reqctx.GetTenantUUID(ctx))
	if c == nil || tenant == "" || canonical(c.TenantUUID) != tenant || !cap.ServiceCredential(ctx) {
		return "", failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID")
	}
	if c.Issuer == "powerx-sts" && (c.ExpiresAt == nil || !s.clock().Before(c.ExpiresAt.Time)) {
		return "", failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED")
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		status := 403
		var direct *cap.DirectGrantError
		if errors.As(err, &direct) {
			status = direct.Status
		}
		switch status {
		case 401:
			return "", failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID")
		case 503:
			return "", failure(503, "IAM_CONTEXT_UNAVAILABLE")
		default:
			return "", failure(403, "IAM_CONTEXT_SERVICE_FORBIDDEN")
		}
	}
	if !access.Granted[CapabilityID] {
		return "", failure(403, "IAM_CONTEXT_SERVICE_FORBIDDEN")
	}
	return tenant, nil
}
func (s *Service) owner(ctx context.Context, tenant, pluginID string) error {
	c := reqctx.GetClaims(ctx)
	if c.Issuer == "powerx-sts" {
		if c.PluginID != pluginID {
			return failure(403, "IAM_CONTEXT_PLUGIN_FORBIDDEN")
		}
		return nil
	}
	key, err := igwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenant, reqctx.AuthenticatedAPIKeyHash(ctx))
	if err != nil {
		return failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if key == nil {
		return failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID")
	}
	if key.ExpiresAt != nil && !s.clock().Before(*key.ExpiresAt) {
		return failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED")
	}
	var grants []igw.IntegrationGatewayAPIKeyPermission
	if err = s.db.WithContext(ctx).Where("api_key_uuid=?", key.UUID).Find(&grants).Error; err != nil {
		return failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	policy, err := owners.ResolveOwnerPolicy(key.PluginOwnerPolicy, grants)
	if err != nil {
		return failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if !has(policy.Owners(), pluginID) {
		return failure(403, "IAM_CONTEXT_PLUGIN_FORBIDDEN")
	}
	if policy.Mode == "legacy_permissions" {
		matched := []igw.IntegrationGatewayAPIKeyPermission{}
		for _, g := range grants {
			if g.PluginID == pluginID {
				matched = append(matched, g)
			}
		}
		if !igwrepo.APIKeyPermissionGranted(matched, APIKeyScope, "read", "capability", "iam_identity_context_read") {
			return failure(403, "IAM_CONTEXT_PLUGIN_FORBIDDEN")
		}
	}
	profile, err := iamrepo.NewAPIKeyProfileRepository(s.db).GetById(ctx, key.ProfileID, nil)
	if err != nil {
		return failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if profile == nil || profile.Status != 1 || profile.TenantUUID != tenant {
		return failure(401, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID")
	}
	return nil
}
func (s *Service) Resolve(ctx context.Context, in Query) (Result, error) {
	return s.resolve(ctx, in, false)
}
func (s *Service) ResolveAdmin(ctx context.Context, in Query) (Result, error) {
	return s.resolve(ctx, in, true)
}
func (s *Service) resolve(ctx context.Context, in Query, admin bool) (Result, error) {
	var out Result
	if !s.ready() {
		return out, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	tenant, err := canonical(reqctx.GetTenantUUID(ctx)), error(nil)
	if !admin {
		tenant, err = s.serviceActor(ctx)
	} else {
		c := reqctx.GetClaims(ctx)
		if c == nil || cap.ServiceCredential(ctx) || c.Issuer != s.opts.UserIssuer || !has(c.Audience, s.opts.UserAudience) || tenant == "" {
			err = failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
		}
	}
	if err != nil {
		return out, err
	}
	if in.Schema != QuerySchema {
		return out, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	if in.ContextTicket == "" {
		return out, failure(401, "IAM_CONTEXT_PROOF_REQUIRED")
	}
	if !ticketPattern.MatchString(in.ContextTicket) {
		return out, failure(401, "IAM_CONTEXT_PROOF_INVALID")
	}
	raw, err := s.store.Get(ctx, s.key(in.ContextTicket))
	if err != nil {
		return out, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if len(raw) == 0 {
		return out, failure(401, "IAM_CONTEXT_PROOF_EXPIRED")
	}
	var p proof
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 {
		return out, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if !s.clock().Before(p.ExpiresAt) {
		return out, failure(401, "IAM_CONTEXT_PROOF_EXPIRED")
	}
	if p.TenantUUID != tenant {
		return out, failure(403, "IAM_CONTEXT_TENANT_FORBIDDEN")
	}
	if !admin {
		if err = s.owner(ctx, tenant, p.PluginID); err != nil {
			return out, err
		}
	} else {
		c := reqctx.GetClaims(ctx)
		if canonical(c.UserUUID) != p.UserUUID || c.ID != p.SessionJTI {
			return out, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
		}
	}
	out, err = s.resolveProof(ctx, p, admin)
	if err != nil {
		return Result{}, err
	}
	if !admin && (p.Transport == "ws_grant" || p.Transport == "ws_upgrade") {
		handle, err := s.issueWSHandle(ctx, in.ContextTicket, p)
		if err != nil {
			return Result{}, err
		}
		out.WSRevalidationHandle = handle
		out.WSRevalidationExpiresAt = p.UserExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return out, nil
}

// resolveProof 是请求解析与 WS 消息复核的同一权威检查路径。
// 它读取实时会话、成员、插件、角色和权限，并在返回前再次围住撤销竞争。
func (s *Service) resolveProof(ctx context.Context, p proof, admin bool) (Result, error) {
	var out Result
	tenant := p.TenantUUID
	user, member, err := s.subject(ctx, p)
	if err != nil {
		return out, err
	}
	if admin {
		allowed, e := rbac.NewRBACService(s.db).EnforceWithSource(ctx, rbac.ActorContext{TenantUUID: tenant, IsRoot: user.IsRoot}, tenant, member.ID, "iam", "identity_context", "read", "platform_capability")
		if e != nil {
			return out, failure(503, "IAM_CONTEXT_UNAVAILABLE")
		}
		if !allowed {
			return out, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
		}
	}
	roles, rows, err := s.repo.Snapshot(ctx, p.TenantUUID, p.MemberUUID, member.ID, p.PluginID, user.IsRoot, s.clock())
	if err != nil {
		return out, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	permissions := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		code := pluginpermission.EffectivePluginPermissionCode(row)
		if code != "" && !seen[code] {
			seen[code] = true
			permissions = append(permissions, code)
		}
	}
	sort.Strings(roles)
	sort.Strings(permissions)
	if !user.IsRoot && p.RequiredPermission != "" && p.RequiredPermission != "runtime.contract:tenant_context" && !has(permissions, p.RequiredPermission) {
		return out, failure(403, "IAM_CONTEXT_REQUEST_FORBIDDEN")
	}
	state, _ := json.Marshal([]any{p.PluginID, p.TenantUUID, p.UserUUID, p.MemberUUID, user.IsRoot, roles, permissions})
	out = Result{Schema: ResultSchema, Mode: "delegated", PluginID: p.PluginID, TenantUUID: p.TenantUUID, UserUUID: p.UserUUID, IsRoot: user.IsRoot, Roles: roles, Permissions: permissions, PolicyVersion: hash(string(state)), PermsHash: hash(strings.Join(permissions, "\n")), SourceMethod: p.Method, SourcePath: p.Path, Transport: p.Transport, SourceRequestID: p.RequestID, SourceTraceID: p.TraceID, TraceID: reqctx.GetTraceID(ctx), ProofExpiresAt: p.ExpiresAt.UTC().Format(time.RFC3339Nano), UserSessionExpiresAt: p.UserExpiresAt.UTC().Format(time.RFC3339Nano)}
	if p.MemberUUID != "" {
		id := p.MemberUUID
		out.MemberUUID = &id
	}
	out.SourceRequiredPermission = p.RequiredPermission
	// Live grant and subject checks fence revocation that raced snapshot hydration.
	if !admin {
		if _, err = s.serviceActor(ctx); err != nil {
			return Result{}, err
		}
	}
	if !admin {
		if err = s.owner(ctx, tenant, p.PluginID); err != nil {
			return Result{}, err
		}
	}
	if _, _, err = s.subject(ctx, p); err != nil {
		return Result{}, err
	}
	latestRoles, latestRows, e := s.repo.Snapshot(ctx, p.TenantUUID, p.MemberUUID, member.ID, p.PluginID, user.IsRoot, s.clock())
	if e != nil {
		return Result{}, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	latestPermissions := []string{}
	latestSeen := map[string]bool{}
	for _, row := range latestRows {
		code := pluginpermission.EffectivePluginPermissionCode(row)
		if code != "" && !latestSeen[code] {
			latestSeen[code] = true
			latestPermissions = append(latestPermissions, code)
		}
	}
	sort.Strings(latestRoles)
	sort.Strings(latestPermissions)
	latestState, _ := json.Marshal([]any{p.PluginID, p.TenantUUID, p.UserUUID, p.MemberUUID, user.IsRoot, latestRoles, latestPermissions})
	if hash(string(latestState)) != out.PolicyVersion {
		return Result{}, failure(409, "IAM_CONTEXT_POLICY_CHANGED")
	}
	logger.InfoF(ctx, "iam_request_identity_resolved plugin_id=%s tenant_uuid=%s source_request_id=%s source_trace_id=%s transport=%s", p.PluginID, p.TenantUUID, p.RequestID, p.TraceID, p.Transport)
	return out, nil
}
