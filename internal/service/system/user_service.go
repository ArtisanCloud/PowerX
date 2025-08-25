// internal/service/system/user_service.go
package system

import (
	"context"
	"errors"
	"fmt"
	coremdl "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
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
func (s *UserService) ListUsers(ctx context.Context, keyword string, status *int16, page, size int, orderBy string) ([]m.User, int64, error) {
	filter := repoi.UserListFilter{
		Keyword: keyword,
		Status:  status,
		Page:    page,
		Size:    size,
		OrderBy: strings.TrimSpace(orderBy),
	}
	return s.UserRepo.List(ctx, filter)
}

func (s *UserService) GetUser(ctx context.Context, id uint64) (*m.User, error) {
	return s.UserRepo.FindByID(ctx, id)
}

// CreateSystemUser - Root 创建全局 User，并在指定租户创建一个 Member；可选写入密码凭证和部门关系
func (s *UserService) CreateSystemUser(
	ctx context.Context,
	user *m.User, // 直接用 GORM 模型
	tenantID uint64, // 要加入的租户
	username string, // 租户内用户名（唯一）
	initialPwd string, // 可选：初始化密码
	deptIDs []uint64, // 可选：部门
) (uint64, error) {

	if tenantID == 0 {
		return 0, errors.New("tenant_id required")
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
				//coremdl.TableIAMMember + ".tenant_id = ?": tenantID,
				coremdl.TableIAMMember + ".username = ?": username,
			}, nil); err != nil {
			return err
		} else if dup != nil {
			return errors.New("username already taken in this tenant")
		}
		if existed, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
			//coremdl.TableIAMMember + ".tenant_id = ?": tenantID,
			coremdl.TableIAMMember + ".user_id = ?": user.ID,
		}, nil); err != nil {
			return err
		} else if existed != nil {
			return errors.New("already a member of this tenant")
		}

		// 3) Create Member
		mem := &m.Member{
			TenantID:    tenantID,
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

		// 4) 可选：创建密码凭证（provider=password，identifier 采用 username）
		if p := utils.Trim(initialPwd); p != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			if _, err = s.CredRepo.
				WithDB(tx).
				Create(ctx, &m.Credential{
					UserID:     user.ID,
					Provider:   "password",
					Identifier: username,     // 也可换成 email/phone
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
					TenantID:     tenantID,
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
func (s *UserService) AddUserToTenant(ctx context.Context, userID uint64, tenantID uint64) (memberID uint64, err error) {
	if userID == 0 || tenantID == 0 {
		return 0, errors.New("user_id/tenant_id required")
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
			coremdl.TableIAMMember + ".tenant_id = ?": tenantID,
			coremdl.TableIAMMember + ".user_id = ?":   userID,
		}, nil); err != nil {
			return err
		} else if existed != nil {
			return errors.New("already a member of this tenant")
		}

		// 3) 生成 base username
		base := deriveBaseUserName(u)
		// 4) 确保租户内唯一
		username, err := s.ensureUniqueUserName(ctx, tenantID, base)
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
			TenantID:    tenantID,
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
		memberID = mem.ID
		return nil
	})
	return memberID, err
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
func (s *UserService) ensureUniqueUserName(ctx context.Context, tenantID uint64, base string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(base))
	if username == "" {
		username = "user"
	}
	try := func(name string) (bool, error) {
		dup, err := s.MemberRepo.GetByCondition(ctx, map[string]any{
			coremdl.TableIAMMember + ".username = ?": name,
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
	_, err := s.UserRepo.Patch(ctx, map[string]any{"id = ?": id}, updates)
	return err
}

func (s *UserService) SetUserStatus(ctx context.Context, id uint64, status int16) error {
	if id == 0 {
		return errors.New("id required")
	}
	_, err := s.UserRepo.Patch(ctx, map[string]any{"id = ?": id}, map[string]any{"status": status})
	return err
}

func (s *UserService) DeleteUser(ctx context.Context, id uint64) error {
	if id == 0 {
		return errors.New("id required")
	}
	_, err := s.UserRepo.Delete(ctx, map[string]any{"id = ?": id}, nil, true)
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
