package authorization_baseline

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	gr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db    *gorm.DB
	clock func() time.Time
}

func NewService(db *gorm.DB) *Service       { return &Service{db: db, clock: time.Now} }
func failure(status int, code string) error { return dto.NewErrorWithCode(status, code, code, nil) }
func format(t time.Time) string             { return baseline.Micro(t).Format("2006-01-02T15:04:05.000000Z") }
func timeValue(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := format(*t)
	return &s
}
func canonical(value string) string {
	u, e := uuid.Parse(strings.TrimSpace(value))
	if e != nil || u == uuid.Nil {
		return ""
	}
	return u.String()
}

var permissionCode = regexp.MustCompile(`^[a-zA-Z0-9_.-]+:[a-zA-Z0-9_.-]+$`)

func validate(in Query) (Query, error) {
	if in.Schema != QuerySchema || len(in.MemberUUIDs) == 0 || len(in.MemberUUIDs) > 100 || len(in.PermissionCodes) == 0 || len(in.PermissionCodes) > 10 || len(in.MemberUUIDs)*len(in.PermissionCodes) > 1000 {
		return in, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	in.MemberUUIDs = append([]string{}, in.MemberUUIDs...)
	seen := map[string]bool{}
	for i, id := range in.MemberUUIDs {
		v := canonical(id)
		if v == "" || seen[v] {
			return in, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
		}
		seen[v] = true
		in.MemberUUIDs[i] = v
	}
	seen = map[string]bool{}
	for _, code := range in.PermissionCodes {
		if !permissionCode.MatchString(code) || seen[code] {
			return in, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
		}
		seen[code] = true
	}
	return in, nil
}

func (s *Service) caller(ctx context.Context, db *gorm.DB) (string, string, error) {
	tenant := canonical(reqctx.GetTenantUUID(ctx))
	c := reqctx.GetClaims(ctx)
	if tenant == "" || c == nil || canonical(c.TenantUUID) != tenant || !cap.ServiceCredential(ctx) {
		return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
	}
	if c.Issuer == "powerx-sts" && (c.ExpiresAt == nil || !s.clock().Before(c.ExpiresAt.Time)) {
		return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
	}
	access, err := cap.NewGrantStatusService(db).CurrentAccess(ctx)
	if err != nil {
		status := 403
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			status = grant.Status
		}
		if status == 503 {
			return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
		}
		if status == 401 {
			return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
		}
		return "", "", failure(status, "IAM_BASELINE_SERVICE_FORBIDDEN")
	}
	if !access.Granted[CapabilityID] {
		return "", "", failure(403, "IAM_BASELINE_SERVICE_FORBIDDEN")
	}
	if c.Issuer == "powerx-sts" && c.PluginID != "" {
		return enabledCaller(db, tenant, c.PluginID)
	}
	key, err := gr.NewIntegrationGatewayAPIKeyRepository(db).FindActiveByHash(ctx, tenant, reqctx.AuthenticatedAPIKeyHash(ctx))
	if err != nil {
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	if key == nil {
		return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
	}
	if key.ExpiresAt != nil && !s.clock().Before(*key.ExpiresAt) {
		return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
	}
	var profile im.APIKeyProfile
	if err = db.Where("id=? AND tenant_uuid=?", key.ProfileID, tenant).Take(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
		}
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	if profile.Status != 1 {
		return "", "", failure(401, "IAM_BASELINE_SERVICE_CREDENTIAL_INVALID")
	}
	var grants []gw.IntegrationGatewayAPIKeyPermission
	if err = db.Where("api_key_uuid = ?", key.UUID).Find(&grants).Error; err != nil {
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	policy, err := owners.ResolveOwnerPolicy(key.PluginOwnerPolicy, grants)
	if err != nil {
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	candidates := map[string]bool{}
	for _, owner := range policy.Owners() {
		for _, g := range grants {
			if g.PluginID == owner && g.Effect == "allow" && g.Scope == APIKeyScope && g.Action == "read" && g.ResourceType == "capability" && g.ResourcePattern == APIKeyResource {
				candidates[owner] = true
			}
		}
	}
	if len(candidates) > 1 {
		return "", "", failure(403, "IAM_BASELINE_PLUGIN_AMBIGUOUS")
	}
	for plugin := range candidates {
		return enabledCaller(db, tenant, plugin)
	}
	return "", "", failure(403, "IAM_BASELINE_SERVICE_FORBIDDEN")
}

func enabledCaller(db *gorm.DB, tenant, plugin string) (string, string, error) {
	var tenantRow tm.Tenant
	err := db.Where("uuid=? AND status=?", tenant, tm.TenantStatusActive).Take(&tenantRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", failure(403, "IAM_BASELINE_SERVICE_FORBIDDEN")
	}
	if err != nil {
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	var instance sm.PluginInstanceConfig
	err = db.Where("tenant_uuid=? AND plugin_id=? AND key=? AND enabled=?", tenant, plugin, "auth.credentials", true).Take(&instance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", failure(403, "IAM_BASELINE_SERVICE_FORBIDDEN")
	}
	if err != nil {
		return "", "", failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	return tenant, plugin, nil
}

// ReadMemberBaselines 整批使用同一只读一致性快照，不补写授权历史。
func (s *Service) ReadMemberBaselines(ctx context.Context, in Query) (Result, error) {
	var out Result
	query, err := validate(in)
	if err != nil {
		return out, err
	}
	if s == nil || s.db == nil {
		return out, failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	if !baseline.SupportedDatabase(s.db) {
		return out, failure(503, "IAM_BASELINE_UNSUPPORTED_DATABASE")
	}
	options := &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	if s.db.Dialector.Name() == "sqlite" {
		options = &sql.TxOptions{ReadOnly: true}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tenant, plugin, err := s.caller(ctx, tx)
		if err != nil {
			return err
		}
		now := baseline.Micro(s.clock())
		if tx.Dialector.Name() == "postgres" {
			if err = tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
				return err
			}
			now = baseline.Micro(now)
		}
		out = Result{Schema: ResultSchema, TenantUUID: tenant, PluginID: plugin, EvaluatedAt: format(now), Items: []Item{}}
		var tracks []im.AuthorizationTracking
		if err = tx.Where("tenant_uuid=? AND plugin_id=? AND permission_code IN ?", tenant, plugin, query.PermissionCodes).Find(&tracks).Error; err != nil {
			return err
		}
		tracking := map[string]im.AuthorizationTracking{}
		for _, t := range tracks {
			tracking[t.PermissionCode] = t
		}
		declared, err := declaredPermissions(tx, plugin)
		if err != nil {
			return err
		}
		for _, code := range query.PermissionCodes {
			if !declared[code] {
				return failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
			}
		}
		var members []im.Member
		if err = tx.Where("tenant_uuid=? AND uuid IN ?", tenant, query.MemberUUIDs).Find(&members).Error; err != nil {
			return err
		}
		present := map[string]bool{}
		memberRows := map[string]im.Member{}
		for _, m := range members {
			present[m.UUID.String()] = true
			memberRows[m.UUID.String()] = m
		}
		facts, err := ar.NewRepository(tx).Load(ctx, tenant, plugin)
		if err != nil {
			return err
		}
		var streams []im.AuthorizationStream
		if err = tx.Where("tenant_uuid=? AND plugin_id=? AND member_uuid IN ? AND permission_code IN ?", tenant, plugin, query.MemberUUIDs, query.PermissionCodes).Find(&streams).Error; err != nil {
			return err
		}
		byKey := map[string]im.AuthorizationStream{}
		ids := []string{}
		for _, stream := range streams {
			if _, duplicate := byKey[stream.MemberUUID+"/"+stream.PermissionCode]; duplicate {
				return failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
			}
			byKey[stream.MemberUUID+"/"+stream.PermissionCode] = stream
			ids = append(ids, stream.UUID.String())
		}
		segments := []im.AuthorizationSegment{}
		if len(ids) > 0 {
			if err = tx.Where("tenant_uuid=? AND stream_uuid IN ? AND effective_from<=? AND (effective_until IS NULL OR effective_until>?)", tenant, ids, now, now).Find(&segments).Error; err != nil {
				return err
			}
		}
		active := map[string]im.AuthorizationSegment{}
		for _, segment := range segments {
			if _, dup := active[segment.StreamUUID]; dup {
				return failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
			}
			active[segment.StreamUUID] = segment
		}
		for _, member := range query.MemberUUIDs {
			for _, code := range query.PermissionCodes {
				item := Item{MemberUUID: member, PermissionCode: code, Status: "error"}
				if !present[member] {
					item.ErrorCode = "IAM_BASELINE_MEMBER_UNAVAILABLE"
				} else if tracking[code].Status != "ready" {
					item.ErrorCode = "IAM_BASELINE_UNAVAILABLE"
				} else if stream, ok := byKey[member+"/"+code]; !ok {
					item.ErrorCode = "IAM_BASELINE_UNAVAILABLE"
				} else if sources, root := baseline.MemberSources(facts, memberRows[member], code); stream.FactsHash != baseline.Digest([]any{sources, root}) {
					// 只读发现账本缺口，不能拿已失真的历史放行或现场补历史。
					item.ErrorCode = "IAM_BASELINE_UNAVAILABLE"
				} else if segment, ok := active[stream.UUID.String()]; !ok {
					item.ErrorCode = "IAM_BASELINE_UNAVAILABLE"
				} else if !reliableSegment(stream, segment) {
					item.ErrorCode = "IAM_BASELINE_UNAVAILABLE"
				} else if segment.State != baseline.Allowed && segment.State != baseline.Denied {
					item.ErrorCode = segment.State
				} else {
					allowed := segment.State == baseline.Allowed
					item.Status = "decided"
					item.Allowed = &allowed
					item.AuthorizationRevision = baseline.Revision(stream, segment)
					item.DataScope = "TENANT"
					item.ScopeFingerprint = stream.ScopeFingerprint
					item.RecheckAt = timeValue(segment.EffectiveUntil)
					if allowed {
						item.BaselineKind = segment.BaselineKind
						item.GrantedAt = timeValue(segment.GrantedAt)
						item.VisibleSince = timeValue(segment.VisibleSince)
						item.KnownSince = timeValue(&stream.KnownSince)
						item.ValidUntil = timeValue(segment.ValidUntil)
					}
				}
				out.Items = append(out.Items, item)
			}
		}
		return nil
	}, options)
	if err != nil {
		var coded *dto.AppError
		if errors.As(err, &coded) {
			return Result{}, err
		}
		return Result{}, failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	return out, nil
}

// 不把损坏或未知的保存状态转换为普通拒绝，也不输出合同未定义的错误码。
func reliableSegment(stream im.AuthorizationStream, segment im.AuthorizationSegment) bool {
	if stream.UUID == uuid.Nil || stream.FactRevision == 0 || stream.KnownSince.IsZero() || stream.ScopeFingerprint != baseline.ScopeFingerprint(stream.TenantUUID) || segment.UUID == uuid.Nil {
		return false
	}
	switch segment.State {
	case baseline.Denied, baseline.UnsupportedScope, baseline.UnsupportedPolicy, baseline.RootBypass, baseline.Unavailable:
		return true
	case baseline.Allowed:
	default:
		return false
	}
	if canonical(segment.EpochUUID) == "" || segment.VisibleSince == nil || segment.VisibleSince.Before(stream.KnownSince) || segment.VisibleSince.After(segment.EffectiveFrom) {
		return false
	}
	if segment.ValidUntil != nil && (!segment.EffectiveFrom.Before(*segment.ValidUntil) || (segment.EffectiveUntil != nil && segment.ValidUntil.Before(*segment.EffectiveUntil))) {
		return false
	}
	switch segment.BaselineKind {
	case "initialized":
		return segment.GrantedAt == nil
	case "native", "reconstructed":
		return segment.GrantedAt != nil && !segment.GrantedAt.IsZero() && !segment.GrantedAt.After(*segment.VisibleSince)
	default:
		return false
	}
}
