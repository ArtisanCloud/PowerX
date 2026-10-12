# Setup 全新安装修复与合并验收

日期：2026-10-10。当前分支：`024-ai-engineering-skills`。

## 已同步 develop

当前分支已快进至远端 develop 的 `3d792fd7ea2426825970bceceb0d6e9db46211ef`，包含：

- `fd8fb1f5`：数据库步骤只执行 migrate；管理员确认后 Complete 执行完整 seed。
- `dd42c2da`：能力标识列宽调整为 256。
- `3d792fd7`：派生适配器标识列宽调整为 512。

同步前记录并校验了 77 个未提交文件，快进后内容逐字节保留。没有代替用户 commit 或 push。本次新增改动直接基于当前 develop，用户提交后即可合并至该 develop 基线；远端若又增加提交，合并前需重新核对。

## 本次补齐

1. **Complete 提前校验管理员**：要求非空用户名、有效邮箱、非空密码；提前拒绝 bcrypt 无法处理的超过 72 个 UTF-8 字节的密码。校验失败返回 400，发生在运行配置、密钥和数据库初始化写入之前。
2. **草稿保留空用户名**：没有草稿时页面仍可建议 `admin`；保存后省略或清空的用户名不再被读取默认值补回。
3. **页面按真实阶段展示**：数据库准备提示只说明表结构已准备；已有普通用户不再使页面跳过管理员确认，也不清空用户正在填写的管理员字段。
4. **回归测试隔离**：HTTP 编排测试使用临时工具桩；启动测试单独编译、直接管理服务进程、使用临时工作目录和空闲端口，并验证监听端口释放。完整 migrate/seed 由真实 PostgreSQL 隔离验收覆盖。
5. **CI 验证真实 schema**：检查能力列宽 256、适配器及健康探测标识列宽 512，并要求目录已发布。移除依赖某个目录 ID 长度恰好 ≥130 的硬编码断言。
6. 同步 OpenAPI 阶段语义、管理员要求、错误状态和部署说明。

管理员用户名可以自定义，`is_root=true` 表示身份权限，不要求用户名叫 `root`。没有修改 SeedRoot 在管理员确认前不创建默认账号的保护，也没有放宽营销 Skill/Agent 对真实管理员成员的依赖。

## 验收结果

| 检查 | 结果 |
| --- | --- |
| 首次启动，独立空 PostgreSQL 库 | 未安装、无应用表、无默认管理员 |
| 未填密码直接 Complete | 400；数据库仍无应用表 |
| 无管理员密码，Provision 连续两次 | 两次成功；用户数为 0；预存记录保留 |
| Provision 后仍未填密码，Complete | 400；未标记 installed；数据保留 |
| 填写自定义管理员后 Complete | 真实 migrate、完整 seed、能力发布成功；一位平台管理员 |
| 完成后重启临时 Core | installed 生效；自定义管理员登录成功 |
| 再次执行完整 seed | 不重复创建管理员；密码 hash 不变；预存记录保留 |
| 4 个受影响 Go 包的全部测试 | 通过 |
| 受影响 Go 包 go vet | 通过 |
| Nuxt 生产构建 | 通过 |
| 容器初始化器测试、JSON/YAML/Python 语法、git diff --check | 通过 |

完整验收证据及请求 trace：[`../evidence/setup-install-20261010.json`](../evidence/setup-install-20261010.json)。真实验收环境为本地新编译 Core + 独立 Docker PostgreSQL 16 / Redis 6；本轮未部署生产、未跑浏览器表单或完整 Docker 镜像发布 CI，也未执行全仓库 Go 测试。

临时 Core 的 18081、PostgreSQL 的 55433、Redis 的 56380 已清理释放。原有用户服务、备份验证容器和数据库保留。

## 重跑检查

在 `backend` 目录运行：

```bash
go test ./internal/transport/http/admin/system ./tests/contract/system ./tests/integration/system ./cmd/database/seed -count=1
go vet ./internal/transport/http/admin/system ./tests/contract/system ./tests/integration/system ./cmd/database/seed
```

在 `web-admin` 目录运行 `npm run build`。完整 Docker 安装链路由 `.github/workflows/publish-container.yml` 在独立 CI 库中执行 `deploy/powerx/container/smoke.py`，不能对已有部署库运行此空库验收脚本。

Setup 操作说明：[`../../../deploy/powerx/container/README.md`](../../../deploy/powerx/container/README.md)；接口合同：[`../../../specs/025-powerx-docker-systemd/contracts/http-openapi.yaml`](../../../specs/025-powerx-docker-systemd/contracts/http-openapi.yaml)。

初始化失败后不标记 installed；部分成功的种子写入可能已经保留，修正失败原因后重试。普通重试不重置已有管理员密码，不使用 refresh 清空数据库。
