package sendChatMsg

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	request2 "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
)

type ParaCreateSendChatMsg struct {
	ByAllEmployees    *bool                  `form:"byAllEmployees" json:"byAllEmployees" binding:"required"`
	Senders           []string               `form:"senders" json:"senders" binding:"required"`
	ToFilterCustomers *bool                  `form:"toFilterCustomers" json:"toFilterCustomers" binding:"required"`
	FilterGender      *int8                  `form:"filterGender" json:"filterGender"`
	FilterChatIDs     []string               `form:"filterChatIDs" json:"filterChatIDs"`
	FilterStartDate   string                 `form:"filterStartDate" json:"filterStartDate"`
	FilterEndDate     string                 `form:"filterEndDate" json:"filterEndDate"`
	WXTagIDs          []string               `form:"wxTagIDs" json:"wxTagIDs"`
	TagIDs            []string               `form:"tagIDs" json:"tagIDs"`
	ExcludedWXTagIDs  []string               `form:"excludedWXTagIDs" json:"excludedWXTagIDs"`
	SendImmediately   *bool                  `form:"sendImmediately" json:"sendImmediately" binding:"required"`
	SendOnTime        string                 `form:"sendOnTime" json:"sendOnTime"`
	Text              request2.TextOfMessage `form:"text" json:"text"`
	Attachments       []*object.HashMap      `form:"attachments" json:"attachments" binding:"required"`
}

func ValidateCreateSendChatMsg(context *gin.Context) {
	var form ParaCreateSendChatMsg
	apiResponse := http.NewAPIResponse(context)

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	sendChatMsg, err := convertParaToSendChatMsgForCreate(context, form)
	if err != nil {
		apiResponse.SetCode(config.API_ERR_CODE_REQUEST_PARAM_ERROR, config.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("sendChatMsg", sendChatMsg)
	context.Next()
}

func convertParaToSendChatMsgForCreate(context *gin.Context, form ParaCreateSendChatMsg) (sendChatMsg *models.SendChatMsg, err error) {

	// Get senders
	if *form.ByAllEmployees {
		serviceEmployee := service.NewEmployeeService(context)
		conditions := &map[string]interface{}{
			"wx_status": wx.WX_EMPLOYEE_STATUS_ACTIVE,
		}
		employees, err := serviceEmployee.GetAllEmployees(globalDatabase.G_DBConnection, conditions)
		if err != nil {
			return nil, err
		}
		if len(employees) == 0 {
			return nil, errors.New("No employee found")
		}

		form.Senders = serviceEmployee.Employee.GetEmployeeUserIDsFromEmployees(employees)
	}
	senders, err := object.JsonEncode(form.Senders)
	if err != nil {
		return nil, err
	}

	// Get customer filters
	filterChatIDs, err := object.JsonEncode(form.FilterChatIDs)
	if err != nil {
		return nil, err
	}
	wxTagIDs, err := object.JsonEncode(form.WXTagIDs)
	if err != nil {
		return nil, err
	}
	tagIDs, err := object.JsonEncode(form.TagIDs)
	if err != nil {
		return nil, err
	}
	excludedWXTagIDs, err := object.JsonEncode(form.ExcludedWXTagIDs)
	if err != nil {
		return nil, err
	}

	filterStartDate := carbon.NewCarbon().Time
	filterEndDate := carbon.NewCarbon().Time
	if *form.ToFilterCustomers {
		filterStartDate = carbon.Parse(form.FilterStartDate).Carbon2Time()
		filterEndDate = carbon.Parse(form.FilterEndDate).Carbon2Time()
	}

	sendOnTime := carbon.Now().Time
	if !*form.SendImmediately {
		sendOnTime = carbon.Parse(form.SendOnTime).Carbon2Time()
	}

	sendChatMsg = &models.SendChatMsg{
		PowerModel:      database.NewPowerModel(),
		ByAllEmployees:  *form.ByAllEmployees,
		Senders:         datatypes.JSON([]byte(senders)),
		SendImmediately: *form.SendImmediately,
		SendOnTime:      sendOnTime,
		FilterCustomers: &models.FilterCustomers{
			ToFilterCustomers:      *form.ToFilterCustomers,
			FilterGender:           *form.FilterGender,
			FilterChatIDs:          datatypes.JSON([]byte(filterChatIDs)),
			FilterStartDate:        filterStartDate,
			FilterEndDate:          filterEndDate,
			FilterWXTagIDs:         datatypes.JSON([]byte(wxTagIDs)),
			FilterTagIDs:           datatypes.JSON([]byte(tagIDs)),
			FilterExcludedWXTagIDs: datatypes.JSON([]byte(excludedWXTagIDs)),
		},
	}

	serviceAccount := service.NewCustomerService(context)
	attachments, _ := object.JsonEncode(form.Attachments)
	text, _ := object.JsonEncode(form.Text)
	for _, sender := range form.Senders {

		// Get customers
		customerUserIDs, err := serviceAccount.GetCustomerIDsByFilters(sender, sendChatMsg.FilterCustomers)
		if err != nil {
			return nil, err
		}
		if len(customerUserIDs) == 0 {
			continue
		}
		//fmt.Dump(customerUserIDs)

		externalUserIDs, _ := object.JsonEncode(customerUserIDs)

		messageTemplate := &wx.WXMessageTemplate{
			PowerCompactModel: database.NewPowerCompactModel(),
			SendChatMsgUUID:   sendChatMsg.GetForeignReferValue(),
			ChatType:          "single",
			ExternalUserIDs:   datatypes.JSON([]byte(externalUserIDs)),
			Sender:            sender,
			Text:              datatypes.JSON([]byte(text)),
			Attachments:       datatypes.JSON([]byte(attachments)),
			Creator:           service.GetAuthEmployee(context).WXUserID.String,
		}
		messageTemplate.UniqueID = messageTemplate.GetComposedUniqueID()
		sendChatMsg.WXMessageTemplates = append(sendChatMsg.WXMessageTemplates, messageTemplate)
	}

	return sendChatMsg, nil
}
