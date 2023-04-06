## 简单开发指南
### 快速开始
1. 在api目录中创建一个 greet.api 文件, 在文件中声明新的接口, 关于api文件的语法详见[Api语法介绍](https://go-zero.dev/cn/docs/design/grammar)
    ```
    // api版本(可省略)
    syntax = "v1"
    
    // api概述(可省略)
    info(
        title: "type title here"
        desc: "type desc here"
        author: "type author here"
        email: "type email here"
        version: "type version here"
    )
    
    // api分组, 声明group后, 生成的handle和logic将会被划分到internal/(handle/logic)/greet目录中
    @server(
        group: greet
    )
    
    // 声明service, 由于本项目是单体服务, 因此所有api的Service声明都是PowerX
    service PowerX {
        // api的描述
        @doc "greet"
        // 生成的handle以及logic的名字
        @handler Hello
        // api接口
        post /api/greet/v1/hello (HelloRequest) returns (HelloReply)
    }
    
    // type定义, 将会生成到internal/types目录
    type HelloRequest {
        Msg string `json:"msg,optional"`
    }
    
    // type定义, 将会生成到internal/types目录
    type HelloReply {
        Msg string `json:"msg"`
    }
    ```
2. 使用goctl指令自动生成golang代码, 如果未安装goctl, 参见[goctl安装](https://go-zero.dev/cn/docs/goctl/goctl)进行安装
    ```
    goctl api go -api ./api/powerx.api -dir .
    ```
3. (可选) 编写数据对象UseCase方法, 在internal/uc中创建greet.go, 编写完成后在uc/powerx.go中的NewPowerXUseCase调用newGreetUseCase(...)方法将实例注入到PowerXUseCase实例中, PowerXUseCase在svc目录中注入到了ServiceContext中  
    [uc/greet.go]()
   ```
    type GreetUseCase struct {
        // dependence...
    }
    
    func newGreetUseCase(depParams...) *GreetUseCase {
        return &GreetUseCase{
            depParams...
        }
    }
    
    // 数据对象的用例方法封装data层方法和基础数据逻辑(本项目中省略了持久层(data层)), 供logic层调用, 持久层(data层)对于logic来说应该是无感的
    func (uc *GreetUseCase) SayHello() string {
        // db operation
        return "Hello!!!"
    }
    ```
    [powerx.go](internal%2Fuc%2Fpowerx.go)
    ```
    type PowerXUseCase struct {
        ...
        Greet *GreetUseCase
        ...
    }
    
    
    
    func NewPowerXUseCase(...) {
    ...
    uc.Greet = newGreetUseCase(...)
    ...
    }
    ```
4. 编写logic代码, 在步骤2执行后, 会在handle以及logic目录/子目录下生成新的go文件, 以步骤2为例, 生成的文件为internal/handler/greet/hellohandler.go和internal/logic/greet/hellologic.go, 其中hellohandler.go目录不需要管, 我们只需要在hellologic.go中的todo注释位置编写业务代码
   ```
   ...
   // todo: add your logic here and delete this line
   ...
   ```
5. 启动服务
   以Goland（推荐，我们团队统一使用Goland作为IDE开发）为例
   在IDE里，配置设置工作目录：/{your-workspace}/{org}/{PowerX}/，这样启动项目的时候，以你项目的根目录为基准路径
   
   