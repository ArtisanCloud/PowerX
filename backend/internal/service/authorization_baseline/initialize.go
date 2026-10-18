package authorization_baseline

import (
	"context"
	"encoding/json"
	"strings"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	ar "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"gorm.io/gorm"
)

// InitializationInput 仅供显式运维流程，不通过只读 capability 暴露。
// GuardUUID 是插件完成写入门禁和存量核对后交付的持久化收据标识，不能由浏览器提交。
type InitializationInput struct {
	TenantUUID      string
	PluginID        string
	PermissionCodes []string
	GuardUUID       string
	Operator        string
	Apply           bool
}
type InitializationResult struct {
	TenantUUID      string                         `json:"tenant_uuid"`
	PluginID        string                         `json:"plugin_id"`
	PermissionCodes []string                       `json:"permission_codes"`
	MemberCount     int                            `json:"member_count"`
	Applied         bool                           `json:"applied"`
	Tracking        map[string]InitializationState `json:"tracking"`
}
type InitializationState struct {
	Status     string  `json:"status"`
	KnownSince *string `json:"known_since"`
}

func declaredPermissions(db *gorm.DB, plugin string) (map[string]bool, error) {
	var rows []im.Permission
	if err := db.Where("source = ?", "plugin:"+plugin).Find(&rows).Error; err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	for _, row := range rows {
		var meta map[string]any
		if json.Unmarshal(row.Meta, &meta) == nil && meta["authorization_baseline"] == true {
			code := pluginpermission.EffectivePluginPermissionCode(row)
			if code != "" {
				declared[code] = true
			}
		}
	}
	return declared, nil
}

// Initialize 默认只预检；apply 必须由已经协调插件事件门禁的操作者显式执行。
// Core 不读取插件事件库，不能替插件确认不存在待保留事件。
func (s *Service) Initialize(ctx context.Context, in InitializationInput) (InitializationResult, error) {
	result := InitializationResult{TenantUUID: canonical(in.TenantUUID), PluginID: strings.TrimSpace(in.PluginID), PermissionCodes: in.PermissionCodes, Tracking: map[string]InitializationState{}}
	if result.TenantUUID == "" || result.PluginID == "" || len(in.PermissionCodes) == 0 || len(in.PermissionCodes) > 10 {
		return result, failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
	}
	_, err := validate(Query{Schema: QuerySchema, MemberUUIDs: []string{result.TenantUUID}, PermissionCodes: in.PermissionCodes})
	if err != nil {
		return result, err
	}
	if in.Apply && (canonical(in.GuardUUID) == "" || strings.TrimSpace(in.Operator) == "") {
		return result, failure(400, "IAM_BASELINE_INITIALIZATION_GUARD_REQUIRED")
	}
	if s == nil || s.db == nil {
		return result, failure(503, "IAM_BASELINE_DEPENDENCY_UNAVAILABLE")
	}
	if !baseline.SupportedDatabase(s.db) {
		return result, failure(409, "IAM_BASELINE_UNSUPPORTED_DATABASE")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.Apply {
			if err := baseline.Lock(tx); err != nil {
				return err
			}
		}
		declared, err := declaredPermissions(tx, result.PluginID)
		if err != nil {
			return err
		}
		for _, code := range in.PermissionCodes {
			if !declared[code] {
				return failure(400, "IAM_BASELINE_INVALID_ARGUMENT")
			}
		}
		facts, err := ar.NewRepository(tx).Load(ctx, result.TenantUUID, result.PluginID)
		if err != nil {
			return err
		}
		if !facts.TenantActive || !facts.PluginActive {
			return failure(409, "IAM_BASELINE_INITIALIZATION_SUBJECT_INACTIVE")
		}
		result.MemberCount = len(facts.Members)
		at := baseline.Micro(s.clock())
		if tx.Dialector.Name() == "postgres" {
			if err = tx.Raw("SELECT clock_timestamp()").Scan(&at).Error; err != nil {
				return err
			}
			at = baseline.Micro(at)
		}
		for _, code := range in.PermissionCodes {
			var row im.AuthorizationTracking
			err = tx.Where("tenant_uuid=? AND plugin_id=? AND permission_code=?", result.TenantUUID, result.PluginID, code).Take(&row).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			result.Tracking[code] = InitializationState{Status: "uninitialized"}
			if err == nil {
				result.Tracking[code] = InitializationState{Status: row.Status, KnownSince: timeValue(row.KnownSince)}
			}
			if !in.Apply {
				continue
			}
			if err == nil && row.Status == "ready" {
				if row.InitializationGuardUUID != canonical(in.GuardUUID) {
					return failure(409, "IAM_BASELINE_ALREADY_INITIALIZED")
				}
				if row.KnownSince == nil {
					return failure(503, "IAM_BASELINE_UNAVAILABLE")
				}
				continue
			}
			row.TenantUUID = result.TenantUUID
			row.PluginID = result.PluginID
			row.PermissionCode = code
			row.Status = "ready"
			row.KnownSince = &at
			row.InitializationGuardUUID = canonical(in.GuardUUID)
			row.InitializedBy = strings.TrimSpace(in.Operator)
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
			result.Tracking[code] = InitializationState{Status: "ready", KnownSince: timeValue(&at)}
		}
		if !in.Apply {
			return nil
		}
		if err = baseline.Reconcile(tx, at); err != nil {
			return err
		}
		return nil
	})
	result.Applied = err == nil && in.Apply
	return result, err
}
