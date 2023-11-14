package errorx

/**
 * 基础错误, 推荐在Logic层处理掉, __前缀代表私有
 */

var ErrRecordNotFound = NewError(400, "RECORD_NOT_FOUND", "记录未找到")

/**
 * 业务错误, 推荐在Logic层处理掉
 */

var ErrUnKnow = NewError(500, "UN_KNOW", "未知错误, 请联系开发团队")
var ErrBadRequest = NewError(400, "BAD_REQUEST", "违规请求")

var ErrUnAuthorization = NewError(401, "UN_AUTHORIZATION", "未授权")
var ErrPhoneUnAuthorization = NewError(401, "UN_PHONE_AUTHORIZATION", "用户需要先授权登录")

var ErrNotFoundObject = NewError(400, "OBJECT_NOT_FOUND", "对象未找到")
var ErrCreateObject = NewError(400, "OBJECT_CREATE", "创建对象失败")
var ErrUpdateObject = NewError(400, "OBJECT_UPDATE", "更新对象失败")
var ErrDuplicatedInsert = NewError(400, "OBJECT_DUPLICATED_INSERT", "有关键字段不能重复插入")
var ErrDeleteObject = NewError(400, "OBJECT_DELETE", "删除对象失败")
var ErrDeleteObjectNotFound = NewError(400, "OBJECT_NOT_FOUND", "未找到删除对象")
var ErrNotFoundStandardPriceBook = NewError(400, "STANDARD_PRICE_BOOK_NOT_FOUND", "标准价格手册未找到")
var ErrOneStandardPriceBookOnly = NewError(400, "STANDARD_PRICE_BOOK_ONLY_ONE", "标准价格手册只能有一本")
var ErrCanNotDeleteStandardPrice = NewError(400, "CAN_NOT_DELETE_STANDARD_PRICE_BOOK", "不能删除标准价格手册")
