package errorx

/**
 * 基础错误, 推荐在Logic层处理掉, __前缀代表私有
 */

var ErrRecordNotFound = NewError(400, "__RECORD_NOT_FOUND", "记录未找到")

/**
 * 业务错误, 推荐在Logic层处理掉
 */

var ErrUnKnow = NewError(500, "UN_KNOW", "未知错误, 请联系开发团队")
var ErrBadRequest = NewError(400, "BAD_REQUEST", "违规请求")

var ErrUnAuthorization = NewError(401, "UN_AUTHORIZATION", "未授权")
var ErrPhoneUnAuthorization = NewError(401, "UN_PHONE_AUTHORIZATION", "用户需要先授权登录")
