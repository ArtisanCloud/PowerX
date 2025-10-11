**插件不需要把源码复制进 PowerX 仓库**。正确做法是：插件独立仓库开发→本地构建出“可分发产物（dist）”→通过 PowerX 的安装接口把**产物目录**安装进去。下面给你一套落地的目录、构建与安装流程。

---

# 一、插件独立仓库目录建议

```
my-hello-plugin/
├── plugin.yaml                  # 清单（必须，含 id/version/entry/前端目录/菜单/RBAC/事件等）
├── backend/
│   ├── cmd/hello/main.go        # 你的服务入口（读取 PORT 环境变量）
│   └── go.mod                   # 独立 go module（建议）
├── frontend/
│   └── admin/                   # 构建后的静态资源（或 admin-src/ 存源码，构建到这里）
├── migrations/                  # 可选：数据迁移脚本
├── contracts/
│   ├── openapi.yaml             # 可选：HTTP 合同
│   └── proto/                   # 可选：gRPC 合同
├── public/                      # 可选：静态公共资源
├── Makefile                     # 一键构建/打包
├── README.md
└── dist/
    └── 0.1.1/                   # 构建后产物（只放运行需要的文件，不放源码）
        ├── plugin.yaml
        ├── backend/
        │   └── bin/hello        # 可执行文件（给 PowerX 运行）
        ├── frontend/
        │   └── admin/...
        ├── migrations/...
        ├── contracts/...
        └── public/...
```

> 关键点：**PowerX 只需要 `dist/<version>` 目录**，源码（`backend/cmd`、`admin-src` 等）都留在插件仓库，**不进入 PowerX**。

---

# 二、`plugin.yaml`（与当前内核对齐）

```yaml
id: com.powerx.demo.hello_world
version: 0.1.1
name: Hello World
description: "Demo plugin"
corex_version: ">=0.1.0"

runtime:
  kind: process
  entry: ./backend/bin/hello          # 相对 dist/<version> 的路径
  args: []
  env:
    LOG_LEVEL: info
  health:
    http: /healthz
    interval: 2s
    timeout: 1s

backend:
  entry: ./backend/bin/hello
  port: 8091
  health: /healthz

endpoints:
  http_base_path: /v1
  grpc:
    enabled: false
    proto_dir: ""

routes:
  basePath: /v1
  adminManifest: /api/v1/admin/manifest
  rbac: /api/v1/admin/rbac

frontend:
  admin:
    kind: static
    static_dir: ./frontend/admin       # 相对 dist/<version>
    proxy_base_path: ""                # 为空则走默认 /_p/:id/api 代理
    menus:
      - id: "hello"
        title: Hello World
        icon: Smile
        order: 50
        path: /plugins/hello

permissions:
  - resource: hello.report
    actions: [view]

rbac:
  resources:
    - resource: hello.report
      actions: [view]

menus:
  - id: "hello"
    title: Hello World
    icon: Smile
    path: /plugins/hello
    order: 50
    children: []

agents:
  - id: "hello.assistant"
    plugin_id: "com.powerx.demo.hello_world"
    name: "Hello 助理"
    description: "演示插件的智能助手"
    default_tools: ["hello.ping"]

tools:
  - id: "hello.ping"
    plugin_id: "com.powerx.demo.hello_world"
    name: "Ping API"
    description: "调用 /api/v1/ping"
    transport: "http"
    endpoint: "/api/v1/ping"
    method: "GET"
    rbac_resource: "hello.report"
    input_schema:
      type: object
      properties: {}
    output_schema:
      type: object
      properties:
        pong:
          type: boolean

events:
  publish:   ["hello.processed"]
  subscribe: ["customer.created"]

assets:
  public_dir: ./public
  webAdminPath: web-admin/.output
```

> 字段要点
> - `backend`：声明子进程的入口、监听端口与健康探针，便于宿主在安装阶段做预检查。
> - `routes`：告知宿主可用的 API 前缀、提供管理端清单与 RBAC 数据的接口地址。
> - `permissions`：插件自带的 RBAC 能力声明，通常与 `rbac.resources` 对应，可由宿主同步写入权限系统。
> - `menus`：提供前端菜单树（支持多级），与 `frontend.admin.menus` 一致，但可读写 JSON 结构化数据。
> - `agents` / `tools`：注册插件可用的智能体与工具，宿主在安装后可据此生成配置或入口。
> - `assets.webAdminPath`：指定打包后的前端 Admin 目录，宿主可直接挂载静态资源。

---

# 三、最小后端入口（独立仓库里）

```go
// backend/cmd/hello/main.go
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8077" // 本地调试缺省
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pong": true}`))
	})
	log.Printf("[hello] listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

> 运行时 **Manager/Supervisor 会注入 PORT**；本地自测可手动 `PORT=9999 ./backend/bin/hello`。

---

# 四、Makefile（把源码“编译+拷贝”到 dist/<version>）

```makefile
# ----- 配置 -----
PLUGIN_ID := com.powerx.demo.hello_world
VERSION   := 0.1.1
DIST_DIR  := dist/$(VERSION)

GOOS      ?= $(shell go env GOOS)
GOARCH    ?= $(shell go env GOARCH)
BIN_NAME  := hello

# ----- 入口 -----
.PHONY: all build pack clean
all: build

build: clean prepare build-backend copy-assets

prepare:
	mkdir -p $(DIST_DIR)/backend/bin
	mkdir -p $(DIST_DIR)/frontend/admin
	mkdir -p $(DIST_DIR)/migrations
	mkdir -p $(DIST_DIR)/contracts/proto
	mkdir -p $(DIST_DIR)/public

# 编译后端
build-backend:
	cd backend && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ../$(DIST_DIR)/backend/bin/$(BIN_NAME) ./cmd/hello

# 拷贝清单与静态资源（注意：只复制“运行需要”的产物）
copy-assets:
	cp plugin.yaml $(DIST_DIR)/
	# 如果你的前端要构建，请先在 admin-src 里执行 npm run build 输出到 frontend/admin
	cp -R frontend/admin/* $(DIST_DIR)/frontend/admin/ || true
	# 可选
	@test ! -d migrations || cp -R migrations/* $(DIST_DIR)/migrations/
	@test ! -f contracts/openapi.yaml || cp contracts/openapi.yaml $(DIST_DIR)/contracts/openapi.yaml
	@test ! -d contracts/proto || cp -R contracts/proto/* $(DIST_DIR)/contracts/proto/
	@test ! -d public || cp -R public/* $(DIST_DIR)/public/

pack: build
	cd dist && zip -r $(PLUGIN_ID)-$(VERSION).zip $(VERSION)

clean:
	rm -rf dist
```

> 这样 `make build` 后，**可分发产物**就在 `dist/0.1.1/`，里面只有运行需要的文件，没有源码。

---

# 五、安装到 PowerX（不放源码！）

有两种常见方式：

### 方式 A：直接用我们已经实现的接口（推荐）

```bash
# 假设 PowerX 在本机 8077 端口
curl -X POST http://localhost:8077/api/admin/plugins/install/local \
  -H 'Content-Type: application/json' \
  -d '{"src_dir":"/ABS/PATH/TO/my-hello-plugin/dist/0.1.1","enable":true}'
```

* `src_dir` 指向 **dist/<version> 目录**；
* `enable=true` 会装完立刻启用（我们之前用 `VerifyChecksum` 临时当开关的话，你已换成更语义化的 `EnableAfterInstall` 也行）；
* 如果只是先装不启用，`enable=false`，再调用：

  ```bash
  curl -X POST http://localhost:8077/api/admin/plugins/com.powerx.demo.hello_world/switch_version \
    -H 'Content-Type: application/json' \
    -d '{"version":"0.1.1","enable":true}'
  ```

### 方式 B：未来的“远程上架”

* CI 产出 `zip` + `sha256`（和可选签名）后，上传到对象存储或插件市场；
* PowerX 管理端调用 `InstallFromURL(url, sha256, signature, opts)` 下载校验后解压安装；
* 这块我们之后加好 `InstallFromURL` 支持就能用。

---

# 六、本地开发循环（无需每次安装）

* 后端：在插件仓库里 `PORT=9999 go run ./backend/cmd/hello`；
* 前端：如果是 SPA，开发时开 dev server；**产物**最终要构建到 `frontend/admin/`；
* 验证与 PowerX 反代联动时，要通过 `/_p/<id>/api/v1/ping` 和 `/_p/<id>/admin/` 打通；
* 真正“装入 PowerX”只在**发布/联调**阶段做一次（或版本迭代时做）。

---

# 七、版本与升级

* 遵循 semver：`0.1.1 → 0.2.0 → 1.0.0`；
* 每个版本都有独立目录 `dist/<version>`；
* `install/local` 先把版本登记为 `installed`；
* `switch_version` 切 `current` 并启用新版本；
* 旧版本可通过 `Uninstall(id, version)` 卸载（我们下一步就给你实现卸载 API）。

---

# 八、Checklist（避免踩坑）

* [ ] `plugin.yaml` 的 `runtime.entry`、`frontend.admin.static_dir` **相对 dist/<version>**；
* [ ] 后端二进制有执行权限（`-rwx`）；
* [ ] 读取 `PORT` 环境变量；
* [ ] 前端一定要把**构建产物**放到 `frontend/admin/`；Nuxt/Spa 请提前确认构建期 `app.baseURL`，详见《[Nuxt 插件 Admin 前端 baseURL 排查指南](./plugins/frontend_admin_baseurl.md)》；
* [ ] 不要把 `node_modules/`、`.git/`、源码拷进 `dist/`；
* [ ] PowerX 的 `plugins/` 目录建议 gitignore（避免把安装产物提交仓库）。

---

这样，你就可以把插件当成**独立产品**来开发、发版和分发：
**独立仓库开发 → Make 打包到 `dist/<ver>` → PowerX 调用安装接口 → 可用。**

如果 OK，我们下一步就实现 **Uninstall**（支持卸载当前/指定版本，带可选 `purge` 删除文件）。
