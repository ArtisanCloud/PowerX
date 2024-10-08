package wechat

import (
	organization2 "PowerX/internal/model/organization"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/types"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/department/response"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/user/request"
	"gorm.io/gorm"
	"strings"
)

// CreateWeWorkUserRequest
//
//	@Description:
//	@receiver uc
//	@param ctx
//	@param dep
//	@return error
func (this *wechatUseCase) CreateWeWorkUserRequest(ctx context.Context, user *organization.WeWorkUser) (err error) {

	create, err := this.wework.User.Create(ctx, this.userModelToWeWorkRequest(user))
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.create.wework.user.error`, *create)
	}

	if err == nil {
		this.modelWeworkOrganization.user.Action(this.db, []*organization.WeWorkUser{user})
	}

	return err

}

// UpdateWeWorkUserRequest
//
//	@Description:
//	@receiver this
//	@param ctx
//	@param dep
//	@return err
func (this *wechatUseCase) UpdateWeWorkUserRequest(ctx context.Context, user *organization.WeWorkUser) (err error) {

	update, err := this.wework.User.Update(ctx, this.userModelToWeWorkRequest(user))

	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.update.wework.organization.user.error`, *update)
	}

	if err == nil {
		this.modelWeworkOrganization.user.Action(this.db, []*organization.WeWorkUser{user})
	}
	return err

}

// userModelToWeWorkRequest
//
//	@Description:
//	@param user
//	@return *request.RequestUserDetail
func (this *wechatUseCase) userModelToWeWorkRequest(user *organization.WeWorkUser) *request.RequestUserDetail {

	return &request.RequestUserDetail{
		Userid:         user.WeWorkUserId,
		Name:           user.Name,
		Alias:          user.Alias,
		Mobile:         user.Mobile,
		Position:       user.Position,
		Email:          user.Email,
		BizMail:        user.BizMail,
		Telephone:      user.Telephone,
		Address:        user.Address,
		MainDepartment: user.WeWorkMainDepartmentId,
	}
}

// PullSyncDepartmentsAndUsersRequest
//
//	@Description:
//	@receiver uc
//	@param ctx
//	@return error
func (this *wechatUseCase) PullSyncDepartmentsAndUsersRequest(ctx context.Context) error {

	list, err := this.wework.Department.SimpleList(ctx, 1)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.pull.wework.sync.organization.list.error`, list.ResponseWork)
	}

	if err != nil {
		return err
	}

	this.gLock.Add(len(list.DepartmentIDs))
	for _, val := range list.DepartmentIDs {
		go func(val response.DepartmentID) {
			defer this.gLock.Done()
			this.deparment(val)
			this.user(val)

		}(val)

	}
	this.gLock.Wait()
	return err
}

// deparment
//
//	@Description:
//	@receiver this
//	@param val
func (this *wechatUseCase) deparment(val response.DepartmentID) {

	department, err := this.wework.Department.Get(this.ctx, val.ID)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.wechat.sync.organization.department.error`, department.ResponseWork)
	}

	if err == nil && department.Department != nil {

		this.modelWeworkOrganization.department.Action(this.db, []*organization.WeWorkDepartment{
			{
				WeWorkDepId:      department.Department.ID,
				Name:             department.Department.Name,
				NameEn:           department.Department.NameEN,
				WeWorkParentId:   department.Department.ParentID,
				Order:            department.Department.Order,
				DepartmentLeader: strings.Join(department.Department.DepartmentLeaders, `,`),
			},
		})

	}

}

// user
//
//	@Description:
//	@receiver this
//	@param val
func (this *wechatUseCase) user(val response.DepartmentID) {

	users, err := this.wework.User.GetDetailedDepartmentUsers(this.ctx, val.ID, 0)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.wework.sync.organization.user.error`, users.ResponseWork)
	}

	if err == nil && len(users.UserList) > 0 {
		users := []*organization.WeWorkUser{}
		for _, user := range users {
			if user != nil {
				open, _ := this.wework.User.UserIdToOpenID(this.ctx, user.WeWorkUserId)
				users = append(users, &organization.WeWorkUser{
					WeWorkUserId:           user.WeWorkUserId,
					Name:                   user.Name,
					Position:               user.Position,
					Mobile:                 user.WeWorkUserId,
					Email:                  user.Email,
					Alias:                  user.Alias,
					OpenUserId:             open.OpenID,
					WeWorkMainDepartmentId: user.WeWorkMainDepartmentId,
					Status:                 user.Status,
					QrCode:                 user.QrCode,
					RefUserId:              0,
				})
			}
		}
		this.modelWeworkOrganization.user.Action(this.db, users)
		// sync to local
		//this.modelOrganization.user.Action(this.db, this.userFromWeWorkSyncToLocal(users))

	}

}

// buildFindManyUsersQueryNoPage
//
//	@Description:
//	@param query
//	@param opt
//	@return *gorm.DB
func buildFindManyUsersQueryNoPage(query *gorm.DB, opt *FindManyWechatUsersOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}
	if len(opt.Emails) > 0 {
		query.Where("email in ?", opt.Emails)
	}
	if len(opt.Mobile) > 0 {
		query.Where("mobile in ?", opt.Mobile)
	}
	if len(opt.Alias) > 0 {
		query.Where("alias in ?", opt.Alias)
	}
	if len(opt.OpenUserId) > 0 {
		query.Where("open_user_id in ?", opt.OpenUserId)
	}
	if len(opt.WeWorkMainDepartmentId) > 0 {
		query.Where("we_work_main_department_id in ? ", opt.WeWorkMainDepartmentId)
	}
	if len(opt.Status) > 0 {
		query.Where("status in ?", opt.Status)
	}
	return query
}

// FindManyWechatUsersPage
//
//	@Description:
//	@receiver uc
//	@param ctx
//	@param opt
//	@return *types.Page[*organization.WeWorkUser]
//	@return error
func (this *wechatUseCase) FindManyWechatUsersPage(ctx context.Context, opt *types.PageOption[FindManyWechatUsersOption]) (*types.Page[*organization.WeWorkUser], error) {

	var users []*organization.WeWorkUser
	var count int64

	query := this.db.WithContext(ctx).Table(this.modelWeworkOrganization.user.TableName())

	if opt.PageIndex == 0 {
		opt.PageIndex = 1
	}
	if opt.PageSize == 0 {
		opt.PageSize = powermodel.PageDefaultSize
	}
	query = buildFindManyUsersQueryNoPage(query, &opt.Option)

	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	err := query.Find(&users).Error

	return &types.Page[*organization.WeWorkUser]{
		List:      users,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, err
}

// getWechatUserIDs
//
//	@Description:
//	@receiver uc
//	@param ctx
//	@param opt
//	@return *types.Page[*organization.WeWorkUser]
//	@return error
func (this *wechatUseCase) getWechatUserIDs(ctx context.Context) (ids []string, err error) {

	ids = organization.AdapterUserSliceUserIDs(func(users []*organization.WeWorkUser) (ids []string) {
		for _, user := range users {
			ids = append(ids, user.WeWorkUserId)
		}
		return ids
	})(this.modelWeworkOrganization.user.Query(this.db))

	return ids, err

}

// userFromWeWorkSyncToLocal
//
//	@Description:
//	@receiver this
//	@param fromUser
//	@return toUser
func (this *wechatUseCase) userFromWeWorkSyncToLocal(fromUser []*organization.WeWorkUser) (toUser []*organization2.User) {

	if fromUser != nil {
		password, _ := organization2.HashPassword(`123456`)
		for _, user := range fromUser {
			toUser = append(toUser, &organization2.User{
				Account:  user.WeWorkUserId,
				Name:     user.Name,
				NickName: user.Name,
				// todo Position 关联
				DepartmentId:  int64(user.WeWorkMainDepartmentId),
				MobilePhone:   user.Mobile,
				Gender:        user.Gender,
				Email:         user.Email,
				ExternalEmail: user.Email,
				Avatar:        user.Avatar,
				Password:      password,
				WeWorkUserId:  user.WeWorkUserId,
			})
		}
	}

	return toUser
}
