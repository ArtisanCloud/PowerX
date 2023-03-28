## PowerX

### 开发
代码生成指令
```
goctl api go -api ./api/powerx.api -dir .
```
typescript
```
goctl api ts -api ./api/powerx.api -dir ./api/ts -unwrap -webapi ./api
```
doc
```
goctl api doc --dir ./api --o ./api/doc
```


```
// install swagger plugin
go install github.com/zeromicro/goctl-swagger@latest

// generate swagger file
goctl api plugin -plugin goctl-swagger="swagger -filename auth.json" -api auth.api -dir .
```