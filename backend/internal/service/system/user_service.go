// internal/service/system/user_service.go
package system

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service/iam"
	coremdl "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
)

type UserService struct {
	DB               *gorm.DB
	UserRepo         *repoi.UserRepository
	MemberRepo       *repoi.MemberRepository
	MemberDeptRepo   *repoi.MemberDepartmentRepository
	CredRepo         *repoi.CredentialRepository
	RefreshTokenRepo *repoi.RefreshTokenRepository
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{
		DB:               db,
		UserRepo:         repoi.NewUserRepository(db),
		MemberRepo:       repoi.NewMemberRepository(db),
		MemberDeptRepo:   repoi.NewMemberDepartmentRepository(db),
		CredRepo:         repoi.NewCredentialRepository(db),
		RefreshTokenRepo: repoi.NewRefreshTokenRepository(db),
	}
}

// ---------------- 对外方法（给 Handler 用） ----------------

// ListUsers - 平台全局视角用户列表
// orderBy 传入例如："id desc" / "created_at asc"
func (s *UserService) ListUsers(ctx context.Context, f repoi.UserListFilter) ([]iam.MemberWithProfile, int64, error) {
	// 基础查询：u LEFT JOIN m（当指定租户时，限定 m.tenant_uuid；否则不限定）
	base := s.DB.WithContext(ctx).Table(coremdl.TableIAMUser + " AS u").
		Joins("LEFT JOIN " + coremdl.TableIAMMember + " AS m ON m.user_id = u.id AND m.deleted_at IS NULL")

	tenantUUID := strings.TrimSpace(f.TenantUUID)
	if tenantUUID != "" {
		// 指定租户视角：限定 m.tenant_uuid，并过滤掉不是该租户成员的行
		base = base.Joins("/* tenant filter */").Where("m.tenant_uuid = ?", tenantUUID).
			Where("m.id IS NOT NULL")
	}

	// 关键词（命中 u.email/u.phone/u.display_name + m.username）
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		kw = strings.ToLower(kw)
		p := "%" + kw + "%"
		base = base.Where("("+
			"LOWER(u.email) LIKE ? OR "+
			"LOWER(u.phone) LIKE ? OR "+
			"LOWER(u.display_name) LIKE ? OR "+
			"LOWER(m.username) LIKE ?"+
			")", p, p, p, p)
	}

	// 状态（这里按 User 的 status 过滤；如果你还要按 member.status 再加一个独立字段过滤）
	if f.Status != nil {
		base = base.Where("u.status = ?", *f.Status)
	}

	// 先算总数：对 u.id 计数（去重）
	var total int64
	if err := base.Select("u.id").Distinct("u.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []iam.MemberWithProfile{}, 0, nil
	}

	// 分页 + 排序：先分页取去重后的 u.id 列表（避免 PG: 42P10）
	order := "u.id DESC"
	if strings.TrimSpace(f.OrderBy) != "" {
		// 仅允许按 u.id / u.created_at / u.updated_at，避免复杂 order 触发 PG 限制
		switch strings.ToLower(strings.TrimSpace(f.OrderBy)) {
		case "id asc":
			order = "u.id ASC"
		case "id desc":
			order = "u.id DESC"
		case "created_at asc":
			order = "u.created_at ASC"
		case "created_at desc":
			order = "u.created_at DESC"
		case "updated_at asc":
			order = "u.updated_at ASC"
		case "updated_at desc":
			order = "u.updated_at DESC"
		default:
			order = "u.id DESC"
		}
	}

	page := f.Page
	size := f.Size
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}

	var ids []uint64
	if err := base.
		Select("u.id").
		Distinct("u.id").
		Order(order).
		Offset((page-1)*size).
		Limit(size).
		Pluck("u.id", &ids).Error; err != nil {
		return nil, 0, err
	}

	// 回表取 users
	var users []m.User
	if err := s.DB.WithContext(ctx).
		Table(coremdl.TableIAMUser+" AS u").
		Where("u.id IN ?", ids).
		Where("u.deleted_at IS NULL").
		Find(&users).Error; err != nil {
		return nil, 0, err
	}
	// 用映射方便组装
	uMap := make(map[uint64]m.User, len(users))
	for _, u := range users {
		uMap[u.ID] = u
	}

	// 如果是租户视角，再取 tenant 下的 member（user_id -> member）
	var mMap map[uint64]m.Member
	if tenantUUID != "" {
		var rows []m.Member
		if err := s.DB.WithContext(ctx).
			Table(coremdl.TableIAMMember).
			Where("tenant_uuid = ? AND user_id IN ?", tenantUUID, ids).
			Where("deleted_at IS NULL").
			Find(&rows).Error; err != nil {
			return nil, 0, err
		}
		mMap = make(map[uint64]m.Member, len(rows))
		for _, mm := range rows {
			// 按 user_id 映射该租户里的唯一 member
			mMap[mm.UserID] = mm
		}
	}

	// 最终拼装（保持 ids 顺序）
	out := make([]iam.MemberWithProfile, 0, len(ids))
	for _, id := range ids {
		u := uMap[id]
		var mem *m.Member
		if tenantUUID != "" {
			if mm, ok := mMap[id]; ok {
				mem = &mm
			}
		}
		out = append(out, iam.MemberWithProfile{
			User:   &u,
			Member: mem,
		})
	}
	return out, total, nil
}

func (s *UserService) GetUser(ctx context.Context, id uint64) (*m.User, error) {
	return s.UserRepo.FindByID(ctx, id)
}

// CreateSystemUser - Root 创建全局 User，并在指定租户创建一个 Member；可选写入密码凭证和部门关系
func (s *UserService) CreateSystemUser(
	ctx context.Context,
	user *m.User, // 直接用 GORM 模型
	tenantUUID string, // 要加入的租户
	username string, // 租户内用户名（唯一）
	initialPwd string, // 可选：初始化密码
	deptIDs []uint64, // 可选：部门
	roleIDs []uint64, // 可选：角色ID（为空时默认 role_user）
) (uint64, error) {

	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return 0, errors.New("tenant_uuid required")
	}
	username = utils.TrimLower(username)
	if username == "" {
		return 0, errors.New("username required")
	}

	// 兜底 user 字段整理
	user.Email = utils.TrimLower(user.Email)
	user.Phone = utils.Trim(user.Phone)
	user.DisplayName = utils.Trim(user.DisplayName)
	user.AvatarURL = utils.Trim(user.AvatarURL)
	user.Status = utils.IfZeroInt16(user.Status, 1)

	var userID uint64
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Create User
		if _, err := s.UserRepo.WithDB(tx).Create(ctx, user); err != nil {
			return err
		}
		userID = user.ID

		// 2) 校验：同租户内 username 唯一；同租户是否已存在该 user 的 member
		if dup, err := s.MemberRepo.
			WithDB(tx).
			GetByCondition(ctx, map[string]any{
				coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
				coremdl.TableIAMMember + ".username = ?":    username,
			}, nil); err != nil {
			return err
		} else if dup != nil {
			return errors.New("username already taken in this tenant")
		}
		if existed, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
			coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
			coremdl.TableIAMMember + ".user_id = ?":     user.ID,
		}, nil); err != nil {
			return err
		} else if existed != nil {
			return errors.New("already a member of this tenant")
		}

		// 3) Create Member
		mem := &m.Member{
			TenantUUID:  tenantUUID,
			UserID:      user.ID,
			Username:    username,
			DisplayName: utils.FirstNonEmpty(user.DisplayName, username),
			AvatarURL:   user.AvatarURL,
			Status:      user.Status,
			Meta:        user.Meta,
		}
		if _, err := s.MemberRepo.
			WithDB(tx).
			Create(ctx, mem); err != nil {
			return err
		}
		// 角色绑定：有显式角色时使用显式角色；否则默认 role_user。
		if err := s.applyMemberRolesTx(ctx, tx, tenantUUID, mem.ID, roleIDs); err != nil {
			return err
		}

		// 4) 可选：创建密码凭证（provider=password，identifier 优先 email > phone > username）
		if p := utils.Trim(initialPwd); p != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			identifier := utils.TrimLower(user.Email)
			if identifier == "" {
				identifier = utils.Trim(user.Phone)
			}
			if identifier == "" {
				identifier = username
			}
			if _, err = s.CredRepo.
				WithDB(tx).
				Create(ctx, &m.Credential{
					UserID:     user.ID,
					Provider:   "password",
					Identifier: identifier,
					SecretHash: string(hash), // bcrypt
					IsPrimary:  true,
				}); err != nil {
				return err
			}
		}

		// 5) 可选：部门隶属
		if len(deptIDs) > 0 {
			rows := make([]*m.MemberDepartment, 0, len(deptIDs))
			for _, did := range deptIDs {
				rows = append(rows, &m.MemberDepartment{
					TenantUUID:   tenantUUID,
					MemberID:     mem.ID,
					DepartmentID: did,
				})
			}
			if _, err := s.MemberDeptRepo.
				WithDB(tx).
				CreateBatch(ctx, rows); err != nil {
				return err
			}
		}

		return nil
	})
	return userID, err
}

// AddUserToTenant - 把已存在的 User 加入指定租户（创建 member）
func (s *UserService) AddUserToTenant(ctx context.Context, userID uint64, tenantUUID string) (memberID uint64, err error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if userID == 0 || tenantUUID == "" {
		return 0, errors.New("user_id/tenant_uuid required")
	}

	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 查 user
		u, err := s.UserRepo.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		if u == nil {
			return gorm.ErrRecordNotFound
		}

		// 2) 已是该租户成员？
		if existed, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
			coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
			coremdl.TableIAMMember + ".user_id = ?":     userID,
		}, nil); err != nil {
			return err
		} else if existed != nil {
			return errors.New("already a member of this tenant")
		}

		// 3) 生成 base username
		base := deriveBaseUserName(u)
		// 4) 确保租户内唯一
		username, err := s.ensureUniqueUserName(ctx, tenantUUID, base)
		if err != nil {
			return err
		}

		// 5) 继承显示名/头像/状态
		displayName := strings.TrimSpace(u.DisplayName)
		if displayName == "" {
			displayName = username
		}
		avatar := strings.TrimSpace(u.AvatarURL)
		status := u.Status
		if status == 0 {
			status = 1
		}

		mem := &m.Member{
			TenantUUID:  tenantUUID,
			UserID:      userID,
			Username:    username,
			DisplayName: displayName,
			AvatarURL:   avatar,
			Status:      status,
			Meta:        u.Meta, // 如不想继承可置空
		}
		if _, err := s.MemberRepo.WithDB(tx).Create(ctx, mem); err != nil {
			return err
		}
		// 默认绑定租户普通角色，和创建用户路径保持一致
		if err := s.applyMemberRolesTx(ctx, tx, tenantUUID, mem.ID, nil); err != nil {
			return err
		}
		memberID = mem.ID
		return nil
	})
	return memberID, err
}

func (s *UserService) ListUserRoleIDs(ctx context.Context, userID uint64, tenantUUID string) ([]uint64, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if userID == 0 || tenantUUID == "" {
		return nil, errors.New("user_id/tenant_uuid required")
	}

	member, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
		coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
		coremdl.TableIAMMember + ".user_id = ?":     userID,
	}, nil)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return []uint64{}, nil
	}

	roles, err := repoi.NewRoleBindingRepository(s.DB).ListRolesByMember(ctx, tenantUUID, member.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(roles))
	for _, role := range roles {
		ids = append(ids, role.ID)
	}
	return ids, nil
}

func (s *UserService) SetUserRoleIDs(ctx context.Context, userID uint64, tenantUUID string, roleIDs []uint64) error {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if userID == 0 || tenantUUID == "" {
		return errors.New("user_id/tenant_uuid required")
	}

	member, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
		coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
		coremdl.TableIAMMember + ".user_id = ?":     userID,
	}, nil)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("member not found in tenant")
	}

	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.applyMemberRolesTx(ctx, tx, tenantUUID, member.ID, roleIDs)
	})
}

func (s *UserService) applyMemberRolesTx(ctx context.Context, tx *gorm.DB, tenantUUID string, memberID uint64, roleIDs []uint64) error {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" || memberID == 0 {
		return errors.New("tenant_uuid/member_id required")
	}

	targetRoleIDs := make([]uint64, 0, len(roleIDs))
	seen := make(map[uint64]struct{}, len(roleIDs))
	for _, id := range roleIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		targetRoleIDs = append(targetRoleIDs, id)
	}

	// 没有显式角色时，兜底到 role_user。
	if len(targetRoleIDs) == 0 {
		var roleUser m.Role
		if err := tx.WithContext(ctx).
			Where("scope = ? AND tenant_uuid = ? AND code = ?", "tenant", tenantUUID, "role_user").
			First(&roleUser).Error; err != nil {
			return err
		}
		targetRoleIDs = []uint64{roleUser.ID}
	}

	var validRoleIDs []uint64
	if err := tx.WithContext(ctx).
		Model(&m.Role{}).
		Where("scope = ? AND tenant_uuid = ? AND id IN ?", "tenant", tenantUUID, targetRoleIDs).
		Pluck("id", &validRoleIDs).Error; err != nil {
		return err
	}
	if len(validRoleIDs) != len(targetRoleIDs) {
		return errors.New("contains invalid tenant role ids")
	}

	if err := tx.WithContext(ctx).
		Where("tenant_uuid = ? AND subject_type = ? AND subject_id = ?", tenantUUID, m.SubMember, memberID).
		Delete(&m.RoleBinding{}).Error; err != nil {
		return err
	}

	bindings := make([]m.RoleBinding, 0, len(validRoleIDs))
	for _, rid := range validRoleIDs {
		bindings = append(bindings, m.RoleBinding{
			TenantUUID:  tenantUUID,
			RoleID:      rid,
			SubjectType: m.SubMember,
			SubjectID:   memberID,
		})
	}
	return tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&bindings).Error
}

func deriveBaseUserName(u *m.User) string {
	// 优先 email 本地部分
	if e := strings.TrimSpace(strings.ToLower(u.Email)); e != "" {
		if at := strings.IndexByte(e, '@'); at > 0 {
			return e[:at]
		}
		return e
	}
	// 其次手机号
	if p := strings.TrimSpace(u.Phone); p != "" {
		return p
	}
	// 兜底：user<ID>
	return "user" + strconv.FormatUint(u.ID, 10)
}

// ensureUniqueUserName 在指定租户下保证 username 唯一，不唯一则在后缀加 -2, -3...
func (s *UserService) ensureUniqueUserName(ctx context.Context, tenantUUID string, base string) (string, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	username := strings.ToLower(strings.TrimSpace(base))
	if username == "" {
		username = "user"
	}
	try := func(name string) (bool, error) {
		dup, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
			coremdl.TableIAMMember + ".tenant_uuid = ?": tenantUUID,
			coremdl.TableIAMMember + ".username = ?":    name,
		}, nil)
		if err != nil {
			return false, err
		}
		return dup == nil, nil
	}
	if ok, err := try(username); err != nil {
		return "", err
	} else if ok {
		return username, nil
	}
	// 加序号直到唯一
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", username, i)
		if ok, err := try(candidate); err != nil {
			return "", err
		} else if ok {
			return candidate, nil
		}
	}
	return "", errors.New("cannot allocate unique username")
}

// UpdateUser - 更新用户字段（按 map 透传）
func (s *UserService) UpdateUser(ctx context.Context, id uint64, updates map[string]any) error {
	if id == 0 {
		return errors.New("id required")
	}
	// 规范化常见字段
	if v, ok := updates["email"].(string); ok {
		updates["email"] = utils.TrimLower(v)
	}
	if v, ok := updates["phone"].(string); ok {
		updates["phone"] = utils.Trim(v)
	}
	if v, ok := updates["display_name"].(string); ok {
		updates["display_name"] = utils.Trim(v)
	}
	if v, ok := updates["avatar_url"].(string); ok {
		updates["avatar_url"] = utils.Trim(v)
	}
	_, err := s.UserRepo.Patch(ctx, map[string]any{"id": id}, updates)
	return err
}

func (s *UserService) SetUserStatus(ctx context.Context, id uint64, status int16) error {
	if id == 0 {
		return errors.New("id required")
	}
	isRoot, err := s.UserRepo.IsRootUser(ctx, id)
	if err != nil {
		return err
	}
	if isRoot {
		return errors.New("root user status cannot be changed")
	}
	_, err = s.UserRepo.Patch(ctx, map[string]any{"id": id}, map[string]any{"status": status})
	return err
}

func (s *UserService) DeleteUser(ctx context.Context, id uint64) error {
	if id == 0 {
		return errors.New("id required")
	}
	isRoot, err := s.UserRepo.IsRootUser(ctx, id)
	if err != nil {
		return err
	}
	if isRoot {
		return errors.New("root user cannot be deleted")
	}
	_, err = s.UserRepo.Delete(ctx, map[string]any{"id": id}, nil, true)
	return err
}

func (s *UserService) RestoreUser(ctx context.Context, id uint64) error {
	if id == 0 {
		return errors.New("id required")
	}
	// 直接 Unscoped 更新 deleted_at = NULL
	return s.DB.WithContext(ctx).Unscoped().
		Model(&m.User{}).
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

// ResetUserPassword - Root 视角重置用户密码。
// 规则：
// 1) 不依赖邮件流程，直接写入密码哈希；
// 2) 同时维护 email/phone/username 三类 identifier 的 password 凭证；
// 3) 若 identifier 已被其他 user 占用，直接报错阻止覆盖。
func (s *UserService) ResetUserPassword(ctx context.Context, id uint64, password string) error {
	if id == 0 {
		return errors.New("id required")
	}
	password = strings.TrimSpace(password)
	if len(password) < 6 {
		return errors.New("password too short")
	}

	u, err := s.UserRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return gorm.ErrRecordNotFound
	}

	members, err := s.MemberRepo.ListByUserID(ctx, id)
	if err != nil {
		return err
	}

	identSet := map[string]struct{}{}
	addIdent := func(v string) {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			identSet[v] = struct{}{}
		}
	}
	addIdent(u.Email)
	addIdent(u.Phone)
	for _, mm := range members {
		if mm != nil {
			addIdent(mm.Username)
		}
	}
	if len(identSet) == 0 {
		return errors.New("no identifier found for user")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	secret := string(hash)

	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for ident := range identSet {
			cred, ferr := s.CredRepo.FindByProviderIdentifier(ctx, "password", ident)
			if ferr == nil && cred != nil {
				if cred.UserID != id {
					return fmt.Errorf("identifier %s is already used by another user", ident)
				}
				if err := tx.WithContext(ctx).
					Model(&m.Credential{}).
					Where("id = ?", cred.ID).
					Updates(map[string]any{
						"secret_hash": secret,
						"is_primary":  true,
					}).Error; err != nil {
					return err
				}
				continue
			}
			if ferr != nil && !errors.Is(ferr, gorm.ErrRecordNotFound) {
				return ferr
			}
			if _, err := s.CredRepo.WithDB(tx).Create(ctx, &m.Credential{
				UserID:     id,
				Provider:   "password",
				Identifier: ident,
				SecretHash: secret,
				IsPrimary:  true,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// ForceLogoutByJTI - 按 JTI 精确撤销 refresh token
func (s *UserService) ForceLogoutByJTI(ctx context.Context, jti string, nowMillis int64) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return errors.New("jti required")
	}
	// RefreshTokenRepo.RevokeByJTI(ctx, jti, nowMillis)
	return s.RefreshTokenRepo.RevokeByJTI(ctx, jti, nowMillis)
}

// （可选）如果你后面要做“按 user 全部下线”，建议在 RefreshTokenRepository
// 新增方法：RevokeAllForUserUUID(ctx, userUUID string, revokedAtMillis int64) error
// 然后在这里封装一个 ForceLogoutAllForUser(ctx, userID uint64) 去查 user.UUID 再调用。
