// internal/service/tenant/tenant_service.go
package tenant

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	mdltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"gorm.io/gorm/clause"
	"strings"

	"gorm.io/gorm"
)

type TenantService struct {
	*service.BaseService
	Repo           *tenantRepo.TenantRepository
	Auth           *authsvc.AuthService // 复用 Register
	MemberRoleRepo *iamrepo.MemberRoleRepository
}

func NewTenantService(db *gorm.DB, auth *authsvc.AuthService, mr *iamrepo.MemberRoleRepository) *TenantService {
	return &TenantService{
		BaseService: &service.BaseService{
			DB: db,
		},
		Repo:           tenantRepo.NewTenantRepository(db),
		Auth:           auth,
		MemberRoleRepo: mr,
	}
}

type ListTenantsOption struct {
	Page, PageSize int
	SortBy         string
	SortOrder      string
	Keyword        string
	Status         *string
	Plan           *string
}

func (s *TenantService) List(ctx context.Context, opt ListTenantsOption) (items []mdltenant.Tenant, total int64, userCount map[uint64]int64, err error) {
	// 排序白名单，防注入
	sortBy := sanitizeSortBy(opt.SortBy, []string{"id", "name", "domain", "created_at", "updated_at"})
	sortOrder := strings.ToLower(opt.SortOrder)
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// 主列表与总数
	items, total, err = s.Repo.FindTenants(ctx, tenantRepo.FindTenantsCond{
		Page:      opt.Page,
		PageSize:  opt.PageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Keyword:   opt.Keyword,
		Status:    opt.Status,
		Plan:      opt.Plan,
	})
	if err != nil {
		return nil, 0, nil, err
	}

	// 附加 user_count（一次聚合）
	userCount, err = s.Repo.CountMembersByTenant(ctx)
	if err != nil {
		// 非致命错误：置空 map
		userCount = map[uint64]int64{}
	}
	return
}

func (s *TenantService) Get(ctx context.Context, id uint64) (*mdltenant.Tenant, error) {
	return s.Repo.GetByID(ctx, id)
}

func sanitizeSortBy(in string, allow []string) string {
	if in == "" {
		return "created_at"
	}
	low := strings.ToLower(in)
	for _, a := range allow {
		if low == a {
			return low
		}
	}
	return "created_at"
}

type InitAdminInput struct {
	UserName, Password     *string
	Email, Phone           *string
	DisplayName, AvatarURL *string
	AssignOwner            *bool // 默认 true
}

type CreateTenantInput struct {
	Key, Name, Plan     string
	Domain, Description *string
	Status              *int16
	InitAdmin           *InitAdminInput // 可选
}

type UpsertTenantInput struct {
	Key, Name, Plan     string
	Domain, Description *string
	Status              *int16
	InitAdmin           *InitAdminInput // 仅在“首次创建”时生效
}

// ---------- Create：严格创建（存在即报错） ----------
func (s *TenantService) Create(ctx context.Context, in CreateTenantInput) (uint64, error) {
	key := strings.TrimSpace(in.Key)
	name := strings.TrimSpace(in.Name)
	plan := strings.TrimSpace(in.Plan)
	if key == "" || name == "" || plan == "" {
		return 0, fmt.Errorf("key/name/plan required")
	}
	if key == mdltenant.SystemTenantKey {
		return 0, fmt.Errorf("system tenant is protected")
	}

	// 1) 严格创建（存在即错误）
	t := &mdltenant.Tenant{Key: key, Name: name, Plan: plan}
	if in.Domain != nil {
		t.Domain = *in.Domain
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Status != nil {
		t.Status = *in.Status
	} else {
		t.Status = 1
	}

	// BaseRepository.Create：存在唯一冲突会报错
	out, err := s.Repo.Create(ctx, t)
	if err != nil {
		return 0, err
	}

	// 2) 可选：初始化管理员
	if in.InitAdmin != nil {
		if err := s.initAdmin(ctx, out.ID, in.InitAdmin); err != nil {
			return out.ID, err // 视需求：出错可回滚或保留租户
		}
	}
	return out.ID, nil
}

// ---------- Upsert：幂等（若存在则更新白名单列；首次创建可初始化管理员） ----------
func (s *TenantService) Upsert(ctx context.Context, in UpsertTenantInput) (uint64, error) {
	key := strings.TrimSpace(in.Key)
	name := strings.TrimSpace(in.Name)
	plan := strings.TrimSpace(in.Plan)
	if key == "" || name == "" || plan == "" {
		return 0, fmt.Errorf("key/name/plan required")
	}

	// System tenant 保护：若已存在则直接返回，避免覆盖
	if key == mdltenant.SystemTenantKey {
		if existed, err := s.Repo.GetByKey(ctx, key); err == nil && existed != nil {
			return existed.ID, nil
		}
	}

	// 先判断是否已存在（用来决定是否需要初始化管理员）
	existed, err := s.Repo.GetByKey(ctx, key)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	isCreate := (existed == nil)

	t := &mdltenant.Tenant{Key: key, Name: name, Plan: plan}
	if in.Domain != nil {
		t.Domain = *in.Domain
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Status != nil {
		t.Status = *in.Status
	} else {
		t.Status = 1
	}

	// BaseRepository.Upsert：需要指定冲突列（key）
	out, err := s.Repo.Upsert(ctx, t, []clause.Column{{Name: "key"}})
	if err != nil {
		return 0, err
	}

	if isCreate && in.InitAdmin != nil {
		if err := s.initAdmin(ctx, out.ID, in.InitAdmin); err != nil {
			return out.ID, err
		}
	}
	return out.ID, nil
}

// ---------- 初始化管理员（复用 AuthService.Register） ----------
func (s *TenantService) initAdmin(ctx context.Context, tenantID uint64, ia *InitAdminInput) error {
	if ia == nil {
		return nil
	}

	// 至少要有用户名 + 密码，或 email/phone 作为 identifier；否则不能注册
	if ia.Password == nil || *ia.Password == "" {
		return fmt.Errorf("admin_password required")
	}
	// 归一化/兜底
	var username string
	if ia.UserName != nil {
		username = strings.TrimSpace(*ia.UserName)
	}
	if username == "" {
		// 若未给用户名，用邮箱/手机号/“admin”之一兜底
		if ia.Email != nil && *ia.Email != "" {
			username = strings.Split(strings.TrimSpace(*ia.Email), "@")[0]
		} else if ia.Phone != nil && *ia.Phone != "" {
			username = *ia.Phone
		} else {
			username = "admin"
		}
	}
	// identifier：优先 email -> phone -> username
	var identifier string
	if ia.Email != nil && *ia.Email != "" {
		identifier = strings.ToLower(strings.TrimSpace(*ia.Email))
	} else if ia.Phone != nil && *ia.Phone != "" {
		identifier = strings.TrimSpace(*ia.Phone)
	} else {
		identifier = username
	}

	// 组织 RegisterOptions
	opt := &authsvc.RegisterOptions{}
	if ia.DisplayName != nil {
		opt.MemberDisplayName = *ia.DisplayName
	}
	if ia.AvatarURL != nil {
		opt.MemberAvatarURL = *ia.AvatarURL
	}
	if ia.Email != nil {
		opt.UserEmail = strings.ToLower(strings.TrimSpace(*ia.Email))
	}
	if ia.Phone != nil {
		opt.UserPhone = strings.TrimSpace(*ia.Phone)
	}

	// 注册：创建/复用 User + 创建 Member + 绑定 role_user
	m, err := s.Auth.Register(ctx, tenantID, username, identifier, *ia.Password, opt)
	if err != nil {
		return err
	}

	// 额外赋 role_owner（可选；默认 true）
	assignOwner := true
	if ia.AssignOwner != nil {
		assignOwner = *ia.AssignOwner
	}
	if assignOwner && s.MemberRoleRepo != nil {
		_ = s.MemberRoleRepo.AssignRolesByCodes(ctx, tenantID, m.ID, "role_owner")
	}
	return nil
}

type UpdateTenantInput struct {
	Name        *string
	Domain      *string
	Status      *int16
	Plan        *string
	Description *string
}

func (s *TenantService) Update(ctx context.Context, id uint64, in UpdateTenantInput) error {
	// 读取并保护 System 租户
	t, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if t.Key == mdltenant.SystemTenantKey {
		return fmt.Errorf("system tenant is protected")
	}

	// 组装允许更新的字段（白名单）
	fields := map[string]any{}
	if in.Name != nil {
		fields["name"] = *in.Name
	}
	if in.Domain != nil {
		fields["domain"] = *in.Domain
	}
	if in.Status != nil {
		fields["status"] = *in.Status
	}
	if in.Plan != nil {
		fields["plan"] = *in.Plan
	}
	if in.Description != nil {
		fields["description"] = *in.Description
	}
	if len(fields) == 0 {
		return nil // 无变更视为成功
	}

	// 可按需补充唯一性校验（domain 唯一等）
	_, err = s.Repo.Patch(ctx, map[string]any{"id": id}, fields)
	return err
}

func (s *TenantService) Delete(ctx context.Context, id uint64) error {
	// 1) 读取并保护 System
	t, err := s.Repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("tenant not found")
	}
	if t.Key == mdltenant.SystemTenantKey {
		return fmt.Errorf("system tenant is protected")
	}

	// 2) 业务保护：可选
	// cntMap, _ := s.Repo.CountMembersByTenant(ctx)
	// if cntMap[id] > 0 { return fmt.Errorf("tenant has members, cannot delete") }

	// 3) 软删（where 分支 + RowsAffected 校验）
	_, err = s.Repo.Delete(ctx, map[string]any{"id": id}, nil, true)
	return err
}
