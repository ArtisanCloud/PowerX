package auth

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/service"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
	"net/http"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

// ======= 对外返回结构（供 Handler 使用） =======
type MeMemberBrief struct {
	TenantID   uint64 `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	MemberID   uint64 `json:"member_id"`
	IsAdmin    bool   `json:"is_admin"`
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
	IsRoot          bool            `json:"is_root"`
	CurrentTenantID uint64          `json:"current_tenant_id"`
	CurrentMemberID *uint64         `json:"current_member_id,omitempty"`
	User            *MeUserBrief    `json:"user,omitempty"`
	Members         []MeMemberBrief `json:"members"`
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
	tenantID := reqctx.GetTenantID(ctx)
	memberID := reqctx.GetMemberID(ctx)

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
	tenantIDs := make([]uint64, 0, len(members))
	for _, mem := range members {
		tenantIDs = append(tenantIDs, mem.TenantID)
	}
	tenantNameMap, _ := s.TenantRepo.MapNamesByIDs(ctx, tenantIDs)

	// 4) 组装 members brief（is_admin 先 false，等你接 RBAC 再填充）
	brs := make([]MeMemberBrief, 0, len(members))
	for _, mem := range members {
		brs = append(brs, MeMemberBrief{
			TenantID:   mem.TenantID,
			TenantName: tenantNameMap[mem.TenantID],
			MemberID:   mem.ID,
			IsAdmin:    false, // TODO: 用 RoleBinding 判断是否为该租户管理员
		})
	}

	return &MeContextResp{
		IsRoot:          isRoot,
		CurrentTenantID: tenantID,
		CurrentMemberID: currentMemberID,
		User:            userBrief,
		Members:         brs,
	}, nil
}
