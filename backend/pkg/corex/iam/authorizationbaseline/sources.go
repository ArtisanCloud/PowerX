package authorizationbaseline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
)

type Grant struct {
	Source     Source
	Permission im.Permission
	RoleCode   string
}

func bound(start, end *int64) (time.Time, *time.Time) {
	var from time.Time
	if start != nil {
		from = time.UnixMilli(*start).UTC()
	}
	var until *time.Time
	if end != nil {
		v := time.UnixMilli(*end).UTC()
		until = &v
	}
	return from, until
}
func intersect(a, b Source) (Source, bool) {
	if b.From.After(a.From) {
		a.From = b.From
	}
	if b.Until != nil && (a.Until == nil || b.Until.Before(*a.Until)) {
		a.Until = b.Until
	}
	return a, a.Until == nil || a.From.Before(*a.Until)
}
func hasCondition(raw []byte) bool {
	v := strings.TrimSpace(string(raw))
	return v != "" && v != "null" && v != "{}"
}
func conditionIdentity(raw []byte) any {
	if !hasCondition(raw) {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	return value
}

// Grants 是网关、身份与时间账本共用的来源解析。中间关系的数字键仅用于既有内部维度匹配。
func Grants(f ar.Facts, memberUUID string, memberID uint64) []Grant {
	roles := map[string]im.Role{}
	for _, r := range f.Roles {
		roles[r.UUID.String()] = r
	}
	perms := map[string]im.Permission{}
	for _, p := range f.Permissions {
		perms[p.PermissionUUID.String()] = p
	}
	byRole := map[string][]im.Permission{}
	for _, rp := range f.RolePermissions {
		if p, ok := perms[rp.PermissionUUID.String()]; ok {
			byRole[rp.RoleUUID.String()] = append(byRole[rp.RoleUUID.String()], p)
		}
	}
	out := []Grant{}
	// root 是 Core 身份派生的内置角色，不在业务角色绑定中人工分配。
	// 必须同时具备有效账号、当前租户的有效成员、正式内置角色及目录授权。
	// 来源仍进入统一决策/时间账本，不能把 is_root 直接替换为 allowed=true。
	for _, member := range f.Members {
		if member.UUID.String() != memberUUID || member.ID != memberID || member.Status != 1 || member.DeletedAt.Valid {
			continue
		}
		for _, user := range f.Users {
			if user.UUID.String() != member.UserUUID || user.ID != member.UserID || !user.IsRoot || user.Status != im.UserStatusActive || user.DeletedAt.Valid {
				continue
			}
			for _, role := range f.Roles {
				if !rootrole.IsManaged(role) || !role.Builtin || role.DeletedAt.Valid {
					continue
				}
				for _, permission := range byRole[role.UUID.String()] {
					source := Source{ID: rootSourcePrefix + Digest([]any{user.UUID, member.UUID, role.UUID, permission.PermissionUUID}), Scope: im.ScopeTenant, Effect: permission.Effect}
					var meta map[string]any
					if len(permission.Meta) > 0 && json.Unmarshal(permission.Meta, &meta) != nil {
						source.Conditional = true
					}
					if scope, ok := meta["data_scope"].(string); ok && scope != "" {
						source.Scope = im.DataScope(strings.ToUpper(scope))
					}
					out = append(out, Grant{Source: source, Permission: permission, RoleCode: string(role.Code)})
				}
			}
		}
	}
	for _, b := range f.Bindings {
		role, ok := roles[b.RoleUUID]
		if !ok {
			continue
		}
		if rootrole.IsManaged(role) {
			continue
		} // 任意人工绑定不能授予 root 身份。
		start, end := bound(b.StartAt, b.EndAt)
		scope := b.DataScope
		if scope == "" {
			scope = im.ScopeTenant
		}
		if (b.ScopeDim != nil || b.ScopeID != nil) && scope == im.ScopeTenant {
			scope = im.ScopeCustom
		}
		// 覆盖写可能删除并重建物理行。来源身份由正式语义确定，不能依赖自增主键。
		subject := fmt.Sprint(b.SubjectID)
		if b.SubjectType == im.SubMember {
			subject = b.SubjectUUID
		}
		source := Source{ID: "binding:" + Digest([]any{b.RoleUUID, b.SubjectType, subject, b.ScopeDim, b.ScopeID, conditionIdentity(b.Condition)}), From: start, Until: end, Scope: scope, Conditional: hasCondition(b.Condition)}
		paths := []Source{}
		if b.SubjectType == im.SubMember && b.SubjectUUID == memberUUID {
			paths = append(paths, source)
		}
		for _, a := range f.Assignments {
			if a.MemberID != memberID || a.DimID != b.SubjectID {
				continue
			}
			if f.ActiveDimensions != nil && !f.ActiveDimensions[a.DimType][a.DimID] {
				continue
			}
			dimension := map[im.AssignmentDim]im.SubjectType{im.DimOrg: im.SubOrg, im.DimTeam: im.SubTeam, im.DimPosition: im.SubPosition, im.DimGroup: im.SubGroup}
			if dimension[a.DimType] != b.SubjectType {
				continue
			}
			af, ae := bound(a.StartAt, a.EndAt)
			s, valid := intersect(source, Source{From: af, Until: ae})
			if !valid {
				continue
			}
			s.ID += fmt.Sprintf("/assignment:%s:%d", a.DimType, a.DimID)
			paths = append(paths, s)
		}
		for _, path := range paths {
			for _, p := range byRole[b.RoleUUID] {
				s := path
				s.ID += "/role:" + role.UUID.String() + "/permission:" + p.PermissionUUID.String()
				s.Effect = p.Effect
				var meta map[string]any
				if len(p.Meta) > 0 && json.Unmarshal(p.Meta, &meta) != nil {
					s.Conditional = true
				}
				if declared, ok := meta["data_scope"].(string); ok && declared != "" && !strings.EqualFold(declared, "TENANT") {
					s.Scope = im.DataScope(strings.ToUpper(declared))
				}
				out = append(out, Grant{Source: s, Permission: p, RoleCode: string(role.Code)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source.ID == out[j].Source.ID {
			return Digest(out[i].Source) < Digest(out[j].Source)
		}
		return out[i].Source.ID < out[j].Source.ID
	})
	unique := out[:0]
	seen := map[string]bool{}
	for _, grant := range out {
		key := Digest(grant.Source)
		if !seen[key] {
			unique = append(unique, grant)
			seen[key] = true
		}
	}
	return unique
}

const rootSourcePrefix = "root-role:"

func MemberSources(f ar.Facts, member im.Member, code string) ([]Source, bool) {
	var user *im.User
	for i := range f.Users {
		if f.Users[i].UUID.String() == member.UserUUID {
			user = &f.Users[i]
			break
		}
	}
	if !f.TenantActive || !f.PluginActive || member.Status != 1 || user == nil || user.Status != im.UserStatusActive || user.DeletedAt.Valid {
		return []Source{}, false
	}
	out := []Source{}
	for _, g := range Grants(f, member.UUID.String(), member.ID) {
		if pluginpermission.EffectivePluginPermissionCode(g.Permission) == code {
			out = append(out, g.Source)
		}
	}
	return out, user.IsRoot
}

// SnapshotPermissions 保留范围来源供业务下游处理，TENANT deny 优先；无法求值的策略不变成业务放行。
func SnapshotPermissions(f ar.Facts, member string, memberID uint64, at time.Time) ([]string, []im.Permission) {
	grants := Grants(f, member, memberID)
	roles := map[string]bool{}
	rows := map[string]im.Permission{}
	deny := map[string]bool{}
	for _, g := range grants {
		s := g.Source
		if at.Before(s.From) || (s.Until != nil && !at.Before(*s.Until)) {
			continue
		}
		code := pluginpermission.EffectivePluginPermissionCode(g.Permission)
		if code == "" {
			continue
		}
		if s.Effect == "deny" {
			deny[code] = true
			continue
		}
		if s.Effect == "allow" {
			roles[g.RoleCode] = true
			rows[code] = g.Permission
		}
	}
	codes := []string{}
	for code := range rows {
		if !deny[code] {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	permissions := []im.Permission{}
	for _, code := range codes {
		permissions = append(permissions, rows[code])
	}
	roleCodes := []string{}
	for code := range roles {
		roleCodes = append(roleCodes, code)
	}
	sort.Strings(roleCodes)
	return roleCodes, permissions
}
