package authorizationbaseline

import (
	"sort"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	"gorm.io/gorm"
)

// RootCatalogRepairHistory 是维护人实际确认的历史变更范围。历史指纹包含
// 原追踪、原流和全部原历史段；不把预览时间当成新的授权起点。
type RootCatalogRepairHistory struct {
	TenantUUID         string    `json:"tenant_uuid"`
	PluginID           string    `json:"plugin_id"`
	MemberUUID         string    `json:"member_uuid"`
	PermissionCode     string    `json:"permission_code"`
	StreamUUID         string    `json:"stream_uuid"`
	BeforeRevision     uint64    `json:"before_revision"`
	AfterRevision      uint64    `json:"after_revision"`
	BeforeFactsHash    string    `json:"before_facts_hash"`
	AfterFactsHash     string    `json:"after_facts_hash"`
	KnownSince         time.Time `json:"known_since"`
	HistoryFingerprint string    `json:"history_fingerprint"`
	Reason             string    `json:"reason,omitempty"`
}

func inspectRootRepairHistory(db *gorm.DB, plan *RootCatalogRepairPlan) ([]RootCatalogRepairHistory, []RootCatalogRepairHistory, error) {
	changes, blocked := []RootCatalogRepairHistory{}, []RootCatalogRepairHistory{}
	if len(plan.Candidates) == 0 {
		return changes, blocked, nil
	}
	var tracks []im.AuthorizationTracking
	if err := db.Where("status=?", "ready").Order("tenant_uuid, plugin_id, permission_code").Find(&tracks).Error; err != nil {
		return nil, nil, err
	}
	cache := map[string]ar.Facts{}
	for _, track := range tracks {
		key := track.TenantUUID + "/" + track.PluginID
		facts, ok := cache[key]
		if !ok {
			var err error
			facts, err = ar.NewRepository(db).Load(db.Statement.Context, track.TenantUUID, track.PluginID)
			if err != nil {
				return nil, nil, err
			}
			// 正式仓储不加载悬空角色的关联。只在内存中投影已确认的候选，
			// 仍由统一 Grants 过滤当前插件有效权限，预览不执行任何源写入。
			for _, candidate := range plan.Candidates {
				facts.RolePermissions = append(facts.RolePermissions, im.RolePermission{RoleID: plan.RootRoleID, RoleUUID: plan.RootUUID, PermissionID: candidate.PermissionID, PermissionUUID: candidate.PermissionUUID, CreatedAt: candidate.CreatedAt})
			}
			cache[key] = facts
		}
		var streams []im.AuthorizationStream
		if err := db.Where("tenant_uuid=? AND plugin_id=? AND permission_code=?", track.TenantUUID, track.PluginID, track.PermissionCode).Find(&streams).Error; err != nil {
			return nil, nil, err
		}
		byMember := map[string]im.AuthorizationStream{}
		for _, stream := range streams {
			byMember[stream.MemberUUID] = stream
		}
		for _, member := range facts.Members {
			sources, root := MemberSources(facts, member, track.PermissionCode)
			if !root {
				continue
			}
			stream, exists := byMember[member.UUID.String()]
			hash := Digest([]any{sources, root})
			if exists && stream.FactsHash == hash {
				continue
			}
			item := RootCatalogRepairHistory{TenantUUID: track.TenantUUID, PluginID: track.PluginID, MemberUUID: member.UUID.String(), PermissionCode: track.PermissionCode, StreamUUID: stream.UUID.String(), BeforeRevision: stream.FactRevision, AfterRevision: stream.FactRevision + 1, BeforeFactsHash: stream.FactsHash, AfterFactsHash: hash, KnownSince: stream.KnownSince}
			var segments []im.AuthorizationSegment
			if exists {
				if err := db.Where("tenant_uuid=? AND stream_uuid=?", stream.TenantUUID, stream.UUID.String()).Order("effective_from, uuid").Find(&segments).Error; err != nil {
					return nil, nil, err
				}
			}
			item.HistoryFingerprint = Digest([]any{track, stream, segments})
			if !exists || track.KnownSince == nil || !canUpgradeRootSources(sources, root, stream, exists) {
				item.Reason = "ROOT_HISTORY_UNRECOGNIZED"
				blocked = append(blocked, item)
			} else {
				changes = append(changes, item)
			}
		}
	}
	less := func(a, b RootCatalogRepairHistory) bool {
		return a.TenantUUID+"/"+a.PluginID+"/"+a.PermissionCode+"/"+a.MemberUUID < b.TenantUUID+"/"+b.PluginID+"/"+b.PermissionCode+"/"+b.MemberUUID
	}
	sort.Slice(changes, func(i, j int) bool { return less(changes[i], changes[j]) })
	sort.Slice(blocked, func(i, j int) bool { return less(blocked[i], blocked[j]) })
	return changes, blocked, nil
}

func applyRootRepairHistory(db *gorm.DB, at time.Time, changes []RootCatalogRepairHistory) error {
	approved := map[string]RootCatalogRepairHistory{}
	tracks := []im.AuthorizationTracking{}
	seen := map[string]bool{}
	for _, change := range changes {
		approved[change.StreamUUID] = change
		key := change.TenantUUID + "/" + change.PluginID + "/" + change.PermissionCode
		if seen[key] {
			continue
		}
		seen[key] = true
		var track im.AuthorizationTracking
		if err := db.Where("tenant_uuid=? AND plugin_id=? AND permission_code=? AND status=?", change.TenantUUID, change.PluginID, change.PermissionCode, "ready").Take(&track).Error; err != nil {
			return err
		}
		tracks = append(tracks, track)
	}
	if err := reconcileTracksWhere(db, at, tracks, func(sources []Source, root bool, stream im.AuthorizationStream, exists bool) bool {
		change, ok := approved[stream.UUID.String()]
		return ok && stream.FactRevision == change.BeforeRevision && stream.FactsHash == change.BeforeFactsHash && Digest([]any{sources, root}) == change.AfterFactsHash && canUpgradeRootSources(sources, root, stream, exists)
	}); err != nil {
		return err
	}
	for _, change := range changes {
		var stream im.AuthorizationStream
		if err := db.Where("tenant_uuid=? AND uuid=?", change.TenantUUID, change.StreamUUID).Take(&stream).Error; err != nil {
			return err
		}
		if stream.FactRevision != change.AfterRevision || stream.FactsHash != change.AfterFactsHash || !stream.KnownSince.Equal(change.KnownSince) {
			return ErrRootRepairPlanConflict
		}
	}
	return nil
}
