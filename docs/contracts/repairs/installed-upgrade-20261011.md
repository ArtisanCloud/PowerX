# 已安装实例升级 Core 实施记录

日期：2026-10-11。任务分支：`codex/026-installed-instance-upgrade`。

## 已实现

- [升级合同](../powerx-installed-upgrade.md)、manifest/result JSON Schema、固定命令与退出码。
- 严格升级配置检查；init/serve/Go loader 不搜索替代配置、不生成密钥、不读取开发 .env、不重新 Setup。本版本不写配置补丁。
- 集中注册、内容寻址校验和、旧 IAM 履历兼容、UUID 尝试记录、只读 status/plan、锁内复核及 apply。
- 专用 PostgreSQL 连接执行锁；旧 migrate 与 apply 共用锁。事务变更和成功记录同提交，失败独立保留；非事务 Knowledge 失败要求恢复。
- 正式系统目录增量同步，保留权限/能力 UUID、effect/status、删除状态、用户描述/图、授权关系和已有租户版本锁。修复候选墓碑占用唯一键与 `metadata:tag:manage` 来源晋升，不复活候选、不执行整套 seed。
- 已安装实例正常启动时保留 Capability 身份与状态，避免启动目录同步覆盖保留结果。
- 固定真实旧镜像对；校验 OCI index/平台 digest/revision，新镜像 database 编译并回读发布 SHA。
- CI 保留空库安装，增加两架构的真实旧 Setup → 新迁移 → 凭据/业务/上传/插件回读 → 幂等/并发/失败/TERM/漂移/checksum/历史阻塞 → 备份恢复回滚。全部通过才生成 manifest 与发布包。

## 本地已执行验证

| 验证 | 结果与边界 |
|---|---|
| Python 初次 init | 2 项通过 |
| Python upgrade config/release | 6 项通过，含多组非法输入与制品拒绝；不是镜像验收 |
| Go database/config/数据库与权限相关包 | 通过；PostgreSQL 专项单独执行 |
| PostgreSQL runner（含 race） | 5 组通过；本机 PostgreSQL 14、专用 `powerx_upgrade_test_*` 库 |
| PostgreSQL Capability 保留 | UUID/ID、disabled、用户图及来源冲突验证通过 |
| 固定旧源码原生库 → 工作区 CLI | 4 步成功，第二次跳过全部；旧 migrate/seed 仅用于隔离旧库 fixture，不是 Docker 或 HTTP Setup 验收 |
| 配置/凭据/Profile grants/业务 | 摘要保持一致；目标使用 test-native / `aaaa…` 测试身份，不是发布 SHA |
| 真实 CLI JSON Schema | plan/apply/status 通过；[证据 JSON](../evidence/installed-upgrade-native-20261011.json) |
| Go vet、YAML/shell/Python 语法、diff | 通过；不代表 hosted runner 已运行 |

integration_gateway 的两个旧 media 测试要求已不存在的 builtin `com.corex.media.assets.manage`，整包测试失败；该定义未由本次升级修改。新增定向 PostgreSQL 测试通过，不以整个包全通过表述。

## 待生成的发布证据

本机 Docker daemon 未运行，GitHub CLI 未登录；本次没有推送、触发远程 workflow、修改 XDocker 或操作现有服务器。真实两架构镜像升级/失败/回滚 workflow **已实现但未执行**。正式 target digest、可执行 manifest、CI run 与发布包仍需分支提交后由 CI 生成。

只声明本地文件存储、同 Core 数据库/关闭的向量拓扑、测试进程插件范围。其他存储或业务插件需要额外验收；XDocker 必须按 `compatibility.validated_scope` 判断。

可以审阅和提交本分支，等待 CI 门禁通过后交付发布包。工作区二进制与仅构建成功的镜像不得当作远程自动升级已验收制品。

## 调试清理

本轮只使用短时 CLI、编译/测试进程和独立测试数据库，没有启动 Backend/Web/Redis/MinIO 服务。原用户进程未关闭或重启。交付前删除临时原生源码、二进制、私有配置及专用测试库；验收 JSON 不含凭证原值。
