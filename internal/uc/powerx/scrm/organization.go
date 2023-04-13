package scrm

import (
	"PowerX/internal/model/scrm/organization"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrganizationUseCase struct {
	db     *gorm.DB
	wework *work.Work
}

func NewOrganizationUseCase(db *gorm.DB, wework *work.Work) *OrganizationUseCase {
	return &OrganizationUseCase{db: db, wework: wework}
}

func (uc *OrganizationUseCase) SyncDepartmentsAndEmployees(ctx context.Context) {
	list, err := uc.wework.Department.SimpleList(ctx, 1)
	if err != nil {
		panic(errors.Wrap(err, "list wework departments failed"))
	}

	for _, d := range list.DepartmentIDs {
		detail, err := uc.wework.Department.Get(ctx, d.ID)
		if err != nil {
			panic(errors.Wrap(err, "get wework department detail failed"))
		}
		dep := organization.WeWorkDepartment{
			WeWorkDepId:    detail.Department.ID,
			Name:           detail.Department.Name,
			NameEn:         detail.Department.NameEN,
			WeWorkParentId: detail.Department.ParentID,
			Order:          detail.Department.Order,
		}
		uc.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dep_id"}},
			UpdateAll: true,
		}).Create(&dep)

		// user
		users, err := uc.wework.User.GetDetailedDepartmentUsers(ctx, d.ID, 0)
		if err != nil {
			panic(errors.Wrap(err, "get wework department users failed"))
		}

		for _, u := range users.UserList {
			emp := organization.WeWorkEmployee{
				WeWorkUserId:           u.UserID,
				Name:                   u.Name,
				Position:               u.Position,
				OpenUserid:             u.OpenUserID,
				WeWorkMainDepartmentId: u.MainDepartment,
				Status:                 u.Status,
				QrCode:                 u.QrCode,
			}
			uc.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "we_work_user_id"}},
				UpdateAll: true,
			}).Create(&emp)
		}
	}
}
