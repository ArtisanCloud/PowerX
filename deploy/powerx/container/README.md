# PowerX：只下载镜像的 Docker 部署

本指南面向第一次部署 PowerX 的用户，不需要 Go、Node、源码编译或宿主机安装 PostgreSQL/Redis。宿主机只需 Docker Engine、Compose 插件、POSIX shell；HTTPS 可交给已有入口服务或 XDocker。开发镜像以远程 develop 分支已提交代码为源码，并固定完整提交 SHA，不包含其他工作区未提交内容。

## 两种部署方式

| 方式 | 适合谁 | 入口与证书 |
| --- | --- | --- |
| 本指南的独立部署包 | 只部署 PowerX，或已有自己的反向代理 | 内置可选 Nginx HTTP 入口；HTTPS 接已有代理 |
| XDocker | 同时管理网站、证书、FRP、多套应用 | 共享 Nginx/Certbot，实例数据库独立 |

XDocker 的具体步骤见其 `docs/guides/powerx/README.md`。两种方式使用同一份前后端镜像，服务器不现场编译。首次部署先使用开发环境，验证后再准备正式环境的数据迁移和备份。

## 1. 准备 Docker

```bash
docker version
docker compose version
```

需要同时看到 Docker Client 和 Server。Ubuntu 用户首次加入 docker 组后须重新登录；尚未配置组权限时，在 Docker 命令前加 `sudo`。macOS 使用 Docker Desktop 并先启动 Docker 引擎。

检查镜像仓库网络：

```bash
curl --connect-timeout 10 --max-time 20 https://ghcr.io/v2/
curl --connect-timeout 10 --max-time 20 https://registry-1.docker.io/v2/
```

HTTP 401 表示网络已连接但需要仓库认证；它不证明完整镜像下载成功。实际用 `docker pull` 验证。不要把 Github HTTPS 下载代理当作 Docker registry 或 SSH 代理。

## 2. 获取部署包

从 PowerX GitHub Releases 选择已验证的 `docker-dev-<提交前12位>` 预发布，下载：

- `powerx-docker.tar.gz`
- `powerx-docker.tar.gz.sha256`

校验 SHA256，再解压。包只包含 Compose、配置示例、启动脚本与本指南，不包含源码、数据库或任何私有凭据。

```bash
sha256sum -c powerx-docker.tar.gz.sha256
tar -xzf powerx-docker.tar.gz
cd powerx-docker
cp .env.example .env
```

macOS 可使用 `shasum -a 256 powerx-docker.tar.gz` 与 `.sha256` 文件比对。下载前确认对应镜像发布工作流完整成功，不能仅根据构建步骤成功判断可部署。

## 3. 编辑公开运行配置

发布包的 `.env.example` 已绑定同一源码 SHA 的前后端镜像。保留这两个固定版本，先编辑：

```dotenv
COMPOSE_PROJECT_NAME=powerx-dev
DEPLOYMENT_ENV=dev
PUBLIC_ORIGIN=http://127.0.0.1:18080
PUBLIC_WS_ORIGIN=ws://127.0.0.1:18080
HTTP_BIND=127.0.0.1
HTTP_PORT=18080
ADMIN_USERNAME=admin
ADMIN_EMAIL=you@example.com
```

`PUBLIC_ORIGIN` 是**浏览器最终访问地址**，不是容器内部服务名。`PUBLIC_WS_ORIGIN` 使用同一主机与端口，HTTP 对应 ws，HTTPS 对应 wss。管理员账号和密码由你在首次 /setup 页面填写，启动脚本不会创建管理员，也不会使用共享默认密码。

内置 Nginx 默认只绑定回环地址。服务器上测试时，通过 SSH 转发 18080；若需要直接开放 HTTP，明确改为 `HTTP_BIND=0.0.0.0` 并放通对应端口。生产入口建议 HTTPS，密码与令牌不要通过公网明文 HTTP 传输。

### 公开或私有镜像

镜像公开时可以匿名拉取。Github 公开源码仓库不会自动使 GHCR 镜像公开，镜像包的可见性须单独配置。发布者应在组织 Packages 中把 **powerx-backend** 和 **powerx-web-admin** 设置为 Public，再用未登录环境验证。

若发布方保留 Private，则需要其授权。使用具有 `read:packages` 的 PAT classic，并确保账号对两个包有 Read 权限：

```bash
docker login ghcr.io -u YOUR_GITHUB_USERNAME
```

登录时输入 PAT，而不是 GitHub 密码。整个部署过程若使用 sudo，则使用 `sudo docker login`，保持登录与拉取的执行用户一致。不要把 PAT 写到 `.env`、Compose 或发到聊天中。

## 4. 初始化并启动

```bash
sh run.sh start
sh run.sh status
```

`start` 只生成容器基础配置和独立数据库/Redis 密码，启动数据库、缓存、后端与前端。首次状态为 `uninstalled`，数据库没有应用表，没有自动 migrate、seed 或管理员。

再次执行会保留现有配置。完成 Setup 后重启也不会回到未安装状态，不会重新生成签名/加密密钥。容器部署仍不会创建公开的本地开发 API keys。

## 5. 在 Setup 完成首次安装

打开 **http://127.0.0.1:18080/setup**；访问首页时前端也会依据未安装状态进入 Setup。

在服务器自己的终端读取 Docker 基础连接参数，不要把输出发到聊天或公开仓库：

```bash
sh run.sh setup-values
```

按页面顺序填写：

| Setup 项 | Docker 部署填写 |
| --- | --- |
| 部署环境 | `.env` 的 DEPLOYMENT_ENV，例如 dev |
| 域名 | 浏览器最终访问主机；API 与页面同源 |
| HTTPS | 由外部代理/XDocker 管理时，不在 PowerX 重复签发 |
| 本地存储 | `/data/uploads`，公开地址是 PUBLIC_ORIGIN 加 `/media` |
| 数据库类型 | PostgreSQL |
| 主机 / 端口 | `postgres` / `5432` |
| 数据库 / 用户 | `powerx` / `powerx` |
| 数据库密码 | setup-values 输出 database.password |
| Redis 主机 / 端口 | `redis` / `6379` |
| Redis 密码 | setup-values 输出 cache.password |
| 管理员账号、邮箱、密码 | 由你在页面设置，启动脚本不会代填密码 |
| 容器内部后端 / Web 端口 | `8080` / `3000`；宿主机入口端口仍由 Compose 管理 |
| 模型、邮箱 | 按实际服务填写；可先保持未启用 |

先测试数据库和 Redis 连接，再按向导保存配置。数据库步骤仅迁移表结构，尚不创建管理员或执行依赖管理员的营销 Skill/Agent 种子；管理员信息确认后，在“完成安装”阶段执行完整种子初始化。完成后后端通过 Docker 重启策略重新加载完整运行状态，页面进入登录。

```bash
curl -fsS http://127.0.0.1:18080/api/v1/admin/setup/status
```

首次预期 uninstalled/configured=false；完成后 installed/configured=true/restart_required=false。如果首次已经 installed，检查是否复用了旧 config/数据库；不要直接删除用户数据。应先备份，并在明确选择的新目录创建全新实例。

AI 模型、知识检索及具体插件需单独配置并验收。上传的插件必须匹配 Linux 架构，不能使用 macOS 构建的二进制。

## 6. HTTPS 或 XDocker 接入

已有反向代理时，可以保留内置 gateway 只监听回环，并代理到 18080。前端页面、`/api/`、`/ws/`、`/media/` 必须同源，代理需支持 WebSocket Upgrade 和长连接/SSE；TLS 在入口终止，转发原 Host 与 X-Forwarded-Proto。

使用 XDocker 时关闭独立 gateway，把后端/Web 接入该实例专用 ingress 网络，由共享 Nginx 接入并用 Certbot 签发证书。PostgreSQL、Redis 仍只在私有网络，不发布宿主机端口。**选择一种部署方式，避免启动两份指向同一数据目录的实例。**

## 7. 数据、备份、升级与回滚

```text
.env                 公开镜像版本、环境、浏览器地址
config/              数据库/Redis 密码、加密/签名密钥、运行配置和用户 Setup 草稿
data/postgres/       数据库
data/redis/          Redis AOF
data/runtime/        上传文件、插件、日志、运行状态
```

不同实例必须使用不同 project 名、config 和 data 目录，开发与正式环境不能共享它们。配置密钥丢失可能使已加密的数据无法读取，备份必须包含 config、数据库和上传/插件数据。

升级先备份数据库与上述私有数据，选择同一次通过验证的前后端 SHA，更新 `.env`，拉取镜像后按发布说明执行：

```bash
docker compose pull backend web-admin init
sh run.sh migrate
sh run.sh start
```

迁移失败停止升级，保留日志和备份。回滚镜像不等于回滚数据库；数据库有不兼容变化时需恢复对应备份。不得执行 `database refresh`、`down -v` 或删除数据目录来升级。

```bash
sh run.sh stop      # 停容器，保留配置和数据
sh run.sh start
```

管理员由 Setup 创建后，修改 `.env` 不会自动修改账号；域名、数据路径和身份相关调整应同时更新持久运行配置并重新验收。不要重新 init 或换一套签名密钥。
