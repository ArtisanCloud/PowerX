package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ======= 对外返回结构（供 Handler 使用） =======
type MeMemberBrief struct {
	TenantUUID   string `json:"tenant_uuid"`
	TenantKey    string `json:"tenant_key"`
	TenantName   string `json:"tenant_name"`
	TenantDomain string `json:"tenant_domain"`
	MemberID     uint64 `json:"member_id"`
	MemberUUID   string `json:"member_uuid"`
	IsAdmin      bool   `json:"is_admin"`
	IsOwner      bool   `json:"is_owner"`
}

type MeUserBrief struct {
	ID          uint64 `json:"id"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Status      int16  `json:"status"`
	IsRoot      bool   `json:"is_root"`
	LastLoginAt *int64 `json:"last_login_at,omitempty"`
}

type MeContextResp struct {
	IsRoot            bool            `json:"is_root"`
	CurrentTenantUUID string          `json:"current_tenant_uuid"`
	CurrentMemberID   *uint64         `json:"current_member_id,omitempty"`
	CurrentMemberUUID string          `json:"current_member_uuid,omitempty"`
	User              *MeUserBrief    `json:"user,omitempty"`
	Members           []MeMemberBrief `json:"members"`
}

type UpdateMyProfileInput struct {
	DisplayName *string
	Email       *string
	Phone       *string
	AvatarURL   *string
}

// ======= Service =======
type MeService struct {
	*service.BaseService
	UserRepo   *repo.UserRepository
	MemberRepo *repo.MemberRepository
	TenantRepo *repotenant.TenantRepository
}

func NewMeService(db *gorm.DB) *MeService {
	return &MeService{
		BaseService: &service.BaseService{
			DB: db,
		},
		UserRepo:   repo.NewUserRepository(db),
		MemberRepo: repo.NewMemberRepository(db),
		TenantRepo: repotenant.NewTenantRepository(db),
	}
}

// GetMeContext 业务逻辑：从 ctx 解析身份 → 加载画像与成员列表 → 组装返回
func (s *MeService) GetMeContext(ctx context.Context) (*MeContextResp, error) {
	userID := reqctx.GetUserID(ctx)
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	memberID := reqctx.GetMemberID(ctx)
	memberUUID := strings.TrimSpace(reqctx.GetMemberUUID(ctx))

	var currentMemberID *uint64
	if memberID != 0 {
		currentMemberID = &memberID
	}

	// 1) 用户画像（可为空）
	var (
		userBrief *MeUserBrief
		isRoot    bool // ✅ 不再用 auth.IsRoot(ctx)，直接以 DB 为准
	)
	if userID != 0 {
		if u, err := s.UserRepo.FindByID(ctx, userID); err == nil && u != nil {
			userBrief = &MeUserBrief{
				ID:          u.ID,
				Email:       u.Email,
				Phone:       u.Phone,
				DisplayName: u.DisplayName,
				AvatarURL:   u.AvatarURL,
				Status:      u.Status,
				LastLoginAt: u.LastLoginAt,
				IsRoot:      u.IsRoot,
			}
			isRoot = u.IsRoot
		}
	}

	// 2) 该 User 加入过的所有租户成员
	members, err := s.MemberRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, dto.NewError(http.StatusInternalServerError, "查询成员失败", err) // 见下方注：你若没有 dto.NewError，可直接 return err
	}

	// 3) 批量取租户名
	tenantUUIDs := make([]string, 0, len(members))
	for _, mem := range members {
		tenantUUIDs = append(tenantUUIDs, mem.TenantUUID)
	}
	tenantBasicMap, _ := s.TenantRepo.MapBasicByUUIDs(ctx, tenantUUIDs)

	// 4) 先批量计算成员是否具备租户管理员/所有者角色
	adminMemberIDSet := map[uint64]struct{}{}
	ownerMemberIDSet := map[uint64]struct{}{}
	if len(members) > 0 {
		memberIDs := make([]uint64, 0, len(members))
		for _, mem := range members {
			memberIDs = append(memberIDs, mem.ID)
		}

		tRB := (&modelIAM.RoleBinding{}).GetTableName(true)
		tRole := (&modelIAM.Role{}).GetTableName(true)

		var rows []struct {
			SubjectID uint64 `gorm:"column:subject_id"`
			Code      string `gorm:"column:code"`
		}
		err := s.DB.WithContext(ctx).
			Table(tRB+" AS rb").
			Select("rb.subject_id, r.code").
			Joins("JOIN "+tRole+" AS r ON r.id = rb.role_id").
			Where("rb.subject_type = ? AND rb.subject_id IN ?", modelIAM.SubMember, memberIDs).
			Where(
				"r.scope = ? AND r.code IN ?",
				string(coreiam.RoleScopeTenant),
				[]string{string(coreiam.CodeRoleAdmin), "role_owner"},
			).
			Scan(&rows).Error
		if err != nil {
			return nil, dto.NewError(http.StatusInternalServerError, "查询成员管理员角色失败", err)
		}
		for _, row := range rows {
			code := strings.ToLower(strings.TrimSpace(row.Code))
			if code == string(coreiam.CodeRoleOwner) {
				ownerMemberIDSet[row.SubjectID] = struct{}{}
				adminMemberIDSet[row.SubjectID] = struct{}{}
				continue
			}
			if code == string(coreiam.CodeRoleAdmin) {
				adminMemberIDSet[row.SubjectID] = struct{}{}
			}
		}
	}

	// 5) 组装 members brief（按 role_binding 实际结果填充 is_admin）
	brs := make([]MeMemberBrief, 0, len(members))
	memberByTenant := make(map[string]uint64, len(members))
	memberUUIDByTenant := make(map[string]string, len(members))
	memberUUIDByID := make(map[uint64]string, len(members))
	for _, mem := range members {
		info := tenantBasicMap[mem.TenantUUID]
		uuidStr := strings.TrimSpace(mem.TenantUUID)
		name := ""
		if info.ID != 0 {
			uuidStr = info.UUID.String()
			name = info.Name
		}
		memberUUIDStr := mem.UUID.String()
		memberByTenant[uuidStr] = mem.ID
		memberUUIDByTenant[uuidStr] = memberUUIDStr
		memberUUIDByID[mem.ID] = memberUUIDStr
		_, isAdmin := adminMemberIDSet[mem.ID]
		_, isOwner := ownerMemberIDSet[mem.ID]
		brs = append(brs, MeMemberBrief{
			TenantUUID:   uuidStr,
			TenantKey:    info.Key,
			TenantName:   name,
			TenantDomain: info.Domain,
			MemberID:     mem.ID,
			MemberUUID:   memberUUIDStr,
			IsAdmin:      isAdmin,
			IsOwner:      isOwner,
		})
	}
	if memberUUID == "" && memberID != 0 {
		memberUUID = memberUUIDByID[memberID]
	}

	// 6) 修正 current_tenant_uuid：
	// db-refresh/本地缓存/token stale 时，ctx 中的 tenant_uuid 可能不在 members 里，导致前端永远查不到数据。
	// 规则：若当前 tenant 不在 members，优先选 "System" 租户，否则选第一个 member 租户。
	if len(brs) > 0 {
		tenantInMembers := false
		for _, b := range brs {
			if b.TenantUUID == tenantUUID {
				tenantInMembers = true
				break
			}
		}
		if isRoot && tenantUUID != "" && !tenantInMembers {
			// root 允许切到“非成员租户”做跨租户管理，但要保证目标租户真实存在。
			if exists, err := s.TenantExists(ctx, tenantUUID); err == nil && exists {
				currentMemberID = nil
				memberUUID = ""
			} else {
				tenantUUID = ""
			}
		}
		if tenantUUID == "" || (!tenantInMembers && !isRoot) {
			preferred := brs[0].TenantUUID
			for _, b := range brs {
				if strings.EqualFold(strings.TrimSpace(b.TenantName), "system") {
					preferred = b.TenantUUID
					break
				}
			}
			tenantUUID = preferred
			if mid, ok := memberByTenant[tenantUUID]; ok {
				currentMemberID = &mid
				memberUUID = memberUUIDByTenant[tenantUUID]
			}
		}
		if memberUUID == "" && tenantUUID != "" {
			memberUUID = memberUUIDByTenant[tenantUUID]
			if mid, ok := memberByTenant[tenantUUID]; ok && currentMemberID == nil {
				currentMemberID = &mid
			}
		}
	}

	return &MeContextResp{
		IsRoot:            isRoot,
		CurrentTenantUUID: tenantUUID,
		CurrentMemberID:   currentMemberID,
		CurrentMemberUUID: memberUUID,
		User:              userBrief,
		Members:           brs,
	}, nil
}

// TenantExists 用于 root 跨租户切换时校验目标租户是否存在。
func (s *MeService) TenantExists(ctx context.Context, tenantUUID string) (bool, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return false, nil
	}
	t, err := s.TenantRepo.GetByUUID(ctx, tenantUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return t != nil, nil
}

func (s *MeService) UpdateMyProfile(ctx context.Context, in UpdateMyProfileInput) (*MeUserBrief, error) {
	userID := reqctx.GetUserID(ctx)
	if userID == 0 {
		return nil, dto.NewUnauthorized("未登录", nil)
	}

	user, err := s.UserRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.NewNotFound("用户不存在", err)
		}
		return nil, dto.NewInternal("查询用户失败", err)
	}

	updates := map[string]any{}

	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" {
			return nil, dto.NewBadRequest("display_name 不能为空", nil)
		}
		updates["display_name"] = name
	}

	if in.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*in.Email))
		if email == "" {
			return nil, dto.NewBadRequest("email 不能为空", nil)
		}
		exist, qerr := s.UserRepo.FindByEmail(ctx, email)
		if qerr != nil && !errors.Is(qerr, gorm.ErrRecordNotFound) {
			return nil, dto.NewInternal("检查邮箱唯一性失败", qerr)
		}
		if exist != nil && exist.ID != userID {
			return nil, dto.NewConflict("邮箱已被占用", nil)
		}
		updates["email"] = email
	}

	if in.Phone != nil {
		phone := strings.TrimSpace(*in.Phone)
		if phone != "" {
			exist, qerr := s.UserRepo.FindByPhone(ctx, phone)
			if qerr != nil && !errors.Is(qerr, gorm.ErrRecordNotFound) {
				return nil, dto.NewInternal("检查手机号唯一性失败", qerr)
			}
			if exist != nil && exist.ID != userID {
				return nil, dto.NewConflict("手机号已被占用", nil)
			}
		}
		updates["phone"] = phone
	}

	if in.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*in.AvatarURL)
	}

	if len(updates) > 0 {
		if err := s.DB.WithContext(ctx).
			Model(&modelIAM.User{}).
			Where("id = ?", userID).
			Updates(updates).Error; err != nil {
			return nil, dto.NewInternal("更新个人资料失败", err)
		}
		user, err = s.UserRepo.FindByID(ctx, userID)
		if err != nil {
			return nil, dto.NewInternal("重新加载用户失败", err)
		}
	}

	return &MeUserBrief{
		ID:          user.ID,
		Email:       user.Email,
		Phone:       user.Phone,
		DisplayName: user.DisplayName,
		AvatarURL:   user.AvatarURL,
		Status:      user.Status,
		LastLoginAt: user.LastLoginAt,
		IsRoot:      user.IsRoot,
	}, nil
}

func (s *MeService) ChangeMyPassword(ctx context.Context, currentPassword, newPassword string) error {
	userID := reqctx.GetUserID(ctx)
	if userID == 0 {
		return dto.NewUnauthorized("未登录", nil)
	}

	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	if currentPassword == "" || newPassword == "" {
		return dto.NewBadRequest("密码不能为空", nil)
	}

	var creds []modelIAM.Credential
	if err := s.DB.WithContext(ctx).
		Model(&modelIAM.Credential{}).
		Where("user_id = ? AND provider = ?", userID, "password").
		Find(&creds).Error; err != nil {
		return dto.NewInternal("查询凭据失败", err)
	}
	if len(creds) == 0 {
		return dto.NewBadRequest("当前账号未配置密码登录", nil)
	}

	matched := false
	for i := range creds {
		if bcrypt.CompareHashAndPassword([]byte(creds[i].SecretHash), []byte(currentPassword)) == nil {
			matched = true
			break
		}
	}
	if !matched {
		return dto.NewBadRequest("当前密码不正确", nil)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return dto.NewInternal("生成密码哈希失败", err)
	}

	if err := s.DB.WithContext(ctx).
		Model(&modelIAM.Credential{}).
		Where("user_id = ? AND provider = ?", userID, "password").
		Update("secret_hash", string(hash)).Error; err != nil {
		return dto.NewInternal("更新密码失败", err)
	}
	return nil
}
