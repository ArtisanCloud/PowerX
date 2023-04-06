# errorx包设计说明及使用指南

`errorx`包提供了一种自定义的错误处理方式，方便在PowerX项目中对错误进行统一管理。以下是该包的详细设计说明和使用指南。

## 设计说明

`errorx`包中主要定义了两个结构体：`Error`和`ResponseErr`。

- `Error`结构体包含如下字段：
    - `StatusCode`：错误状态码，例如400、401等；
    - `Reason`：错误原因，简短地描述错误类型；
    - `Msg`：错误信息，具体描述错误的内容。
- `ResponseErr`结构体为GoZero声明, 用于包装为请求的错误响应：
    - `Reason`：与`Error`结构体中的`Reason`字段相对应；
    - `Msg`：与`Error`结构体中的`Msg`字段相对应。

`errorx`包还定义了一些函数和预定义错误：
- `NewError()`函数用于创建一个新的`Error`实例；
- `Error`结构体实现了`Error()`方法，用于返回错误信息(枚举Reason值)；
- `WithCause()`函数用于在已有错误的基础上为Msg拼接错误原因, 这将会返回一个新的错误；
- `Data()`方法用于返回一个`ResponseErr`结构体实例, 为GoZero提供；

- 预定义了一些常见错误，例如`ErrUnKnow`、`ErrBadRequest`等。

## 使用指南

### 创建自定义错误

使用`NewError()`函数创建一个新的自定义错误：

```
var ErrCustomError = errorx.NewError(400, "CUSTOM_ERROR", "这是一个自定义错误")

// 如果希望仅在逻辑中使用该错误, 而不会返回用作API错误, 请在Reason前加上"__"
var ErrCustomError = errorx.NewError(400, "__CUSTOM_ERROR", "这是一个自定义错误")
```

### 抛出自定义错误

在需要抛出错误的地方，直接返回预定义的错误：

`return errorx.ErrCustomError`

###  在错误中添加新的原因

使用`WithCause()`函数在已有错误的基础上添加新的错误原因：

`err = errorx.WithCause(err, "新的错误原因")`

## 使用示例
```
/**
比较完整的Error例子
*/

// ErrUserNotFound (errorx.go中定义)
var ErrUserNotFound = errorx.NewError(400, "USER_NOT_FOUND", "用户不存在")

type ExampleUseCase struct{}

func (e ExampleUseCase) Example() error {
// if user not found
return ErrUserNotFound
}

type ExampleLogic struct {
ExampleUseCase
}

func (e ExampleLogic) Example() error {
err := e.ExampleUseCase.Example()
if errors.Is(err, ErrUserNotFound) {
// 这里可以直接返回error -> reason: "USER_NOT_FOUND"; msg : "用户不存在"
return err

// 或者对error进行包装 -> reason: "USER_NOT_FOUND"; msg : "用户不存在: 请检查参数是否正确"
// return errorx.WithCause(err, "请检查参数是否正确")

// 或者返回其他error -> reason: "OTHER"; msg : "Other"
// return errorx.NewError(400, "OTHER", "Other")
}
return nil
}
```