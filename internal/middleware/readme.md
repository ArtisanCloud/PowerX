## Middleware 中间件

#### [auth.go](auth.go)
实现了简单JWT Token解析和校验, 并使用Casbin验证访问权限, 当前Casbin策略以文件形式加载(见[casbin.go](..%2Fuc%2Fcasbin.go))