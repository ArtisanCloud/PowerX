package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"gorm.io/gorm"
	"strings"
	"time"

	repoIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

const ROOT_USERNAME = "root"

type MemberService struct {
	db               *gorm.DB
	MemberRepo       *repoIAM.MemberRepository
	UserRepo         *repoIAM.UserRepository
	MemberDeptRepo   *repoIAM.MemberDepartmentRepository
	DeptClosureRepo  *repoIAM.DepartmentClosureRepository
	RefreshTokenRepo *repoIAM.RefreshTokenRepository
}

func NewMemberService(db *gorm.DB) *MemberService {
	return &MemberService{
		db:               db,
		MemberRepo:       repoIAM.NewMemberRepository(db),
		UserRepo:         repoIAM.NewUserRepository(db),
		MemberDeptRepo:   repoIAM.NewMemberDepartmentRepository(db),
		DeptClosureRepo:  repoIAM.NewDepartmentClosureRepository(db),
		RefreshTokenRepo: repoIAM.NewRefreshTokenRepository(db),
	}
}

// -------- 输入输出（仅 service 内部使用） --------
type ListMembersOption struct {
	Page, PageSize int
	SortBy         string // "id" | "created_at" | "updated_at"
	SortOrder      string // "asc" | "desc"
	TenantID       uint64
	Keyword        string
	Status         *int16
	DeptID         *uint64
	Recursive      bool
}
type MemberWithProfile struct {
	Member  modelIAM.Member
	User    *modelIAM.User
	DeptIDs []uint64
}
type CreateMemberInput struct {
	Member          modelIAM.Member
	User            modelIAM.User // 用于 ensure/create user（email/phone/头像/展示名）
	DeptIDs         []uint64      // 初始部门
	InitialPassword string        // 如需写 credential，可在此接入
}
type UpdateMemberInput struct {
	Member  *modelIAM.Member
	User    *modelIAM.User
	DeptIDs *[]uint64 // nil 不改；空数组清空
}

// 列表（按租户/关键词/状态/部门递归筛选），不写 SQL，借助 BaseRepository.FindByCondition 的 callback
func (s *MemberService) ListMembers(ctx context.Context, opt ListMembersOption) (items []MemberWithProfile, total int64, err error) {
	cond := map[string]interface{}{
		model.TableIAMMember + ".tenant_id = ?": opt.TenantID,
		// 关键：过滤掉 root
		model.TableIAMMember + ".username <> ?": ROOT_USERNAME,
	}

	// 用 BaseRepository 的分页能力；排序也交给它
	page, err := s.MemberRepo.FindByCondition(ctx, cond, opt.Page, opt.PageSize,
		func(db *gorm.DB, _ interface{}) *gorm.DB {
			q := db.Table(model.TableIAMMember)

			// 关键词（命中 member.username/email/phone/display_name）
			if kw := strings.TrimSpace(opt.Keyword); kw != "" {
				like := "%" + strings.ToLower(kw) + "%"
				q = q.Where("("+
					"LOWER("+model.TableIAMMember+".username) LIKE ? OR "+
					"LOWER("+model.TableIAMMember+".email) LIKE ? OR "+
					"LOWER("+model.TableIAMMember+".phone) LIKE ? OR "+
					"LOWER("+model.TableIAMMember+".display_name) LIKE ?"+
					")", like, like, like, like)
			}

			// 状态
			if opt.Status != nil {
				q = q.Where(model.TableIAMMember+".status = ?", *opt.Status)
			}

			// 部门过滤（可递归）
			if opt.DeptID != nil {
				if opt.Recursive {
					q = q.Joins("JOIN "+model.TableIAMMemberDepartment+" md ON md.member_id = "+model.TableIAMMember+".id AND md.tenant_id = "+model.TableIAMMember+".tenant_id").
						Joins("JOIN "+model.TableIAMDepartmentClosure+" dc ON dc.tenant_id = "+model.TableIAMMember+".tenant_id AND dc.descendant_id = md.department_id").
						Where("dc.ancestor_id = ?", *opt.DeptID)
				} else {
					q = q.Joins("JOIN "+model.TableIAMMemberDepartment+" md ON md.member_id = "+model.TableIAMMember+".id AND md.tenant_id = "+model.TableIAMMember+".tenant_id").
						Where("md.department_id = ?", *opt.DeptID)
				}
			}

			// 排序（白名单）
			sb, so := model.TableIAMMember+".id", "DESC"
			switch strings.ToLower(strings.TrimSpace(opt.SortBy)) {
			case "created_at":
				sb = model.TableIAMMember + ".created_at"
			case "updated_at":
				sb = model.TableIAMMember + ".updated_at"
			case "id", "":
				sb = model.TableIAMMember + ".id"
			}
			if strings.ToLower(opt.SortOrder) == "asc" {
				so = "ASC"
			}
			return q.Order(sb + " " + so)
		}, nil)
	if err != nil {
		return nil, 0, err
	}

	// 组装返回（补 User/DeptIDs，保持你原来的逻辑）
	items = make([]MemberWithProfile, 0, len(page.List))
	memberIDs := make([]uint64, 0, len(page.List))
	userIDs := make([]uint64, 0, len(page.List))
	for _, mem := range page.List {
		memberIDs = append(memberIDs, mem.ID)
		userIDs = append(userIDs, mem.UserID)
	}

	userByID := map[uint64]*modelIAM.User{}
	for _, uid := range userIDs {
		if u, e := s.UserRepo.FindByID(ctx, uid); e == nil && u != nil {
			userByID[uid] = u
		}
	}

	deptIDsMap := map[uint64][]uint64{}
	if len(memberIDs) > 0 {
		type pair struct{ MemberID, DepartmentID uint64 }
		var ps []pair
		if err2 := s.db.WithContext(ctx).Table(model.TableIAMMemberDepartment).
			Select("member_id, department_id").
			Where("tenant_id = ? AND member_id IN ?", opt.TenantID, memberIDs).
			Scan(&ps).Error; err2 == nil {
			for _, p := range ps {
				deptIDsMap[p.MemberID] = append(deptIDsMap[p.MemberID], p.DepartmentID)
			}
		}
	}

	for _, mem := range page.List {
		items = append(items, MemberWithProfile{
			Member:  *mem,
			User:    userByID[mem.UserID],
			DeptIDs: deptIDsMap[mem.ID],
		})
	}
	return items, page.Total, nil
}

func (s *MemberService) GetMember(ctx context.Context, tenantID, memberID uint64) (*MemberWithProfile, error) {
	mem, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
		model.TableIAMMember + ".tenant_id = ?": tenantID,
		model.TableIAMMember + ".id = ?":        memberID,
	}, nil)
	if err != nil {
		return nil, err
	}
	if mem == nil {
		return nil, gorm.ErrRecordNotFound
	}
	u, err := s.UserRepo.FindByID(ctx, mem.UserID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var deptIDs []uint64
	if err := s.db.WithContext(ctx).Table(model.TableIAMMemberDepartment).
		Where("tenant_id = ? AND member_id = ?", tenantID, mem.ID).
		Pluck("department_id", &deptIDs).Error; err != nil {
		return nil, err
	}
	return &MemberWithProfile{Member: *mem, User: u, DeptIDs: deptIDs}, nil
}

func (s *MemberService) CreateMember(ctx context.Context, tenantID uint64, in CreateMemberInput) (uint64, error) {
	var newID uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ensure/create user（email / phone）
		var u *modelIAM.User
		var err error
		if e := strings.TrimSpace(in.User.Email); e != "" {
			u, err = s.UserRepo.FindByEmail(ctx, strings.ToLower(e))
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if u == nil {
			if p := strings.TrimSpace(in.User.Phone); p != "" {
				u, err = s.UserRepo.FindByPhone(ctx, p)
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
		}
		if u == nil {
			u = &modelIAM.User{
				Email:       strings.ToLower(strings.TrimSpace(in.User.Email)),
				Phone:       strings.TrimSpace(in.User.Phone),
				DisplayName: strings.TrimSpace(in.Member.DisplayName),
				AvatarURL:   strings.TrimSpace(in.Member.AvatarURL),
				Status:      1,
				Meta:        in.User.Meta,
			}
			if in.Member.Status != 0 {
				u.Status = in.Member.Status
			}
			if _, err = s.UserRepo.Create(ctx, u); err != nil {
				return err
			}
			// TODO: 写入 Credential（如需要）
		}

		mem := in.Member
		mem.ID = 0
		mem.TenantID = tenantID
		mem.UserID = u.ID
		if mem.Status == 0 {
			mem.Status = u.Status
		}
		mem.Username = strings.ToLower(strings.TrimSpace(mem.Username))

		if _, err := s.MemberRepo.Create(ctx, &mem); err != nil {
			return err
		}
		newID = mem.ID

		// 部门绑定（覆盖式初始）
		if len(in.DeptIDs) > 0 {
			// 用 BaseRepository.Delete + CreateBatch（在 repo 上下文）
			if _, err := s.MemberDeptRepo.Delete(ctx, map[string]interface{}{
				"tenant_id = ?": tenantID,
				"member_id = ?": mem.ID,
			}, nil, true); err != nil {
				return err
			}
			rows := make([]*modelIAM.MemberDepartment, 0, len(in.DeptIDs))
			for _, did := range in.DeptIDs {
				rows = append(rows, &modelIAM.MemberDepartment{
					TenantID:     tenantID,
					MemberID:     mem.ID,
					DepartmentID: did,
				})
			}
			if _, err := s.MemberDeptRepo.CreateBatch(ctx, rows); err != nil {
				return err
			}
		}
		return nil
	})
	return newID, err
}

func (s *MemberService) UpdateMember(ctx context.Context, tenantID, memberID uint64, in UpdateMemberInput) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mem, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
			model.TableIAMMember + ".tenant_id = ?": tenantID,
			model.TableIAMMember + ".id = ?":        memberID,
		}, nil)
		if err != nil {
			return err
		}
		if mem == nil {
			return gorm.ErrRecordNotFound
		}

		if in.Member != nil {
			fields := map[string]interface{}{}
			if v := strings.TrimSpace(in.Member.Username); v != "" {
				fields["username"] = strings.ToLower(v)
			}
			if v := strings.TrimSpace(in.Member.DisplayName); v != "" {
				fields["display_name"] = v
			}
			if v := strings.TrimSpace(in.Member.AvatarURL); v != "" {
				fields["avatar_url"] = v
			}
			if in.Member.Status != 0 {
				fields["status"] = in.Member.Status
			}
			if in.Member.Meta != nil {
				fields["meta"] = in.Member.Meta
			}
			if len(fields) > 0 {
				if _, err := s.MemberRepo.Patch(ctx,
					map[string]interface{}{
						model.TableIAMMember + ".tenant_id = ?": tenantID,
						model.TableIAMMember + ".id = ?":        memberID,
					},
					fields,
				); err != nil {
					return err
				}
			}
		}

		if in.User != nil {
			uFields := map[string]interface{}{}
			if v := strings.TrimSpace(in.User.Email); v != "" {
				uFields["email"] = strings.ToLower(v)
			}
			if v := strings.TrimSpace(in.User.Phone); v != "" {
				uFields["phone"] = v
			}
			if v := strings.TrimSpace(in.User.DisplayName); v != "" {
				uFields["display_name"] = v
			}
			if v := strings.TrimSpace(in.User.AvatarURL); v != "" {
				uFields["avatar_url"] = v
			}
			if in.User.Status != 0 {
				uFields["status"] = in.User.Status
			}
			if in.User.Meta != nil {
				uFields["meta"] = in.User.Meta
			}
			if len(uFields) > 0 {
				if _, err := s.UserRepo.Patch(ctx,
					map[string]interface{}{"id = ?": mem.UserID},
					uFields,
				); err != nil {
					return err
				}
			}
		}

		if in.DeptIDs != nil {
			if _, err := s.MemberDeptRepo.Delete(ctx, map[string]interface{}{
				"tenant_id = ?": tenantID,
				"member_id = ?": memberID,
			}, nil, true); err != nil {
				return err
			}
			if len(*in.DeptIDs) > 0 {
				rows := make([]*modelIAM.MemberDepartment, 0, len(*in.DeptIDs))
				for _, did := range *in.DeptIDs {
					rows = append(rows, &modelIAM.MemberDepartment{
						TenantID:     tenantID,
						MemberID:     memberID,
						DepartmentID: did,
					})
				}
				if _, err := s.MemberDeptRepo.CreateBatch(ctx, rows); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *MemberService) SetMemberStatus(ctx context.Context, tenantID, memberID uint64, status int16, _ string) error {
	_, err := s.MemberRepo.Patch(ctx,
		map[string]interface{}{
			model.TableIAMMember + ".tenant_id = ?": tenantID,
			model.TableIAMMember + ".id = ?":        memberID,
		},
		map[string]interface{}{"status": status},
	)
	return err
}

func (s *MemberService) DeleteMember(ctx context.Context, tenantID, memberID uint64) error {
	_, err := s.MemberRepo.Delete(ctx, map[string]interface{}{
		model.TableIAMMember + ".tenant_id = ?": tenantID,
		model.TableIAMMember + ".id = ?":        memberID,
	}, nil, true)
	return err
}

func (s *MemberService) RestoreMember(ctx context.Context, tenantID, memberID uint64) error {
	// 用 Unscoped 更新 deleted_at
	return s.db.WithContext(ctx).Unscoped().
		Table(model.TableIAMMember).
		Where("tenant_id = ? AND id = ?", tenantID, memberID).
		Update("deleted_at", nil).Error
}

func (s *MemberService) PutMemberDepartments(ctx context.Context, tenantID, memberID uint64, deptIDs []uint64) error {
	// 覆盖式写入，用 BaseRepository
	if _, err := s.MemberDeptRepo.Delete(ctx, map[string]interface{}{
		"tenant_id = ?": tenantID,
		"member_id = ?": memberID,
	}, nil, true); err != nil {
		return err
	}
	if len(deptIDs) == 0 {
		return nil
	}
	rows := make([]*modelIAM.MemberDepartment, 0, len(deptIDs))
	for _, did := range deptIDs {
		rows = append(rows, &modelIAM.MemberDepartment{
			TenantID:     tenantID,
			MemberID:     memberID,
			DepartmentID: did,
		})
	}
	_, err := s.MemberDeptRepo.CreateBatch(ctx, rows)
	return err
}

func (s *MemberService) ForceLogout(ctx context.Context, tenantID, memberID uint64, jti string) error {
	mem, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
		model.TableIAMMember + ".tenant_id = ?": tenantID,
		model.TableIAMMember + ".id = ?":        memberID,
	}, nil)
	if err != nil {
		return err
	}
	if mem == nil {
		return gorm.ErrRecordNotFound
	}
	now := time.Now()
	if strings.TrimSpace(jti) != "" {
		return s.RefreshTokenRepo.RevokeByJTI(ctx, jti, now.UnixMilli())
	}
	return s.RefreshTokenRepo.RevokeAllForUser(ctx, mem.UserID, tenantID, now)
}
