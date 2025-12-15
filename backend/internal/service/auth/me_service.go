package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// ======= 对外返回结构（供 Handler 使用） =======
type MeMemberBrief struct {
	TenantUUID string `json:"tenant_uuid"`
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
	IsRoot            bool            `json:"is_root"`
	CurrentTenantUUID string          `json:"current_tenant_uuid"`
	CurrentMemberID   *uint64         `json:"current_member_id,omitempty"`
	User              *MeUserBrief    `json:"user,omitempty"`
	Members           []MeMemberBrief `json:"members"`
	// PowerX 上下文签名（由网关生成），用于插件侧转发
	Ctx    string `json:"ctx,omitempty"`
	CtxSig string `json:"ctx_sig,omitempty"`
	CtxJwt string `json:"ctx_jwt,omitempty"`
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

	// 4) 组装 members brief（is_admin 先 false，等你接 RBAC 再填充）
	brs := make([]MeMemberBrief, 0, len(members))
	for _, mem := range members {
		info := tenantBasicMap[mem.TenantUUID]
		uuidStr := strings.TrimSpace(mem.TenantUUID)
		name := ""
		if info.ID != 0 {
			uuidStr = info.UUID.String()
			name = info.Name
		}
		brs = append(brs, MeMemberBrief{
			TenantUUID: uuidStr,
			TenantName: name,
			MemberID:   mem.ID,
			IsAdmin:    false, // TODO: 用 RoleBinding 判断是否为该租户管理员
		})
	}

	return &MeContextResp{
		IsRoot:            isRoot,
		CurrentTenantUUID: tenantUUID,
		CurrentMemberID:   currentMemberID,
		User:              userBrief,
		Members:           brs,
	}, nil
}
