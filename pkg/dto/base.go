package dto

// pkg/dto/base.go

import (
	"fmt"
	"github.com/gin-gonic/gin/binding"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// BaseResponse 通用响应结构
type BaseResponse struct {
	Code      int         `json:"code" example:"200" description:"状态码"`
	Message   string      `json:"message" example:"success" description:"响应消息"`
	Data      interface{} `json:"data,omitempty" description:"响应数据"`
	Timestamp int64       `json:"timestamp" example:"1699171845" description:"时间戳"`
	RequestID string      `json:"request_id,omitempty" example:"req_123456" description:"请求ID"`
}

// SuccessResponse 成功响应
type SuccessResponse struct {
	Code      int         `json:"code" example:"200"`
	Message   string      `json:"message" example:"success"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Code      int                    `json:"code" example:"400"`
	Message   string                 `json:"message" example:"参数错误"`
	Error     string                 `json:"error,omitempty" example:"validation failed"`
	Details   map[string]interface{} `json:"details,omitempty" description:"错误详情"`
	Timestamp int64                  `json:"timestamp"`
	RequestID string                 `json:"request_id,omitempty"`
}

// PaginationRequest 分页请求基础结构
type PaginationRequest struct {
	Page      int    `json:"page" form:"page" validate:"omitempty,min=1" example:"1" description:"页码"`
	PageSize  int    `json:"page_size" form:"page_size" validate:"omitempty,min=1,max=100" example:"10" description:"每页数量"`
	SortBy    string `json:"sort_by" form:"sort_by" example:"created_at" description:"排序字段"`
	SortOrder string `json:"sort_order" form:"sort_order" validate:"omitempty,oneof=asc desc" example:"desc" description:"排序方向"`
}

// PaginationResponse 分页响应结构
type PaginationResponse struct {
	Total    int64 `json:"total" example:"100" description:"总数量"`
	Page     int   `json:"page" example:"1" description:"当前页码"`
	PageSize int   `json:"page_size" example:"10" description:"每页数量"`
	Pages    int   `json:"pages" example:"10" description:"总页数"`
}

// ListResponse 列表响应结构
type ListResponse struct {
	Items      interface{}         `json:"items" description:"数据列表"`
	Pagination *PaginationResponse `json:"pagination,omitempty" description:"分页信息"`
}

// 全局验证器实例
var validate *validator.Validate

// 初始化验证器
func init() {
	validate = validator.New()
	// 支持字母、数字、短横线、中划线
	err := validate.RegisterValidation("alphanumdash", func(fl validator.FieldLevel) bool {
		value := fl.Field().String()
		matched, _ := regexp.MatchString("^[a-zA-Z0-9_-]+$", value)
		return matched
	})
	if err != nil {
		panic(err)
	}
}

// ValidateRequest 验证请求结构体
func ValidateRequest(req interface{}) error {
	return validate.Struct(req)
}

// ValidateRequestWithContext 验证请求结构体（支持 context）
func ValidateRequestWithContext(c *gin.Context, req interface{}) error {
	// 先尝试 path 变量: /agents/:agent_id/status
	_ = c.ShouldBindUri(req) // 有就绑；没有也不报错（忽略）

	var err error
	switch c.Request.Method {
	case http.MethodGet, http.MethodDelete:
		err = c.ShouldBindQuery(req)
	default:
		// 根据 Content-Type 决定
		ct := c.ContentType()
		switch {
		case strings.HasPrefix(ct, binding.MIMEJSON):
			err = c.ShouldBindJSON(req)
		case strings.HasPrefix(ct, binding.MIMEMultipartPOSTForm),
			strings.HasPrefix(ct, binding.MIMEPOSTForm),
			ct == "":
			// form 或未声明 Content-Type 时回退到通用绑定（会走 form/query）
			err = c.ShouldBind(req)
		default:
			// 兜底
			err = c.ShouldBind(req)
		}
	}

	if err != nil {
		return fmt.Errorf("参数绑定失败: %w", err)
	}
	if err := ValidateRequest(req); err != nil {
		return fmt.Errorf("参数验证失败: %w", err)
	}
	return nil
}

// SetDefaultPagination 设置分页默认值
func (p *PaginationRequest) SetDefaultPagination() {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 10
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	if p.SortOrder == "" {
		p.SortOrder = "desc"
	}
	if p.SortBy == "" {
		p.SortBy = "created_at"
	}
}

// CalculatePages 计算总页数
func (p *PaginationResponse) CalculatePages() {
	if p.PageSize > 0 {
		p.Pages = int((p.Total + int64(p.PageSize) - 1) / int64(p.PageSize))
	}
}

// ResponseSuccess 返回成功响应
func ResponseSuccess(c *gin.Context, data interface{}) {
	response := SuccessResponse{
		Code:      http.StatusOK,
		Message:   "success",
		Data:      data,
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}
	c.JSON(http.StatusOK, response)
}

// ResponseError 返回错误响应
func ResponseError(c *gin.Context, code int, message string, err error) {
	response := ErrorResponse{
		Code:      code,
		Message:   message,
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}

	if err != nil {
		response.Error = err.Error()
	}

	c.JSON(code, response)
}

// ResponseValidationError 返回验证错误响应
func ResponseValidationError(c *gin.Context, err error) {
	details := make(map[string]interface{})

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, fieldError := range validationErrors {
			details[fieldError.Field()] = getValidationErrorMessage(fieldError)
		}
	}

	response := ErrorResponse{
		Code:      http.StatusBadRequest,
		Message:   "参数验证失败",
		Error:     err.Error(),
		Details:   details,
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}

	c.JSON(http.StatusBadRequest, response)
}

// ResponseList 返回列表响应
func ResponseList(c *gin.Context, items interface{}, pagination *PaginationResponse) {
	if pagination != nil {
		pagination.CalculatePages()
	}

	response := SuccessResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data: ListResponse{
			Items:      items,
			Pagination: pagination,
		},
		Timestamp: time.Now().Unix(),
		RequestID: getRequestID(c),
	}

	c.JSON(http.StatusOK, response)
}

// getRequestID 获取请求ID
func getRequestID(c *gin.Context) string {
	if requestID := c.GetHeader("X-Request-ID"); requestID != "" {
		return requestID
	}
	if requestID := c.GetString("request_id"); requestID != "" {
		return requestID
	}
	return ""
}

// getValidationErrorMessage 获取验证错误消息
func getValidationErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "此字段为必填项"
	case "min":
		return fmt.Sprintf("最小值为 %s", fe.Param())
	case "max":
		return fmt.Sprintf("最大值为 %s", fe.Param())
	case "email":
		return "请输入有效的邮箱地址"
	case "oneof":
		return fmt.Sprintf("值必须是以下之一: %s", fe.Param())
	default:
		return fmt.Sprintf("验证失败: %s", fe.Tag())
	}
}
