package service

import (
	"errors"
	databasePoweLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	modelSocialite "github.com/ArtisanCloud/PowerSocialite/v2/src/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type CustomerService struct {
	Service  *Service
	Customer *models.Customer
}

/**
 ** 初始化构造函数
 */
func NewCustomerService(ctx *gin.Context) (r *CustomerService) {
	r = &CustomerService{
		Service:  NewService(ctx),
		Customer: models.NewCustomer(nil),
	}
	return r
}

func (srv *CustomerService) GetList(db *gorm.DB, conditions *map[string]interface{}, page int, pageSize int) (paginator *databasePoweLib.Pagination, err error) {

	arrayCustomers := []*models.Customer{}

	preloads := []string{"PivotEmployees.PivotWXTags"}
	//preloads := []string{}

	pagination, err := databasePoweLib.GetList(db, conditions, &arrayCustomers, preloads, page, pageSize)

	return pagination, err
}

func (srv *CustomerService) UpsertCustomerByWXCustomer(db *gorm.DB, customer *modelWX.WXCustomer) (err error) {
	err = srv.UpsertCustomers(db, models.ACCOUNT_UNIQUE_ID, []*models.Customer{
		&models.Customer{
			PowerModel: databasePoweLib.NewPowerModel(),
			WXCustomer: &modelWX.WXCustomer{
				Name:            customer.Name,
				Mobile:          customer.Mobile,
				Position:        customer.Position,
				Avatar:          customer.Avatar,
				CorpName:        customer.CorpName,
				CorpFullName:    customer.CorpFullName,
				ExternalProfile: customer.ExternalProfile,
				Gender:          customer.Gender,
				ExternalUserID:  customer.ExternalUserID,
			},
		},
	}, []string{
		"name",
		"mobile",
		"position",
		"avatar",
		"corp_name",
		"corp_full_name",
		"external_profile",
		"gender",
	})

	return err
}

func (srv *CustomerService) UpsertCustomers(db *gorm.DB, uniqueName string, customers []*models.Customer, fieldsToUpdate []string) error {

	if len(customers) <= 0 {
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = databasePoweLib.GetModelFields(&models.Customer{})
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).Create(&customers)

	return result.Error
}

func (srv *CustomerService) UpsertCustomer(db *gorm.DB, customer *models.Customer, withAssociation bool) (savedCustomer *models.Customer, err error) {

	customer.UpdatedAt = time.Now()
	if customer.UUID == "" {
		customer.UUID = uuid.NewString()
		customer.CreatedAt = time.Now()
		savedCustomer, err = srv.SaveCustomer(db, customer)
	} else {
		savedCustomer, err = srv.UpdateCustomer(db, customer, withAssociation)
	}

	return savedCustomer, err
}

func (srv *CustomerService) SaveCustomer(db *gorm.DB, customer *models.Customer) (*models.Customer, error) {

	db = db.Create(customer)

	return customer, db.Error
}

func (srv *CustomerService) UpdateCustomer(db *gorm.DB, customer *models.Customer, withAssociation bool) (*models.Customer, error) {

	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(customer)

	return customer, db.Error
}

func (srv *CustomerService) DeleteCustomers(db *gorm.DB, customer []*models.Customer) error {

	db = db.Delete(customer)

	return db.Error
}

func (srv *CustomerService) DeleteCustomer(db *gorm.DB, customer *models.Customer) error {

	db = db.Delete(customer)

	return db.Error
}

func (srv *CustomerService) GetCustomers(db *gorm.DB, uuids []string) (customers []*models.Customer, err error) {

	customers = []*models.Customer{}

	db = db.Where("uuid in (?)", uuids)
	result := db.Find(&customers)
	return customers, result.Error
}

func (srv *CustomerService) GetCustomer(db *gorm.DB, uuid string) (customer *models.Customer, err error) {

	customer = &models.Customer{}

	db = db.Scopes(
		databasePoweLib.WhereUUID(uuid),
	)
	result := db.First(customer)
	return customer, result.Error
}

func (srv *CustomerService) GetCustomersByOpenIDs(db *gorm.DB, openids []string) (customers []*models.Customer, err error) {

	customers = []*models.Customer{}

	db = db.Where("open_id in (?)", openids)
	result := db.Find(&customers)
	return customers, result.Error
}

func (srv *CustomerService) GetCustomerByOpenID(db *gorm.DB, openID string) (customer *models.Customer, err error) {

	customer = &models.Customer{}

	condition := &map[string]interface{}{
		"open_id": openID,
	}
	err = databasePoweLib.GetFirst(db, condition, customer, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return customer, err
}

func (srv *CustomerService) GetCustomerIDsByExternalUserIDsAndFilters(db *gorm.DB, employeeUserID string, externalUserIDs []string, filter *models.FilterCustomers) (customerIDs []string, err error) {

	if len(externalUserIDs) <= 0 {
		return nil, errors.New("no customer ids found")
	}

	// filtered by customers' excluded tags
	filterExcludedWXTagIDs := []string{}
	err = object.JsonDecode(filter.FilterExcludedWXTagIDs, &filterExcludedWXTagIDs)
	if len(filterExcludedWXTagIDs) > 0 {
		externalUserIDs, err = srv.GetCustomerIDsFilteredByExcludedTagIDs(db, employeeUserID, externalUserIDs, filterExcludedWXTagIDs)
		if err != nil {
			return nil, err
		}
		if len(externalUserIDs) <= 0 {
			return nil, errors.New("no customer ids found after filtering by excluded tags")
		}
	}

	db = db.Model(&models.Customer{}).
		Debug().
		Where("external_user_id in (?)", externalUserIDs)

	// filtered by customers' gender
	switch filter.FilterGender {
	case models.SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_ALL:
		break
	case models.SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_MALE:
		db = db.Where("gender", modelWX.WX_CUSTOMER_GENDER_MALE)
		break
	case models.SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_FEMALE:
		db = db.Where("gender", modelWX.WX_CUSTOMER_GENDER_FEMALE)
		break
	case models.SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_UNKNOW:
		db = db.
			Where("gender!=", modelWX.WX_CUSTOMER_GENDER_MALE).
			Where("gender!=", modelWX.WX_CUSTOMER_GENDER_FEMALE)

		break
	default:

	}

	// filtered by customers' group chat
	filterChatIDs := []string{}
	err = object.JsonDecode(filter.FilterChatIDs, &filterChatIDs)
	if err != nil {
		return nil, err
	}
	if len(filterChatIDs) > 0 {
		db = db.Joins("LEFT JOIN wx_group_chat_members AS groupChatMembers ON groupChatMembers.user_id = customers.external_user_id").
			Where("GroupChatMembers.wx_group_chat_id IN (?)", filterChatIDs)
	}

	// filtered by customers' added datetime
	if !filter.FilterStartDate.IsZero() && !filter.FilterEndDate.IsZero() {
		db = db.Joins("LEFT JOIN r_customer_to_employee AS rCustomerToEmployeeCreateTime ON rCustomerToEmployeeCreateTime.customer_refer_id = customers.external_user_id").
			Where("rCustomerToEmployeeCreateTime.employee_refer_id = ?", employeeUserID).
			Where("rCustomerToEmployeeCreateTime.create_time BETWEEN ? AND ?", filter.FilterStartDate.Unix(), filter.FilterEndDate.Unix())
	}

	// filtered by customers' wx tags
	filterWXTagIDs := []string{}
	err = object.JsonDecode(filter.FilterWXTagIDs, &filterWXTagIDs)
	if len(filterWXTagIDs) > 0 {
		db = db.Joins("LEFT JOIN r_customer_to_employee AS rCustomerToEmployeeWXTag ON rCustomerToEmployeeWXTag.customer_refer_id = customers.external_user_id").
			Joins("LEFT JOIN r_wx_tag_to_object AS rWXTagToObject ON rWXTagToObject.taggable_object_id = rCustomerToEmployeeWXTag.index_customer_to_employee_id").
			Where("rCustomerToEmployeeWXTag.employee_refer_id = ?", employeeUserID).
			Where("rWXTagToObject.taggable_owner_type = ?", (&models.RCustomerToEmployee{}).GetTableName(true)).
			Where("rWXTagToObject.tag_id IN (?)", filterWXTagIDs)
	}

	// filtered by customers' tags
	filterTagIDs := []string{}
	err = object.JsonDecode(filter.FilterTagIDs, &filterTagIDs)
	if len(filterTagIDs) > 0 {
		db = db.Joins("LEFT JOIN r_tag_to_object AS rTagToObject ON rTagToObject.taggable_object_id = customers.external_user_id").
			Where("rTagToObject.taggable_owner_type = ?", (&models.Customer{}).GetTableName(true)).
			Where("rTagToObject.tag_id IN (?)", filter.FilterTagIDs)
	}

	// query customers' ids
	result := db.
		Distinct().
		Pluck("external_user_id", &customerIDs)

	return customerIDs, result.Error
}

func (srv *CustomerService) GetCustomerIDsFilteredByExcludedTagIDs(db *gorm.DB, employeeUserID string, externalUserIDs []string, filterExcludedWXTagIDs []string) (filterCustomerUserIDs []string, err error) {

	// filtered by customers' excluded tags

	filteredExcludedCustomerIDs := []string{}
	db = db.Model(&models.Customer{}).
		//Debug().
		Where("external_user_id in (?)", externalUserIDs).
		Joins("LEFT JOIN r_customer_to_employee AS rCustomerToEmployee ON rCustomerToEmployee.customer_refer_id = customers.external_user_id").
		Joins("LEFT JOIN r_wx_tag_to_object AS rWXTagToObject ON rWXTagToObject.taggable_object_id = rCustomerToEmployee.index_customer_to_employee_id").
		Where("rCustomerToEmployee.employee_refer_id = ?", employeeUserID).
		Where("rWXTagToObject.taggable_owner_type = ?", (&models.RCustomerToEmployee{}).GetTableName(true)).
		Where("rWXTagToObject.tag_id IN (?)", filterExcludedWXTagIDs).
		Distinct().Pluck("external_user_id", &filteredExcludedCustomerIDs)

	filterCustomerUserIDs = []string{}
	for _, externalUserID := range externalUserIDs {
		if !object.InArray(externalUserID, filteredExcludedCustomerIDs) {
			filterCustomerUserIDs = append(filterCustomerUserIDs, externalUserID)
		}
	}
	return filterCustomerUserIDs, err
}

func (srv *CustomerService) GetCustomersByExternalUserIDs(db *gorm.DB, externalUserIDs []string) (customers []*models.Customer, err error) {

	customers = []*models.Customer{}

	db = db.Where("external_user_id in (?)", externalUserIDs)
	result := db.Find(&customers)
	return customers, result.Error
}

func (srv *CustomerService) GetCustomerByExternalUserID(db *gorm.DB, externalUserID string) (customer *models.Customer, err error) {
	customer = &models.Customer{}

	preloads := []string{"PivotEmployees.PivotWXTags"}

	condition := &map[string]interface{}{
		"external_user_id": externalUserID,
	}
	err = databasePoweLib.GetFirst(db, condition, customer, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return customer, err

}

func (srv *CustomerService) FollowEmployee(db *gorm.DB, customer *models.Customer, followInfo *modelSocialite.FollowUser) (pivot *models.RCustomerToEmployee, err error) {

	pivot, err = (&models.RCustomerToEmployee{}).UpsertPivotByFollowUser(db, customer, followInfo)

	return pivot, err
}

func (srv *CustomerService) SyncFollowEmployee(db *gorm.DB, customer *models.Customer, followInfo *modelSocialite.FollowUser) (pivots *models.RCustomerToEmployee, err error) {

	err = srv.ClearEmployees(db, customer)
	if err != nil {
		return nil, err
	}
	return srv.FollowEmployee(db, customer, followInfo)

}

func (srv *CustomerService) UnfollowEmployee(db *gorm.DB, customer *models.Customer, employees []*models.Employee) (err error) {
	err = db.Model(customer).
		//Debug().
		Association("FollowUsers").
		Delete(employees)

	return err
}

func (srv *CustomerService) ClearEmployees(db *gorm.DB, customer *models.Customer) (err error) {
	err = db.Model(customer).
		//Debug().
		Association("FollowUsers").
		Clear()

	return err
}

func (srv *CustomerService) SyncCustomers(employeeUserIDs []string, cursor string) (err error) {

	serviceEmployee := NewEmployeeService(nil)

	if len(employeeUserIDs) <= 0 {
		employeeUserIDs, err = serviceEmployee.GetEmployeeUserIDs(global.DBConnection)
		if err != nil {
			return err
		}
	}

	// sync employee's contacts with userid from wx
	response, _ := wecom.WeComApp.App.ExternalContact.BatchGet(employeeUserIDs, cursor, 200)
	if response.ErrCode != 0 {
		return errors.New(response.ErrMSG)
	}

	// parse the result of employees from wx
	for _, contact := range response.ExternalContactList {
		// parse contacts from wx
		customer := srv.NewCustomerFromWXContact(contact.ExternalContact)
		err = srv.UpsertCustomerByWXCustomer(global.DBConnection, customer.WXCustomer)

		// sync follow user info
		employee, err := serviceEmployee.GetEmployeeByUserID(global.DBConnection, contact.FollowInfo.UserID)
		if err != nil || employee == nil {
			fmt.Dump(err.Error())
		}
		//fmt.Dump(contact.FollowInfo)

		pivot, err := (&models.RCustomerToEmployee{}).UpsertPivotByFollowUser(global.DBConnection, customer, contact.FollowInfo)
		if err != nil {
			fmt.Dump(err.Error())
		}

		// sync wx tags to employee
		if len(contact.FollowInfo.TagIDs) > 0 {
			serviceWXTag := wecom.NewWXTagService(nil)
			err = serviceWXTag.SyncWXTagsByFollowInfos(global.DBConnection, pivot, contact.FollowInfo)

		}

	}

	return err
}

func (srv *CustomerService) NewCustomerFromWXContact(contact *modelSocialite.ExternalContact) *models.Customer {

	externalUserID := object.NewNullString("", false)
	if contact.ExternalUserID != "" {
		externalUserID = object.NewNullString(contact.ExternalUserID, true)
	}

	unionID := object.NewNullString("", false)
	if contact.UnionID != "" {
		unionID = object.NewNullString(contact.UnionID, true)
	}

	customer := &models.Customer{
		PowerModel: &databasePoweLib.PowerModel{

			UUID: uuid.New().String(),
			//CreatedAt:   carbon.Now(),
			UpdatedAt: time.Now(),
		},
		WXCustomer: &modelWX.WXCustomer{
			ExternalUserID: externalUserID,
			Name:           contact.Name,
			Position:       contact.Position,
			Avatar:         contact.Avatar,
			CorpName:       contact.CorpName,
			CorpFullName:   contact.CorpFullName,
			UnionID:        unionID,
			WXType:         modelWX.WX_TYPE_WEWORK,
			Gender:         contact.Gender,
			//ExternalProfile: contract.ExternalProfile,
		},
	}

	return customer
}

// -------------------------------------------------------------------------------
func (srv *CustomerService) GetCustomerIDsByFilters(employeeUserID string, filter *models.FilterCustomers) (customerUserIDs []string, err error) {
	customerUserIDs = []string{}
	mdl := &models.RCustomerToEmployee{}
	pivots, err := mdl.GetPivotsByEmployeeUserID(global.DBConnection, employeeUserID)
	if err != nil {
		return nil, err
	}

	customerUserIDs = mdl.ConvertCustomerUserIDs(pivots)

	if filter.ToFilterCustomers {
		customerUserIDs, err = srv.GetCustomerIDsByExternalUserIDsAndFilters(global.DBConnection, employeeUserID, customerUserIDs, filter)
		if err != nil {
			return customerUserIDs, err
		}
		if len(customerUserIDs) <= 0 {
			return nil, errors.New("no customer found")
		}
	}

	return customerUserIDs, nil
}
