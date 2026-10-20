package authorizationbaseline

import (
	"errors"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Reconcile 只在共同事务边界的生命周期初始化或源事务提交前调用，不能从 GET 调用。
func Reconcile(db *gorm.DB, at time.Time) error {
	var tracks []im.AuthorizationTracking
	if err := db.Where("status = ?", "ready").Order("tenant_uuid, plugin_id, permission_code").Find(&tracks).Error; err != nil {
		return err
	}
	return reconcileTracks(db, at, tracks)
}

func reconcileTracks(db *gorm.DB, at time.Time, tracks []im.AuthorizationTracking) error {
	return reconcileTracksWhere(db, at, tracks, nil)
}

func reconcileTracksWhere(db *gorm.DB, at time.Time, tracks []im.AuthorizationTracking, accept func([]Source, bool, im.AuthorizationStream, bool) bool) error {
	cache := map[string]ar.Facts{}
	for _, track := range tracks {
		key := track.TenantUUID + "/" + track.PluginID
		facts, ok := cache[key]
		if !ok {
			var err error
			facts, err = ar.NewRepository(db).Load(db.Statement.Context, track.TenantUUID, track.PluginID)
			if err != nil {
				return err
			}
			cache[key] = facts
		}
		if track.KnownSince == nil {
			return errors.New("tracking ready without known_since")
		}
		var streams []im.AuthorizationStream
		if err := db.Where("tenant_uuid=? AND plugin_id=? AND permission_code=?", track.TenantUUID, track.PluginID, track.PermissionCode).Find(&streams).Error; err != nil {
			return err
		}
		byMember := map[string]im.AuthorizationStream{}
		for _, s := range streams {
			byMember[s.MemberUUID] = s
		}
		members := map[string]im.Member{}
		for _, m := range facts.Members {
			members[m.UUID.String()] = m
		}
		for id := range byMember {
			if _, ok := members[id]; !ok {
				members[id] = im.Member{}
			}
		}
		for id, member := range members {
			sources, root := MemberSources(facts, member, track.PermissionCode)
			if member.UUID == uuid.Nil {
				sources = []Source{}
				root = false
			}
			hash := Digest([]any{sources, root})
			stream, exists := byMember[id]
			if exists && stream.FactsHash == hash {
				continue
			}
			if accept != nil && !accept(sources, root, stream, exists) {
				continue
			}
			var previous *im.AuthorizationSegment
			if exists {
				var segment im.AuthorizationSegment
				err := db.Where("tenant_uuid=? AND stream_uuid=? AND effective_from<=? AND (effective_until IS NULL OR effective_until>?)", stream.TenantUUID, stream.UUID.String(), at, at).Order("effective_from DESC").Take(&segment).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				if err == nil {
					previous = &segment
				}
			} else {
				stream = im.AuthorizationStream{TenantUUID: track.TenantUUID, PluginID: track.PluginID, MemberUUID: id, PermissionCode: track.PermissionCode, ScopeFingerprint: ScopeFingerprint(track.TenantUUID), KnownSince: *track.KnownSince}
				stream.UUID = uuid.New()
			}
			stream.FactsHash = hash
			stream.FactRevision++
			plan := Plan(sources, at, root)
			initial := !exists && at.Equal(*track.KnownSince)
			AssignEpochs(plan, previous, initial)

			if exists {
				if err := db.Where("tenant_uuid=? AND stream_uuid=? AND effective_from>=?", stream.TenantUUID, stream.UUID.String(), at).Unscoped().Delete(&im.AuthorizationSegment{}).Error; err != nil {
					return err
				}
				if err := db.Model(&im.AuthorizationSegment{}).Where("tenant_uuid=? AND stream_uuid=? AND effective_from<? AND (effective_until IS NULL OR effective_until>?)", stream.TenantUUID, stream.UUID.String(), at, at).Update("effective_until", at).Error; err != nil {
					return err
				}
				// 真正事实变化采用新决策身份，当前仍允许时保留 epoch。
				plan[0].UUID = uuid.New()
				if err := db.Model(&im.AuthorizationStream{}).Where("tenant_uuid=? AND uuid=?", stream.TenantUUID, stream.UUID).Updates(map[string]any{"facts_hash": stream.FactsHash, "fact_revision": stream.FactRevision}).Error; err != nil {
					return err
				}
			} else if err := db.Create(&stream).Error; err != nil {
				return err
			}
			for i := range plan {
				plan[i].TenantUUID = stream.TenantUUID
				plan[i].StreamUUID = stream.UUID.String()
			}
			if len(plan) > 0 {
				if err := db.Create(&plan).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}
