# 接口设计规范 

## 路由设计规范 


```
建议路由使用Restful的方式来缩短URI
```

### 请求方法 

| 请求方法   | 含义          |
|:-------|:------------|
| Get    | 获取数据对象      |
| Post   | 创建数据对象/执行动作 |
| Put    | 完全替换数据对象    |
| Patch  | 更新数据对象的指定属性 |
| Delete | 删除数据对象      |

### URI 

- 路径中非参数的部分, 使用 `-` 连缀 


```
/path/my-path
```

- 建议以对象层级的方式进行描述 


```
GET: /users/:userId  //users指的是所有用户,userId是一个路径参数, 该uri的语义就是获取所有用户中userId=xxx的这个用户的数据对象
```

- 建议URI统一加前缀以区分服务, 后面可以加版本号区分接口版本 


```
/user-service/......
/user-service/v1/......
```

- 对于BFF的API接口, 建议加上 `/api` 前缀, 与Http RPC做出区分
```
/api/xxx-service/v1/......
```

- 如果某个 api 接口其作用是执行一个动作
```
POST /api/xxx-service/v1/op/xx-action
POST /api/xxx-service/v1/op/xxx
```

### 参数 

- 对于Get请求, 使用 路径参数 + Query参数的方式 

- 对于非Get请求, 使用 路径参数 + Json Request Body 

- 请求参数使用小驼峰格式, 如 `/path?userName=xxx&userType=xxx`, 便于前后端开发 

- 路径参数用来指定具体的对象, 一般是id, 例如 `/project/:projectId/sub-projects/:subProjectId`

### 响应 

- 使用Json响应 

- HttpCode代表响应状态  

| HttpCode | 响应状态                                             |
|----------|--------------------------------------------------|
| 200      | Ok, 请求没问题都可以用这个                                  |
| 202      | Accepted, 如果是异步动作, 可以用这个表示请求已被接受                 |
| 400      | Bad Request, 错误的请求, 参数错误可以用这个表示, 没有满足Api接口的要求/校验 |
| 401      | Unauthorized, 未授权, 没有授权或者虚假的授权                   |
| 403      | Forbidden, 禁止访问的资源, 401是限制用户的可访问, 403是限制资源的可访问   |
| 404      | Not Found, 未找到请求的资源                              |
| 500      | Internal Server Error 一般该代码指代Unknow异常            |

注意: 对于前端来说, 400/401/403/500 都可以统一拦截, 其他代码则由调用者进行处理 

### 错误处理 

- 当HttpCode != 2xx时, Response Body中就应该是Error的内容, 至少要包括两个字段 reason 和 message, reason表示错误的原因(枚举Key), message则是显示或者描述该错误的字段 


```
Response.status = 404

Response.body = {
  "reason": "USER_NOT_FOUND",
  "message": "user not found, invalid userId"
}
```
