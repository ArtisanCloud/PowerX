package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenantmdl "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"strings"
	"time"

	repoIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

const ROOT_USERNAME = "root"

type MemberService struct {
	*service.BaseService
	MemberRepo       *repoIAM.MemberRepository
	UserRepo         *repoIAM.UserRepository
	MemberDeptRepo   *repoIAM.MemberDepartmentRepository
	DeptClosureRepo  *repoIAM.DepartmentClosureRepository
	RefreshTokenRepo *repoIAM.RefreshTokenRepository
	CredRepo         *repoIAM.CredentialRepository
}

func NewMemberService(db *gorm.DB) *MemberService {
	return &MemberService{
		BaseService: &service.BaseService{
			DB: db,
		},
		MemberRepo:       repoIAM.NewMemberRepository(db),
		UserRepo:         repoIAM.NewUserRepository(db),
		MemberDeptRepo:   repoIAM.NewMemberDepartmentRepository(db),
		DeptClosureRepo:  repoIAM.NewDepartmentClosureRepository(db),
		RefreshTokenRepo: repoIAM.NewRefreshTokenRepository(db),
		CredRepo:         repoIAM.NewCredentialRepository(db),
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
	cond := map[string]interface{}{} // 这里不再用 cond 传筛选，全部放在回调里

	page, err := s.MemberRepo.FindByCondition(ctx, cond, opt.Page, opt.PageSize,
		func(db *gorm.DB, _ interface{}) *gorm.DB {
			q := db.Table(model.TableIAMMember+" AS m").
				Joins("LEFT JOIN "+model.TableIAMUser+" AS u ON u.id = m.user_id").
				// 明确租户、过滤 root、软删
				Where("m.tenant_id = ?", opt.TenantID).
				Where("m.username <> ?", ROOT_USERNAME).
				Where("m.deleted_at IS NULL")

			// 关键词：m.username / m.display_name / u.email / u.phone
			if kw := strings.TrimSpace(opt.Keyword); kw != "" {
				kw = strings.ToLower(kw)
				like := "%" + kw + "%"
				q = q.Where("("+
					"LOWER(m.username) LIKE ? OR "+
					"LOWER(m.display_name) LIKE ? OR "+
					"LOWER(u.email) LIKE ? OR "+
					"LOWER(u.phone) LIKE ?"+
					")", like, like, like, like)
			}

			// 状态
			if opt.Status != nil {
				q = q.Where("m.status = ?", *opt.Status)
			}

			// 部门过滤（可递归）
			if opt.DeptID != nil {
				if opt.Recursive {
					q = q.
						Joins("JOIN "+model.TableIAMMemberDepartment+" AS md ON md.member_id = m.id AND md.tenant_id = m.tenant_id").
						Joins("JOIN "+model.TableIAMDepartmentClosure+" AS dc ON dc.tenant_id = m.tenant_id AND dc.descendant_id = md.department_id").
						Where("dc.ancestor_id = ?", *opt.DeptID)
				} else {
					q = q.
						Joins("JOIN "+model.TableIAMMemberDepartment+" AS md ON md.member_id = m.id AND md.tenant_id = m.tenant_id").
						Where("md.department_id = ?", *opt.DeptID)
				}
			}

			// ✅ 关键：DISTINCT + 只按 m.id 排序，避免 PG 报错 42P10
			so := "DESC"
			if strings.ToLower(opt.SortOrder) == "asc" {
				so = "ASC"
			}
			return q.Distinct("m.id").Order("m.id " + so)
		}, nil)
	if err != nil {
		return nil, 0, err
	}

	// 组装返回：一次性批量取 user，避免 N+1
	items = make([]MemberWithProfile, 0, len(page.List))
	memberIDs := make([]uint64, 0, len(page.List))
	userIDs := make([]uint64, 0, len(page.List))
	seenU := make(map[uint64]struct{}, len(page.List))
	for _, mem := range page.List {
		memberIDs = append(memberIDs, mem.ID)
		if _, ok := seenU[mem.UserID]; !ok {
			seenU[mem.UserID] = struct{}{}
			userIDs = append(userIDs, mem.UserID)
		}
	}

	userByID := make(map[uint64]*modelIAM.User, len(userIDs))
	if len(userIDs) > 0 {
		var users []modelIAM.User
		if err := s.DB.WithContext(ctx).
			Table(model.TableIAMUser).
			Where("id IN ?", userIDs).
			Where("deleted_at IS NULL").
			Find(&users).Error; err != nil {
			return nil, 0, err
		}
		for i := range users {
			u := users[i] // 注意取地址
			userByID[u.ID] = &u
		}
	}

	// 批量取成员所属部门
	deptIDsMap := map[uint64][]uint64{}
	if len(memberIDs) > 0 {
		type row struct{ MemberID, DepartmentID uint64 }
		var rows []row
		if err := s.DB.WithContext(ctx).Table(model.TableIAMMemberDepartment).
			Select("member_id, department_id").
			Where("tenant_id = ? AND member_id IN ?", opt.TenantID, memberIDs).
			Scan(&rows).Error; err != nil {
			return nil, 0, err
		}
		for _, r := range rows {
			deptIDsMap[r.MemberID] = append(deptIDsMap[r.MemberID], r.DepartmentID)
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
	if err := s.DB.WithContext(ctx).Table(model.TableIAMMemberDepartment).
		Where("tenant_id = ? AND member_id = ?", tenantID, mem.ID).
		Pluck("department_id", &deptIDs).Error; err != nil {
		return nil, err
	}
	return &MemberWithProfile{Member: *mem, User: u, DeptIDs: deptIDs}, nil
}

func (s *MemberService) CreateMember(ctx context.Context, tenantID uint64, in CreateMemberInput) (uint64, error) {
	var newID uint64
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) ensure/create user
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
			if _, err = s.UserRepo.WithDB(tx).Create(ctx, u); err != nil {
				if repository.IsUniqueViolation(err) {
					return errors.New("user already exists")
				}
				return err
			}
		}

		// 2) optional：写入密码 Credential（仅当提供了 InitialPassword 且不存在密码凭证时）
		if pw := strings.TrimSpace(in.InitialPassword); pw != "" {
			// 选 identifier：email > phone > username
			identifier := strings.ToLower(strings.TrimSpace(u.Email))
			if identifier == "" {
				identifier = strings.TrimSpace(u.Phone)
			}
			if identifier == "" {
				identifier = strings.ToLower(strings.TrimSpace(in.Member.Username))
			}
			if identifier == "" {
				return errors.New("initial_password provided but no identifier to bind (email/phone/username all empty)")
			}

			// 已有则跳过；未有则创建
			cred, err := s.CredRepo.FindByProviderIdentifier(ctx, "password", identifier)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if cred == nil {
				hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
				if err != nil {
					return err
				}
				if _, err = s.CredRepo.WithDB(tx).Create(ctx, &modelIAM.Credential{
					UserID:     u.ID,
					Provider:   "password",
					Identifier: identifier,
					SecretHash: string(hash),
					IsPrimary:  true,
				}); err != nil {
					if repository.IsUniqueViolation(err) {
						return errors.New("credential already exists")
					}
					return err
				}
			}
		}

		// 3) create member（先兜底 username）
		mem := in.Member
		mem.ID = 0
		mem.TenantID = tenantID
		mem.UserID = u.ID
		if mem.Status == 0 {
			mem.Status = u.Status
		}
		mem.Username = strings.ToLower(strings.TrimSpace(mem.Username))
		if mem.Username == "" {
			return errors.New("member.username required")
		}

		// 可选：显式检查 username 租户内唯一，错误更友好
		if dup, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
			model.TableIAMMember + ".tenant_id = ?": tenantID,
			model.TableIAMMember + ".username = ?":  mem.Username,
		}, nil); err != nil {
			return err
		} else if dup != nil {
			return errors.New("username already taken in this tenant")
		}

		if _, err = s.MemberRepo.Create(ctx, &mem); err != nil {
			if repository.IsUniqueViolation(err) {
				return errors.New("member already exists")
			}
			return err
		}
		newID = mem.ID

		// 4) 初始部门（覆盖式）
		if len(in.DeptIDs) > 0 {
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
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
				if _, err := s.MemberRepo.WithDB(tx).Patch(ctx,
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
				if _, err := s.UserRepo.WithDB(tx).Patch(ctx,
					map[string]interface{}{"id = ?": mem.UserID},
					uFields,
				); err != nil {
					return err
				}
			}
		}

		if in.DeptIDs != nil {
			if _, err := s.MemberDeptRepo.WithDB(tx).Delete(ctx, map[string]interface{}{
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
				if _, err = s.MemberDeptRepo.WithDB(tx).CreateBatch(ctx, rows); err != nil {
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
			model.TableIAMMember + ".tenant_id": tenantID,
			model.TableIAMMember + ".id":        memberID,
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
	return s.DB.WithContext(ctx).Unscoped().
		Table(model.TableIAMMember).
		Where("tenant_id = ? AND id = ?", tenantID, memberID).
		Update("deleted_at", nil).Error
}

func (s *MemberService) PutMemberDepartments(ctx context.Context, tenantID, memberID uint64, deptIDs []uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 校验 member 是否存在于本租户
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

		// 2) 若传了部门，校验这些部门都在本租户里
		if len(deptIDs) > 0 {
			var existCnt int64
			if err = tx.Table(model.TableIAMDepartment).
				Where("tenant_id = ? AND id IN ?", tenantID, deptIDs).
				Count(&existCnt).Error; err != nil {
				return err
			}
			if existCnt != int64(len(deptIDs)) {
				return errors.New("some departments not found in this tenant")
			}
		}

		// 3) 先清空，再覆盖写入
		if _, err = s.MemberDeptRepo.WithDB(tx).Delete(ctx, map[string]interface{}{
			"tenant_id": tenantID,
			"member_id": memberID,
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
		_, err = s.MemberDeptRepo.WithDB(tx).CreateBatch(ctx, rows)
		return err
	})
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

	// 统一用毫秒时间戳
	nowMs := time.Now().UnixMilli()

	// 1) 指定 JTI：单条撤销
	if strings.TrimSpace(jti) != "" {
		return s.RefreshTokenRepo.RevokeByJTI(ctx, jti, nowMs)
	}

	// 2) 全部撤销：需要 user_uuid 和 tenant_uuid
	// 2.1 查询 user_uuid
	u, err := s.UserRepo.FindByID(ctx, mem.UserID)
	if err != nil || u == nil {
		if err == nil {
			err = gorm.ErrRecordNotFound
		}
		return err
	}
	userUUID := u.UUID.String()

	// 2.2 查询 tenant_uuid（不引 tenantRepo，直接用 GORM 模型取）
	var tenantUUID string
	if err := s.DB.WithContext(ctx).
		Model(&tenantmdl.Tenant{}).
		Select("uuid").
		Where("id = ?", mem.TenantID).
		Scan(&tenantUUID).Error; err != nil {
		return err
	}
	if tenantUUID == "" {
		return gorm.ErrRecordNotFound
	}

	return s.RefreshTokenRepo.RevokeAllForUser(ctx, userUUID, tenantUUID, nowMs)
}

// ====== 查询某 user 在本租户的 member（仅返回 GORM 模型） ======
func (s *MemberService) GetMemberByUser(ctx context.Context, tenantID, userID uint64) (*modelIAM.Member, *modelIAM.User, error) {
	mem, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
		model.TableIAMMember + ".tenant_id = ?": tenantID,
		model.TableIAMMember + ".user_id = ?":   userID,
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	if mem == nil {
		return nil, nil, gorm.ErrRecordNotFound
	}
	u, err := s.UserRepo.FindByID(ctx, mem.UserID)
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		return nil, nil, gorm.ErrRecordNotFound
	}
	return mem, u, nil
}

// ====== 批量查询：返回 members 列表 + users 映射（key=user_id） ======
func (s *MemberService) BatchGetMembersByUsers(ctx context.Context, tenantID uint64, userIDs []uint64) ([]modelIAM.Member, map[uint64]modelIAM.User, error) {
	if len(userIDs) == 0 {
		return []modelIAM.Member{}, map[uint64]modelIAM.User{}, nil
	}
	// 去重
	seen := make(map[uint64]struct{}, len(userIDs))
	ids := make([]uint64, 0, len(userIDs))
	for _, id := range userIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	var members []modelIAM.Member
	if err := s.DB.WithContext(ctx).
		Table(model.TableIAMMember).
		Where("tenant_id = ? AND user_id IN ?", tenantID, ids).
		Where("deleted_at IS NULL").
		Find(&members).Error; err != nil {
		return nil, nil, err
	}
	if len(members) == 0 {
		return []modelIAM.Member{}, map[uint64]modelIAM.User{}, nil
	}

	uidSet := make(map[uint64]struct{}, len(members))
	uids := make([]uint64, 0, len(members))
	for _, m := range members {
		if _, ok := uidSet[m.UserID]; !ok {
			uidSet[m.UserID] = struct{}{}
			uids = append(uids, m.UserID)
		}
	}

	var users []modelIAM.User
	if err := s.DB.WithContext(ctx).
		Table(model.TableIAMUser).
		Where("id IN ?", uids).
		Where("deleted_at IS NULL").
		Find(&users).Error; err != nil {
		return nil, nil, err
	}
	userMap := make(map[uint64]modelIAM.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	return members, userMap, nil
}

// ====== 把已有 user 加入本租户（创建 member）——无入参 struct，纯参数 ======
func (s *MemberService) AddExistingUserAsMember(
	ctx context.Context,
	tenantID uint64,
	userID uint64,
	username string,
	displayName string,
	avatarURL string,
	status *int16,
	deptID *uint64,
	roleIDs []uint64, // 当前 Service 未接角色仓库，先保留参数位
	meta datatypes.JSON,
	initPassword *string, // 当前 Service 未接凭证仓库，先保留参数位
) (uint64, error) {

	username = strings.ToLower(strings.TrimSpace(username))
	if tenantID == 0 || userID == 0 || username == "" {
		return 0, errors.New("tenant_id/user_id/username required")
	}

	var newID uint64
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// user 是否存在
		u, err := s.UserRepo.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		if u == nil {
			return gorm.ErrRecordNotFound
		}

		// 不可重复加入同一租户
		if existed, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
			model.TableIAMMember + ".tenant_id = ?": tenantID,
			model.TableIAMMember + ".user_id = ?":   userID,
		}, nil); err != nil {
			return err
		} else if existed != nil {
			return errors.New("already a member of this tenant")
		}

		// username 租户内唯一
		if dup, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
			model.TableIAMMember + ".tenant_id = ?": tenantID,
			model.TableIAMMember + ".username = ?":  username,
		}, nil); err != nil {
			return err
		} else if dup != nil {
			return errors.New("username already taken in this tenant")
		}

		mem := &modelIAM.Member{
			TenantID:    tenantID,
			UserID:      userID,
			Username:    username,
			DisplayName: strings.TrimSpace(firstNonEmpty(displayName, username)),
			AvatarURL:   strings.TrimSpace(avatarURL),
			Status:      1,
			Meta:        meta,
		}
		if status != nil {
			mem.Status = *status
		}
		if _, err := s.MemberRepo.WithDB(tx).Create(ctx, mem); err != nil {
			return err
		}
		newID = mem.ID

		// 主部门（可选）
		if deptID != nil && *deptID != 0 {
			rows := []*modelIAM.MemberDepartment{{
				TenantID:     tenantID,
				MemberID:     mem.ID,
				DepartmentID: *deptID,
			}}
			if _, err := s.MemberDeptRepo.WithDB(tx).CreateBatch(ctx, rows); err != nil {
				return err
			}
		}

		// 角色/密码：当前 Service 未集成相关 Repo，这里不做；后续接入 Auth/Role 再补

		return nil
	})
	return newID, err
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
