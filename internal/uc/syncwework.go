package uc

import (
	"PowerX/internal/types"
	"PowerX/pkg/slicex"
	"PowerX/pkg/treex"
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/user/request"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"strconv"
	"time"
)

type SyncWeWorkUseCase struct {
	db *gorm.DB
	*WeWorkUseCase
	employee   *EmployeeUseCase
	department *DepartmentUseCase
	auth       *AuthUseCase
	tag        *TagUseCase
}

func newSyncWeWorkUseCase(db *gorm.DB, wework *WeWorkUseCase, employee *EmployeeUseCase, department *DepartmentUseCase, auth *AuthUseCase, tag *TagUseCase) *SyncWeWorkUseCase {
	return &SyncWeWorkUseCase{
		db:            db,
		WeWorkUseCase: wework,
		employee:      employee,
		department:    department,
		tag:           tag,
		auth:          auth,
	}
}

type WeWorkDepartment struct {
	ID       int64 `gorm:"unique"`
	Name     string
	NameEN   string
	Leaders  pq.StringArray `gorm:"type:text[]"`
	ParentId int64
	DepOrder int32
	*types.SyncModel

	RelationDepartmentId int64
}

type WeWorkEmployee struct {
	ID               string `gorm:"unique"`
	Name             string
	DepartmentIds    pq.Int64Array `gorm:"type:bigint[]"`
	DepartmentOrders pq.Int32Array `gorm:"type:int[]"`
	Position         string
	Gender           string
	Email            string
	BizMail          string
	Mobile           string
	IsLeaderInDept   pq.Int32Array  `gorm:"type:int[]"`
	DirectLeader     pq.StringArray `gorm:"type:text[]"`
	Avatar           string
	ThumbAvatar      string
	Telephone        string
	Alias            string
	Status           int
	Address          string
	EnglishName      string
	OpenUserId       string
	MainDepartment   int
	*types.SyncModel

	RelationEmployeeId int64
}

func (s *SyncWeWorkUseCase) FetchDepartments(ctx context.Context) {
	depResult, err := s.API.Department.List(ctx, 0)
	if err != nil {
		panic(errors.Wrap(err, "fetch deps failed"))
	}
	if depResult.ErrCode != 0 {
		panic(errors.New(fmt.Sprintf("%d: %s", depResult.ErrCode, depResult.ErrMSG)))
	}

	var deps []*WeWorkDepartment
	for _, dep := range depResult.Departments {
		deps = append(deps, &WeWorkDepartment{
			ID:       int64(dep.ID),
			Name:     dep.Name,
			NameEN:   dep.NameEN,
			Leaders:  dep.DepartmentLeaders,
			ParentId: int64(dep.ParentID),
			DepOrder: int32(dep.Order),
		})
	}
	if err := s.db.Model(&WeWorkDepartment{}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		// todo update set
		DoNothing: true,
	}).CreateInBatches(&deps, 20).Error; err != nil {
		panic(errors.Wrap(err, "save wework data failed"))
	}
}

// FetchEmployees 在调用该方法前应该先调用 FetchDepartments, 确保表内有值
func (s *SyncWeWorkUseCase) FetchEmployees(ctx context.Context) {
	var deps []*WeWorkDepartment
	if err := s.db.WithContext(ctx).Model(&WeWorkDepartment{}).Find(&deps).Error; err != nil {
		panic(errors.Wrap(err, "find all wework deps failed"))
	}
	var employees []*WeWorkEmployee
	for _, dep := range deps {
		userResult, err := s.API.User.GetDetailedDepartmentUsers(ctx, int(dep.ID), 0)
		if err != nil {
			panic(errors.Wrap(err, "fetch employees filed"))
		}
		if userResult.ErrCode != 0 {
			panic(errors.New(fmt.Sprintf("%d: %s", userResult.ErrCode, userResult.ErrMSG)))
		}
		for _, employee := range userResult.UserList {
			var depIds []int64
			for _, v := range employee.Department {
				depIds = append(depIds, int64(v))
			}
			var orders []int32
			for _, o := range employee.Order {
				orders = append(orders, int32(o))
			}
			var isLeaders []int32
			for _, i := range employee.IsLeaderInDept {
				isLeaders = append(isLeaders, int32(i))
			}
			employees = append(employees, &WeWorkEmployee{
				ID:               employee.UserID,
				Name:             employee.Name,
				DepartmentIds:    depIds,
				DepartmentOrders: orders,
				Position:         employee.Position,
				Gender:           employee.Gender,
				Email:            employee.Email,
				// sdk miss biz_mail
				BizMail:        "",
				Mobile:         employee.Mobile,
				IsLeaderInDept: isLeaders,
				// sdk miss direct_leader
				DirectLeader: []string{},
				Avatar:       employee.Avatar,
				ThumbAvatar:  employee.ThumbAvatar,
				Telephone:    employee.Telephone,
				Alias:        employee.Alias,
				Status:       employee.Status,
				// sdk miss address
				Address:        "",
				EnglishName:    employee.EnglishName,
				OpenUserId:     employee.OpenUserID,
				MainDepartment: employee.MainDepartment,
				SyncModel: &types.SyncModel{
					SyncID:    0,
					CreatedAt: time.Time{},
					UpdatedAt: time.Time{},
					DeletedAt: gorm.DeletedAt{
						Time:  time.Time{},
						Valid: false,
					},
				},
				RelationEmployeeId: 0,
			})
			if err := s.db.Model(&WeWorkEmployee{}).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				// todo update set
				DoNothing: true,
			}).Create(&employees).Error; err != nil {
				panic(errors.Wrap(err, "save employees failed"))
			}
			employees = employees[:0]
		}
	}
}

func (s *SyncWeWorkUseCase) SyncDepartmentsToSystem(ctx context.Context) {
	var deps []*WeWorkDepartment
	if err := s.db.WithContext(ctx).Model(&WeWorkDepartment{}).Find(&deps).Error; err != nil {
		panic(errors.Wrap(err, "find all wework department failed"))
	}
	// 企业微信部门转化为部门树
	depTree, err := treex.MakeTree[*WeWorkDepartment, int64](deps, func(t *WeWorkDepartment) int64 {
		return t.ID
	}, func(t *WeWorkDepartment) int64 {
		return t.ParentId
	}, 1)
	if err != nil {
		panic(errors.Wrap(err, "wework department make tree failed"))
	}
	// 将企业微信的树层级copy为系统内的部门树
	depPointMap := make(map[*Department]*WeWorkDepartment)
	var copyTree func(depTree treex.Node[*WeWorkDepartment]) treex.Node[*Department]
	copyTree = func(depTree treex.Node[*WeWorkDepartment]) treex.Node[*Department] {
		node := treex.Node[*Department]{
			Elem: &Department{
				Name: depTree.Elem.Name,
			},
		}
		depPointMap[node.Elem] = depTree.Elem
		var children []treex.Node[*Department]
		for _, child := range depTree.Children {
			children = append(children, copyTree(child))
		}
		node.Children = children
		return node
	}
	sysDepTree := copyTree(*depTree)
	if sysDepTree.Elem.PId == 0 {
		department, err := s.department.FindOneDepartment(ctx, FindOneDepartmentOption{})
		if err != nil {
			panic(errors.Wrap(err, "dep init?"))
		}
		sysDepTree.Elem.PId = department.ID
	}

	// 按照层级持久化
	var saveDepTree func(node treex.Node[*Department])
	saveDepTree = func(node treex.Node[*Department]) {
		relationDep := depPointMap[node.Elem]
		if relationDep.RelationDepartmentId == 0 {
			err = s.department.CreateDepartment(ctx, node.Elem)
			if err != nil {
				panic(errors.Wrap(err, "save wework dep relation failed"))
			}
		} else {
			node.Elem, err = s.department.FindOneDepartment(ctx, FindOneDepartmentOption{Id: &relationDep.RelationDepartmentId})
			if err != nil {
				panic(errors.Wrap(err, "save wework dep relation failed"))
			}
		}

		err = s.db.WithContext(ctx).Model(&WeWorkDepartment{}).Where(relationDep.SyncID).
			Update("relation_department_id", node.Elem.ID).Error
		if err != nil {
			panic(errors.Wrap(err, "save wework dep relation failed"))
		}
		for _, child := range node.Children {
			child.Elem.PId = node.Elem.ID
			child.Elem.AncestorIds = append(node.Elem.AncestorIds, child.Elem.PId)
			saveDepTree(child)
		}
	}
	saveDepTree(sysDepTree)
}

func (s *SyncWeWorkUseCase) SyncEmployeeToSystem(ctx context.Context) {
	var weworkEmployees []*WeWorkEmployee
	var insertEmployees []*Employee
	var updateEmployees []*Employee
	var offset int
	var count int64
	err := s.db.WithContext(ctx).Model(&WeWorkEmployee{}).Count(&count).Error
	if err != nil {
		panic(errors.Wrap(err, "count wework insertEmployees failed"))
	}

	var weworkDeps []*WeWorkDepartment
	err = s.db.WithContext(ctx).Model(&WeWorkDepartment{}).Select("id", "relation_department_id").Find(&weworkDeps).Error
	if err != nil {
		panic(errors.Wrap(err, "find relation_department_id failed"))
	}
	depIdMap := make(map[int64]int64)
	for _, dep := range weworkDeps {
		depIdMap[dep.ID] = dep.RelationDepartmentId
	}

	for int64(offset) < count {
		if err := s.db.WithContext(ctx).Model(&WeWorkEmployee{}).Offset(offset).Limit(100).Find(&weworkEmployees).Error; err != nil {
			panic(errors.Wrap(err, "find all wework employee failed"))
		}
		relationMap := make(map[*Employee]*WeWorkEmployee)
		for _, e := range weworkEmployees {
			gender, _ := strconv.ParseInt(e.Gender, 10, 8)
			var status EmployeeStatus
			if e.Status == 1 {
				status = EmployeeStatusEnable
			}
			if e.Status > 1 {
				status = EmployeeStatusDisable
			}
			var depIds pq.Int64Array
			for _, id := range e.DepartmentIds {
				rid, ok := depIdMap[id]
				if !ok {
					continue
				}
				depIds = append(depIds, rid)
			}

			g := Gender(gender)
			ie := Employee{
				// todo define sync account rule
				Account:       e.Mobile,
				Name:          e.Name,
				Position:      e.Position,
				DepartmentIds: depIds,
				MobilePhone:   e.Mobile,
				Gender:        &g,
				Email:         e.Email,
				Avatar:        e.Avatar,
				Status:        &status,
				Password:      "123456",
			}
			relationMap[&ie] = e
			if e.RelationEmployeeId != 0 {
				ie.Model = &types.Model{
					ID: e.RelationEmployeeId,
				}
				updateEmployees = append(updateEmployees, &ie)
			} else {
				insertEmployees = append(insertEmployees, &ie)
			}
		}
		s.employee.CreateEmployees(ctx, insertEmployees)
		for _, employee := range insertEmployees {
			wEmployee := relationMap[employee]
			err = s.db.WithContext(ctx).Model(&WeWorkEmployee{}).Where(wEmployee.SyncID).
				Update("relation_employee_id", employee.ID).Error
			if err != nil {
				panic(errors.Wrap(err, "update wework employee relation failed"))
			}
		}
		for _, employee := range updateEmployees {
			s.employee.UpdateEmployeeById(ctx, employee)
		}

		if err != nil {
			panic(errors.Wrap(err, "create consumer for employee failed"))
		}

		weworkEmployees = weworkEmployees[:0]
		insertEmployees = insertEmployees[:0]
		updateEmployees = updateEmployees[:0]
		offset = offset + 100
	}
}

func (s *SyncWeWorkUseCase) SyncDepartmentsLeadersToSystem(ctx context.Context) {
	var employees []*WeWorkEmployee
	if err := s.db.WithContext(ctx).Model(&WeWorkEmployee{}).Find(&employees).Error; err != nil {
		panic(errors.Wrap(err, "find all wework employee failed"))
	}
	panic("todo implement")
}

func getRelationKey(typ string, platform string, ids any) []string {
	if reflect.ValueOf(ids).Kind() != reflect.Array {
		return nil
	}
	prefix := typ + "_" + platform + "_"
	var uniKeys []string
	for _, id := range ids.([]any) {
		uniKeys = append(uniKeys, fmt.Sprintf("%s%v", prefix, id))
	}
	return uniKeys
}

func (s *SyncWeWorkUseCase) SyncTagsToSystem(ctx context.Context) {
	tagResult, err := s.API.UserTag.List(ctx)
	if err != nil {
		panic(errors.Wrap(err, "request tag data failed"))
	}
	wIds := slicex.SlicePluck(tagResult.TagList, func(item *request.RequestTag) int64 {
		return int64(item.TagID)
	})
	uniKeys := getRelationKey(RelationTypeTag, PlatformWeWork, wIds)
	var existRelations []*SyncRelation
	err = s.db.WithContext(ctx).Model(&SyncRelation{}).Where("platform_uni_key in ?", uniKeys).
		Find(&existRelations).Error
	if err != nil {
		panic(errors.Wrap(err, "find existed tag relation id failed"))
	}

}
